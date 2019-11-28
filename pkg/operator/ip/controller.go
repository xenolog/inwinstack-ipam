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
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/thoas/go-funk"

	"github.com/golang/glog"
	blendedv1 "github.com/inwinstack/blended/apis/inwinstack/v1"
	"github.com/inwinstack/blended/constants"
	blended "github.com/inwinstack/blended/generated/clientset/versioned"
	informerv1 "github.com/inwinstack/blended/generated/informers/externalversions/inwinstack/v1"
	listerv1 "github.com/inwinstack/blended/generated/listers/inwinstack/v1"
	"github.com/inwinstack/blended/k8sutil"
	"github.com/inwinstack/ipam/pkg/config"
	"github.com/inwinstack/ipam/pkg/ipaddr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Controller represents the controller of ip
type Controller struct {
	blendedset blended.Interface
	lister     listerv1.IPLister
	synced     cache.InformerSynced
	queue      workqueue.RateLimitingInterface
}

// NewController creates an instance of the ip controller
func NewController(blendedset blended.Interface, informer informerv1.IPInformer) *Controller {
	controller := &Controller{
		blendedset: blendedset,
		lister:     informer.Lister(),
		synced:     informer.Informer().HasSynced,
		queue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "IPs"),
	}
	informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueue,
		UpdateFunc: func(old, new interface{}) {
			oo := old.(*blendedv1.IP)
			no := new.(*blendedv1.IP)
			k8sutil.MakeNeedToUpdate(&no.ObjectMeta, oo.Spec, no.Spec)
			if k8sutil.IsNeedToUpdate(no.ObjectMeta) {
				// Don't change the IP pool name
				no.Spec.PoolName = oo.Spec.PoolName
			}
			controller.enqueue(no)
		},
	})
	return controller
}

// Run serves the ip controller
func (c *Controller) Run(ctx context.Context, threadiness int) error {
	glog.Info("Starting the ip controller")
	glog.Info("Waiting for the ip informer caches to sync")
	if ok := cache.WaitForCacheSync(ctx.Done(), c.synced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, ctx.Done())
	}
	return nil
}

// Stop stops the ip controller
func (c *Controller) Stop() {
	glog.Info("Stopping the ip controller")
	c.queue.ShutDown()
}

func (c *Controller) runWorker() {
	defer utilruntime.HandleCrash()
	for c.processNextWorkItem() {
	}
}

func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.queue.Get()
	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.queue.Done(obj)
		key, ok := obj.(string)
		if !ok {
			c.queue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("IP expected string in workqueue but got %#v", obj))
			return nil
		}

		if err := c.reconcile(key); err != nil {
			c.queue.AddRateLimited(key)
			return fmt.Errorf("IP error syncing '%s': %s, requeuing", key, err.Error())
		}

		c.queue.Forget(obj)
		glog.V(2).Infof("IP successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}
	return true
}

func (c *Controller) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.queue.Add(key)
}

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
		glog.V(4).Infof("Allocate address for '%s'", ip.Name)
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
	ip.Status.LastUpdateTime = metav1.Now()
	delete(ip.Annotations, constants.NeedUpdateKey)
	if _, err := c.blendedset.InwinstackV1().IPs(ip.Namespace).UpdateStatus(ip); err != nil {
		return err
	}
	return nil
}

