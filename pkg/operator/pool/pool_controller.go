/*
Copyright © 2018 inwinSTACK Inc

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pool

import (
	"fmt"
	"net"
	"reflect"
	"strings"

	"github.com/golang/glog"
	blendedv1 "github.com/inwinstack/blended/apis/inwinstack/v1"
	"github.com/inwinstack/blended/constants"
	"github.com/inwinstack/blended/k8sutil"
	"github.com/inwinstack/ipam/pkg/ipallocator"
	"github.com/inwinstack/ipam/pkg/version"
	"github.com/thoas/go-funk"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
)

func (c *Controller) reconcile(key string) error {
	// var poolCopy *blendedv1.Pool

	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: '%s'", key))
		return err
	}

	pool, err := c.lister.Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("pool '%s' in work queue no longer exists", key))
			return err
		}
		return err
	}

	if !pool.ObjectMeta.DeletionTimestamp.IsZero() {
		return c.cleanup(pool)
	}

	if err := c.checkAndUdateFinalizer(pool); err != nil {
		return err
	}

	ranges, capacity, err := c.calculateRanges(&pool.Spec)
	if err != nil {
		return c.makeFailedStatus(pool, err)
	} else if k8sutil.IsNeedToUpdate(pool.ObjectMeta) ||
		pool.Spec.Gateway != pool.Status.Gateway ||
		pool.Spec.CIDR != pool.Status.CIDR || pool.Status.CIDR == "" ||
		capacity != pool.Status.Capacity ||
		!reflect.DeepEqual(pool.Spec.Nameservers, pool.Status.Nameservers) ||
		!reflect.DeepEqual(ranges, pool.Status.Ranges) ||
		pool.Status.Phase != blendedv1.PoolActive {
		if err := c.makeStatus(pool); err != nil {
			return c.makeFailedStatus(pool, err)
		}
	}

	return nil
}

func (c *Controller) checkAndUdateFinalizer(pool *blendedv1.Pool) error {
	poolCopy := pool.DeepCopy()
	ok := funk.ContainsString(poolCopy.Finalizers, constants.CustomFinalizer)
	if poolCopy.Status.Phase == blendedv1.PoolActive && !ok {
		glog.V(4).Infof("UdateFinalizer for Pool '%s'", poolCopy.Name)
		k8sutil.AddFinalizer(&poolCopy.ObjectMeta, constants.CustomFinalizer)
		if err := c.updatePool(poolCopy); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) makeStatus(pool *blendedv1.Pool) error {
	glog.V(4).Infof("makeStatus for Pool '%s'", pool.Name)
	poolCopy := pool.DeepCopy()
	if poolCopy.Status.AllocatedIPs == nil {
		poolCopy.Status.AllocatedIPs = []string{}
	}

	if pool.Spec.CIDR != pool.Status.CIDR || pool.Status.CIDR == "" {
		// CIDR updated
		glog.V(4).Infof("Modifying Status.CIDR for Pool '%s'", pool.Name)
		cidr := strings.TrimSpace(pool.Spec.CIDR)
		ipA, ipN, err := net.ParseCIDR(cidr)
		if err != nil {
			return c.makeFailedStatus(
				pool,
				fmt.Errorf("Wrong CIDR '%s' for pool '%s': %w", cidr, pool.Name, err),
			)
		}
		ipAddr := ipA.String()
		masklen, _ := ipN.Mask.Size()
		if ipAddr != ipN.IP.String() {
			newCidr := fmt.Sprintf("%s/%d", ipN.IP, masklen)
			glog.V(4).Infof("Wrong CIDR '%s' for pool '%s'. Corrected, will be used '%s'.", cidr, pool.Name, newCidr)
			cidr = newCidr
			ipA, ipN, _ = net.ParseCIDR(cidr)
		}
		poolCopy.Status.CIDR = cidr

		//todo(sv): Check whether poolGateway into CIDR
	}

	if pool.Spec.Gateway != pool.Status.Gateway {
		glog.V(4).Infof("Modifying Status.Gateway for Pool '%s'", pool.Name)
		_, poolNet, err := net.ParseCIDR(poolCopy.Status.CIDR) // only poolCopy should be used here
		if err != nil {
			return c.makeFailedStatus(
				poolCopy,
				fmt.Errorf("Wrong Status.CIDR '%s' in the pool '%s': %w", poolCopy.Status.CIDR, pool.Name, err),
			)
		}
		masklen, _ := poolNet.Mask.Size()
		gwAddrString := strings.TrimSpace(pool.Spec.Gateway)
		gwA, gwNet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", gwAddrString, masklen))
		if err != nil {
			return c.makeFailedStatus(
				poolCopy,
				fmt.Errorf("Wrong Gateway '%s' for pool '%s': %w", gwAddrString, pool.Name, err),
			)
		}
		if !reflect.DeepEqual(*gwNet, *poolNet) {
			return c.makeFailedStatus(
				poolCopy,
				fmt.Errorf("Gateway '%s' not in pool '%s' CIDR '%s'", gwAddrString, pool.Name, poolCopy.Status.CIDR),
			)
		}
		poolCopy.Status.Gateway = gwA.String()
	}

	if !reflect.DeepEqual(pool.Spec.Nameservers, pool.Status.Nameservers) {
		glog.V(4).Infof("Modifying Status.Nameservers for Pool '%s'", pool.Name)
		// validate nameservers IP addrsses
		poolCopy.Status.Nameservers = []string{}
		for _, ip := range pool.Spec.Nameservers {
			nsA, _, err := net.ParseCIDR(fmt.Sprintf("%s/32", ip))
			nsAddr := nsA.String()
			if err != nil {
				return c.makeFailedStatus(
					poolCopy,
					fmt.Errorf("Wrong nameservr address '%s' for pool '%s': %w", ip, pool.Name, err),
				)
			}
			poolCopy.Status.Nameservers = append(poolCopy.Status.Nameservers, nsAddr)
		}
		// DO NOT SORT IT !!! order may be very important !!!
	}

	ranges, capacity, _ := c.calculateRanges(&pool.Spec)

	poolCopy.Status.Reason = ""
	poolCopy.Status.Capacity = capacity
	poolCopy.Status.Allocatable = capacity - len(poolCopy.Status.AllocatedIPs)

	poolCopy.Status.Ranges = ranges
	poolCopy.Status.Phase = blendedv1.PoolActive
	delete(poolCopy.Annotations, constants.NeedUpdateKey)
	k8sutil.AddFinalizer(&poolCopy.ObjectMeta, constants.CustomFinalizer)
	return c.updatePool(poolCopy)
}

func (c *Controller) makeFailedStatus(pool *blendedv1.Pool, e error) error {
	poolCopy := pool.DeepCopy()
	poolCopy.Status.Reason = e.Error()
	poolCopy.Status.Phase = blendedv1.PoolFailed
	delete(poolCopy.Annotations, constants.NeedUpdateKey)
	if err := c.updatePool(poolCopy); err != nil {
		return err
	}
	glog.Errorf("Pool got an error: %v", e)
	return nil
}

func (c *Controller) cleanup(pool *blendedv1.Pool) error {
	poolCopy := pool.DeepCopy()
	glog.V(4).Infof("Clanup Pool '%s'", poolCopy.Name)
	poolCopy.Status.Phase = blendedv1.PoolTerminating
	if len(poolCopy.Status.AllocatedIPs) == 0 {
		k8sutil.RemoveFinalizer(&poolCopy.ObjectMeta, constants.CustomFinalizer)
	}
	return c.updatePoolStatus(poolCopy)
}

func (c *Controller) updatePoolStatus(pool *blendedv1.Pool) error {
	pool.Status.LastUpdate = metav1.Now()
	pool.Status.Version = version.GetVersion()
	if _, err := c.blendedset.InwinstackV1().Pools().UpdateStatus(pool); err != nil {
		glog.V(4).Infof("error while update poolStatus '%s': %+v.", pool.Name, err)
		return err
	}
	return nil
}

func (c *Controller) updatePool(pool *blendedv1.Pool) (err error) {
	var newPool *blendedv1.Pool
	poolStatus := pool.Status.DeepCopy()
	if newPool, err = c.blendedset.InwinstackV1().Pools().Update(pool); err != nil {
		glog.V(4).Infof("error while update pool '%s': %+v.", pool.Name, err)
	} else {
		newPool.Status = *poolStatus
		err = c.updatePoolStatus(newPool)
	}
	return err
}

func (c *Controller) calculateRanges(spec *blendedv1.PoolSpec) ([]string, int, error) {
	return ipallocator.CalculateRanges(
		spec.CIDR, spec.IncludeRanges,
		spec.UseWholeCidr,
		spec.ExcludeRanges, []string{spec.Gateway}, spec.Nameservers,
	)
}
