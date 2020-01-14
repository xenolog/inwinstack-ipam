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

package ip

import (
	"fmt"
	"net"
	"strings"

	blendedv1 "github.com/inwinstack/blended/apis/inwinstack/v1"
	"github.com/inwinstack/blended/constants"
	"github.com/inwinstack/blended/k8sutil"
	"github.com/inwinstack/ipam/pkg/config"
	"github.com/inwinstack/ipam/pkg/ipallocator"
	"github.com/inwinstack/ipam/pkg/version"
	"github.com/thoas/go-funk"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

func (c *Controller) reconcile(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return err
	}

	ip, err := c.lister.IPs(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("ip '%s' in work queue no longer exists", key))
			return err
		}
		return err
	}

	if !ip.ObjectMeta.DeletionTimestamp.IsZero() {
		return c.deallocate(ip)
	}

	if err := c.checkAndUdateFinalizer(ip); err != nil {
		return err
	}

	if ip.Status.Phase != blendedv1.IPActive || k8sutil.IsNeedToUpdate(ip.ObjectMeta) {
		klog.Infof("Allocate address for '%s'", ip.Name)
		if err := c.allocate(ip); err != nil {
			return err
		}
	}

	return nil
}

func (c *Controller) checkAndUdateFinalizer(ip *blendedv1.IP) error {
	ipCopy := ip.DeepCopy()
	ok := funk.ContainsString(ipCopy.Finalizers, constants.CustomFinalizer)
	if ipCopy.Status.Phase == blendedv1.IPActive && !ok {
		k8sutil.AddFinalizer(&ipCopy.ObjectMeta, constants.CustomFinalizer)
		if _, err := c.blendedset.InwinstackV1().IPs(ipCopy.Namespace).Update(ipCopy); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) makeFailedStatus(ip *blendedv1.IP, e error) error {
	ip.Status.Address = ""
	ip.Status.Phase = blendedv1.IPFailed
	ip.Status.Reason = fmt.Sprintf("%+v.", e)
	delete(ip.Annotations, constants.NeedUpdateKey)
	return c.updateIPStatus(ip)
}

func (c *Controller) allocate(ip *blendedv1.IP) (rv error) {
	var wantedIP string

	ipCopy := ip.DeepCopy()
	updateIP := false
	updateIPstatus := false

	if ipCopy.Labels == nil {
		ipCopy.Labels = map[string]string{}
	}

	if strings.ToUpper(ip.Spec.MAC) != ip.Status.MAC {
		// Setup or change Label and Status for MAC address
		mac := strings.ToUpper(ip.Spec.MAC)
		if mac != "" {
			ipCopy.Labels[config.MacLabel] = strings.ReplaceAll(mac, ":", "-")
		} else {
			delete(ipCopy.Labels, config.MacLabel)
		}
		ipCopy.Status.MAC = mac
		klog.Infof("MAC label for '%s' changed to '%s'", ip.Name, mac)
		updateIP = true
	}

	if ipCopy.Labels[config.PoolLabel] != ip.Spec.PoolName {
		// Setup or change Label for Pool
		ipCopy.Labels[config.PoolLabel] = ip.Spec.PoolName
		klog.Infof("Pool label for '%s' changed to '%s'", ip.Name, ip.Spec.PoolName)
		updateIP = true
	}

	pool, err := c.blendedset.InwinstackV1().Pools().Get(ipCopy.Spec.PoolName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	poolCopy := pool.DeepCopy()

	switch pool.Status.Phase {
	case blendedv1.PoolActive:

		// Validate POOL's CIDR and prepare for future use
		_, cidrNet, err := net.ParseCIDR(pool.Status.CIDR)
		if err != nil {
			return c.makeFailedStatus(ipCopy, fmt.Errorf("The '%s' pool is inoperable, wrong CIDR: %w", pool.Name, err))
		}

		if ipCopy.Status.Address == "" {
			if pool.Status.Allocatable == 0 {
				return c.makeFailedStatus(ipCopy, fmt.Errorf("The '%s' pool has been exhausted", pool.Name))
			}

			if ipCopy.Spec.WantedAddress != "" {
				// Specific IP address requested:
				// check to duplicate allocation
				if funk.ContainsString(pool.Status.AllocatedIPs, ipCopy.Spec.WantedAddress) {
					return c.makeFailedStatus(
						ipCopy,
						fmt.Errorf("Requested IP address '%s' already allocated from pool '%s'", ipCopy.Spec.WantedAddress, pool.Name),
					)
				}
				wantedIP = ipCopy.Spec.WantedAddress
			} else {
				a := len(pool.Status.AllocatedIPs)
				if a > 0 {
					wantedIP = pool.Status.AllocatedIPs[a-1]
				} else {
					wantedIP = ""
				}
			}

			allocatedIP, _, freeIPs, err := ipallocator.Allocate(poolCopy.Status.Ranges, pool.Status.AllocatedIPs, wantedIP)
			if err != nil {
				return c.makeFailedStatus(ipCopy, err)
			} else if allocatedIP != wantedIP && ipCopy.Spec.WantedAddress != "" {
				return c.makeFailedStatus(
					ipCopy,
					fmt.Errorf("Requested IP address '%s' can't be allocated from pool '%s'", ipCopy.Spec.WantedAddress, pool.Name),
				)
			}

			if ipCopy.Spec.WantedAddress != "" {
				// prevent to break seris
				poolCopy.Status.AllocatedIPs = append([]string{allocatedIP}, poolCopy.Status.AllocatedIPs...)
			} else {
				poolCopy.Status.AllocatedIPs = append(poolCopy.Status.AllocatedIPs, allocatedIP)
			}
			poolCopy.Status.Allocatable = freeIPs
			ipCopy.Status.Reason = ""
			ipCopy.Status.Address = allocatedIP
			updateIPstatus = true
			if err := c.updatePoolStatus(poolCopy); err != nil {
				// If the pool failed to update, this resource will be requeued
				return err
			}

			ipCopy.Status.Phase = blendedv1.IPActive
			k8sutil.AddFinalizer(&ipCopy.ObjectMeta, constants.CustomFinalizer)
		}

		// Add CIDR notation, using Pool's CIDR
		cidrNetSize, _ := cidrNet.Mask.Size()
		ipCopy.Status.CIDR = fmt.Sprintf("%s/%d", ipCopy.Status.Address, cidrNetSize)
		ipCopy.Status.Gateway = pool.Status.Gateway
		ipCopy.Status.Nameservers = pool.Status.Nameservers
		updateIPstatus = true

	case blendedv1.PoolTerminating:
		ipCopy.Status.Reason = fmt.Sprintf("The '%s' pool has been terminated.", pool.Name)
		ipCopy.Status.Phase = blendedv1.IPFailed
		updateIPstatus = true

	default:
		ipCopy.Status.Reason = fmt.Sprintf("The '%s' pool is inoperable", pool.Name)
		ipCopy.Status.Phase = blendedv1.IPFailed
		updateIPstatus = true
	}

	if ipCopy.Labels[config.IPlabel] != ipCopy.Status.Address {
		// Setup or change Label for IP address
		if ipCopy.Status.Address != "" {
			ipCopy.Labels[config.IPlabel] = ipCopy.Status.Address
		} else {
			delete(ipCopy.Labels, config.IPlabel)
		}
		klog.Infof("IP label for '%s' changed to '%s'", ip.Name, ipCopy.Status.Address)
		updateIP = true
	}

	if _, ok := ipCopy.Annotations[constants.NeedUpdateKey]; ok {
		delete(ipCopy.Annotations, constants.NeedUpdateKey)
		updateIP = true
	}

	if updateIP {
		rv = c.updateIP(ipCopy)
	} else if updateIPstatus {
		rv = c.updateIPStatus(ipCopy)
	}

	return rv
}

func (c *Controller) deallocate(ip *blendedv1.IP) error {
	ipCopy := ip.DeepCopy()
	pool, err := c.blendedset.InwinstackV1().Pools().Get(ipCopy.Spec.PoolName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	poolCopy := pool.DeepCopy()
	poolCopy.Status.AllocatedIPs = funk.FilterString(poolCopy.Status.AllocatedIPs, func(v string) bool {
		return v != ip.Status.Address
	})
	poolCopy.Status.Allocatable = poolCopy.Status.Capacity - len(poolCopy.Status.AllocatedIPs)
	if err := c.updatePoolStatus(poolCopy); err != nil {
		return err
	}

	ipCopy.Status.Phase = blendedv1.IPTerminating
	delete(ip.Annotations, constants.NeedUpdateKey)
	k8sutil.RemoveFinalizer(&ipCopy.ObjectMeta, constants.CustomFinalizer)
	return c.updateIP(ipCopy)
}

func (c *Controller) updatePoolStatus(pool *blendedv1.Pool) error {
	pool.Status.Version = version.GetVersion()
	pool.Status.LastUpdate = metav1.Now()
	if _, err := c.blendedset.InwinstackV1().Pools().UpdateStatus(pool); err != nil {
		klog.Infof("error while update poolStatus '%s': %+v.", pool.Name, err)
		return err
	}
	return nil
}

func (c *Controller) updatePool(pool *blendedv1.Pool) (err error) {
	var newPool *blendedv1.Pool
	poolStatus := pool.Status.DeepCopy()
	// poolCopy := pool.DeepCopy()
	if newPool, err = c.blendedset.InwinstackV1().Pools().Update(pool); err != nil {
		klog.Infof("error while update pool '%s': %+v.", pool.Name, err)
	} else {
		newPool.Status = *poolStatus
		err = c.updatePoolStatus(newPool)
	}
	return err
}

func (c *Controller) updateIPStatus(ip *blendedv1.IP) error {
	ip.Status.Version = version.GetVersion()
	ip.Status.LastUpdate = metav1.Now()
	if _, err := c.blendedset.InwinstackV1().IPs(ip.Namespace).UpdateStatus(ip); err != nil {
		klog.Infof("error while update IPStatus '%s': %+v.", ip.Name, err)
		return err
	}
	return nil
}

func (c *Controller) updateIP(ip *blendedv1.IP) (err error) {
	var newIP *blendedv1.IP
	IPStatus := ip.Status.DeepCopy()
	if newIP, err = c.blendedset.InwinstackV1().IPs(ip.Namespace).Update(ip); err != nil {
		klog.Infof("error while update IP '%s': %+v.", ip.Name, err)
	} else {
		newIP.Status = *IPStatus
		err = c.updateIPStatus(newIP)
	}
	return err
}