func (c *Controller) allocate(ip *blendedv1.IP) (rv error) {
	var allocatedIP string

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
		glog.V(4).Infof("MAC label for '%s' changed to '%s'", ip.Name, mac)
		updateIP = true
	}

	if ipCopy.Labels[config.PoolLabel] != ip.Spec.PoolName {
		// Setup or change Label for Pool
		ipCopy.Labels[config.PoolLabel] = ip.Spec.PoolName
		glog.V(4).Infof("Pool label for '%s' changed to '%s'", ip.Name, ip.Spec.PoolName)
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

			parser := ipaddr.NewParser(pool.Spec.Addresses, pool.Spec.AvoidBuggyIPs, pool.Spec.AvoidGatewayIPs)
			ips, err := parser.FilterIPs(pool.Status.AllocatedIPs, pool.Spec.FilterIPs)
			if err != nil {
				return c.makeFailedStatus(ipCopy, err)
			}

			if ipCopy.Spec.WantedAddress != "" {
				// Specific IP address requested:
				// check to duplicate allocation
				if funk.ContainsString(pool.Status.AllocatedIPs, ipCopy.Spec.WantedAddress) {
					return c.makeFailedStatus(
						ipCopy,
						fmt.Errorf("IP address '%s' can't be allocated twice from pool '%s'", ipCopy.Spec.WantedAddress, pool.Name),
					)
				}
				// check is IP address in pool range
				if !funk.ContainsString(ips, ipCopy.Spec.WantedAddress) {
					return c.makeFailedStatus(
						ipCopy,
						fmt.Errorf("IP address '%s' is out of range %v for pool '%s'", ipCopy.Spec.WantedAddress, pool.Spec.Addresses, pool.Name),
					)
				}
				allocatedIP = ipCopy.Spec.WantedAddress
			} else {
				allocatedIP = ips[0]
			}

			poolCopy.Status.AllocatedIPs = append(poolCopy.Status.AllocatedIPs, allocatedIP)
			poolCopy.Status.Allocatable = poolCopy.Status.Capacity - len(poolCopy.Status.AllocatedIPs)
			ipCopy.Status.Reason = ""
			ipCopy.Status.Address = allocatedIP
			updateIPstatus = true
			if err := c.updatePoolStatus(poolCopy); err != nil {
				// If the pool failed to update, this res will requeue
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
	}

	if ipCopy.Labels[config.IPlabel] != ipCopy.Status.Address {
		// Setup or change Label for IP address
		if ipCopy.Status.Address != "" {
			ipCopy.Labels[config.IPlabel] = ipCopy.Status.Address
		} else {
			delete(ipCopy.Labels, config.IPlabel)
		}
		glog.V(4).Infof("IP label for '%s' changed to '%s'", ip.Name, ipCopy.Status.Address)
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

	ipCopy.Status.LastUpdateTime = metav1.Now()
	ipCopy.Status.Phase = blendedv1.IPTerminating
	delete(ip.Annotations, constants.NeedUpdateKey)
	k8sutil.RemoveFinalizer(&ipCopy.ObjectMeta, constants.CustomFinalizer)
	if _, err := c.blendedset.InwinstackV1().IPs(ipCopy.Namespace).UpdateStatus(ipCopy); err != nil {
		return err
	}
	return nil
}

func (c *Controller) updatePoolStatus(pool *blendedv1.Pool) error {
	pool.Status.LastUpdateTime = metav1.Now()
	if _, err := c.blendedset.InwinstackV1().Pools().UpdateStatus(pool); err != nil {
		glog.V(4).Infof("error while update poolStatus '%s': %+v.", pool.Name, err)
		return err
	}
	return nil
}

func (c *Controller) updatePool(pool *blendedv1.Pool) (err error) {
	var newPool *blendedv1.Pool
	poolStatus := pool.Status.DeepCopy()
	// poolCopy := pool.DeepCopy()
	if newPool, err = c.blendedset.InwinstackV1().Pools().Update(pool); err != nil {
		glog.V(4).Infof("error while update pool '%s': %+v.", pool.Name, err)
	} else {
		newPool.Status = *poolStatus
		err = c.updatePoolStatus(newPool)
	}
	return err
}

func (c *Controller) updateIPStatus(ip *blendedv1.IP) error {
	ip.Status.LastUpdateTime = metav1.Now()
	if _, err := c.blendedset.InwinstackV1().IPs(ip.Namespace).UpdateStatus(ip); err != nil {
		glog.V(4).Infof("error while update IPStatus '%s': %+v.", ip.Name, err)
		return err
	}
	return nil
}

func (c *Controller) updateIP(ip *blendedv1.IP) (err error) {
	var newIP *blendedv1.IP
	IPStatus := ip.Status.DeepCopy()
	if newIP, err = c.blendedset.InwinstackV1().IPs(ip.Namespace).Update(ip); err != nil {
		glog.V(4).Infof("error while update IP '%s': %+v.", ip.Name, err)
	} else {
		newIP.Status = *IPStatus
		err = c.updateIPStatus(newIP)
	}
	return err
}
