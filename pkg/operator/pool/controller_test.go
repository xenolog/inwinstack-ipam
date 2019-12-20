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
	"context"
	"testing"
	"time"

	blendedv1 "github.com/inwinstack/blended/apis/inwinstack/v1"
	blendedfake "github.com/inwinstack/blended/generated/clientset/versioned/fake"
	blendedinformers "github.com/inwinstack/blended/generated/informers/externalversions"
	"github.com/inwinstack/ipam/pkg/config"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const timeout = 5 * time.Second

func TestPoolController(t *testing.T) {
	var lastUpdate metav1.Time
	ctx, cancel := context.WithCancel(context.Background())
	cfg := &config.Config{Threads: 2}
	blendedset := blendedfake.NewSimpleClientset()
	informer := blendedinformers.NewSharedInformerFactory(blendedset, 0)

	controller := NewController(blendedset, informer.Inwinstack().V1().Pools())
	go informer.Start(ctx.Done())
	assert.Nil(t, controller.Run(ctx, cfg.Threads))

	pool := &blendedv1.Pool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pool",
		},
		Spec: blendedv1.PoolSpec{
			CIDR: "172.22.132.0/24",
			// UseWholeCidr:  true,
		},
	}

	// Create the pool
	_, err := blendedset.InwinstackV1().Pools().Create(pool)
	assert.Nil(t, err)

	failed := true
	for start := time.Now(); time.Since(start) < timeout; {
		p, err := blendedset.InwinstackV1().Pools().Get(pool.Name, metav1.GetOptions{})
		assert.Nil(t, err)

		if p.Status.Phase == blendedv1.PoolActive {
			assert.Equal(t, []string{}, p.Status.AllocatedIPs)
			assert.Equal(t, []string{"172.22.132.1-172.22.132.254"}, p.Status.Ranges)
			assert.Equal(t, 254, p.Status.Capacity)
			assert.Equal(t, 254, p.Status.Allocatable)
			failed = false
			lastUpdate = p.Status.LastUpdate
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	assert.Equal(t, false, failed, "The pool object failed to make status.")

	// Success to update the pool by adding gateway in the middle of network
	gpool, err := blendedset.InwinstackV1().Pools().Get(pool.Name, metav1.GetOptions{})
	assert.Nil(t, err)

	gpool.Spec.Gateway = "172.22.132.5"
	_, err = blendedset.InwinstackV1().Pools().Update(gpool)
	assert.Nil(t, err)

	failed = true
	for start := time.Now(); time.Since(start) < timeout; {
		p, err := blendedset.InwinstackV1().Pools().Get(pool.Name, metav1.GetOptions{})
		assert.Nil(t, err)

		if p.Status.LastUpdate != lastUpdate {
			assert.Equal(t, 253, p.Status.Capacity)
			assert.Equal(t, 253, p.Status.Allocatable)
			assert.Equal(t, []string{
				"172.22.132.1-172.22.132.4",
				"172.22.132.6-172.22.132.254",
			}, p.Status.Ranges)
			failed = false
			lastUpdate = p.Status.LastUpdate
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	assert.Equal(t, false, failed, "The service object failed to sync status.")
	// lastUpdate := gpool.Status.LastUpdate

	// Success to update the pool by UseWholeCidr
	gpool, err = blendedset.InwinstackV1().Pools().Get(pool.Name, metav1.GetOptions{})
	assert.Nil(t, err)

	gpool.Spec.UseWholeCidr = true
	_, err = blendedset.InwinstackV1().Pools().Update(gpool)
	assert.Nil(t, err)

	failed = true
	for start := time.Now(); time.Since(start) < timeout; {
		p, err := blendedset.InwinstackV1().Pools().Get(pool.Name, metav1.GetOptions{})
		assert.Nil(t, err)

		if p.Status.LastUpdate != lastUpdate { // 256 - 1 because Gateway set
			assert.Equal(t, 255, p.Status.Allocatable)
			assert.Equal(t, []string{
				"172.22.132.0-172.22.132.4",
				"172.22.132.6-172.22.132.255",
			}, p.Status.Ranges)
			failed = false
			lastUpdate = p.Status.LastUpdate
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	assert.Equal(t, false, failed, "The service object failed to sync status.")

	// Success to update the pool by exclude address range
	gpool, err = blendedset.InwinstackV1().Pools().Get(pool.Name, metav1.GetOptions{})
	assert.Nil(t, err)

	gpool.Spec.ExcludeRanges = append(gpool.Spec.ExcludeRanges, []string{
		"172.22.132.0-172.22.132.5",
		"172.22.132.32-172.22.132.64",
		"172.22.132.250-172.22.132.255",
	}...)
	_, err = blendedset.InwinstackV1().Pools().Update(gpool)
	assert.Nil(t, err)

	failed = true
	for start := time.Now(); time.Since(start) < timeout; {
		p, err := blendedset.InwinstackV1().Pools().Get(pool.Name, metav1.GetOptions{})
		assert.Nil(t, err)

		if p.Status.LastUpdate != lastUpdate {
			assert.Equal(t, 211, p.Status.Capacity)
			assert.Equal(t, 211, p.Status.Allocatable)
			assert.Equal(t, []string{
				"172.22.132.6-172.22.132.31",
				"172.22.132.65-172.22.132.249",
			}, p.Status.Ranges)
			failed = false
			lastUpdate = p.Status.LastUpdate
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	assert.Equal(t, false, failed, "The service object failed to sync status.")

	// Success to update the pool, define includeRanges
	gpool, err = blendedset.InwinstackV1().Pools().Get(pool.Name, metav1.GetOptions{})
	assert.Nil(t, err)

	gpool.Spec.IncludeRanges = append(gpool.Spec.IncludeRanges, []string{
		"172.22.132.8-172.22.132.16",
		"172.22.132.128-172.22.132.196",
	}...)
	_, err = blendedset.InwinstackV1().Pools().Update(gpool)
	assert.Nil(t, err)

	failed = true
	for start := time.Now(); time.Since(start) < timeout; {
		p, err := blendedset.InwinstackV1().Pools().Get(pool.Name, metav1.GetOptions{})
		assert.Nil(t, err)

		if p.Status.LastUpdate != lastUpdate {
			assert.Equal(t, 78, p.Status.Capacity)
			assert.Equal(t, 78, p.Status.Allocatable)
			assert.Equal(t, []string{
				"172.22.132.8-172.22.132.16",
				"172.22.132.128-172.22.132.196",
			}, p.Status.Ranges)
			failed = false
			lastUpdate = p.Status.LastUpdate
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	assert.Equal(t, false, failed, "The service object failed to sync status.")

	// Success to update the pool, define DNS and cut provided ranges
	gpool, err = blendedset.InwinstackV1().Pools().Get(pool.Name, metav1.GetOptions{})
	assert.Nil(t, err)

	gpool.Spec.Nameservers = append(gpool.Spec.Nameservers, []string{
		"172.22.132.12",
		"172.22.132.140",
	}...)
	_, err = blendedset.InwinstackV1().Pools().Update(gpool)
	assert.Nil(t, err)

	failed = true
	for start := time.Now(); time.Since(start) < timeout; {
		p, err := blendedset.InwinstackV1().Pools().Get(pool.Name, metav1.GetOptions{})
		assert.Nil(t, err)

		if p.Status.LastUpdate != lastUpdate {
			assert.Equal(t, 76, p.Status.Capacity)
			assert.Equal(t, 76, p.Status.Allocatable)
			assert.Equal(t, []string{
				"172.22.132.8-172.22.132.11",
				"172.22.132.13-172.22.132.16",
				"172.22.132.128-172.22.132.139",
				"172.22.132.141-172.22.132.196",
			}, p.Status.Ranges)
			failed = false
			lastUpdate = p.Status.LastUpdate
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	assert.Equal(t, false, failed, "The service object failed to sync status.")

	// Failed to update the pool
	gpool, err = blendedset.InwinstackV1().Pools().Get(pool.Name, metav1.GetOptions{})
	assert.Nil(t, err)
	gpool.Spec.IncludeRanges = []string{"172.22.132.250-172.22.132.267"}

	_, err = blendedset.InwinstackV1().Pools().Update(gpool)
	assert.Nil(t, err)

	failed = true
	for start := time.Now(); time.Since(start) < timeout; {
		p, err := blendedset.InwinstackV1().Pools().Get(pool.Name, metav1.GetOptions{})
		assert.Nil(t, err)

		if p.Status.Phase == blendedv1.PoolFailed {
			assert.NotNil(t, p.Status.Reason)
			failed = false
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	assert.Equal(t, false, failed, "The service object failed to get error status.")

	// Delete the pool
	assert.Nil(t, blendedset.InwinstackV1().Pools().Delete(pool.Name, nil))

	_, err = blendedset.InwinstackV1().Pools().Get(pool.Name, metav1.GetOptions{})
	assert.NotNil(t, err)

	cancel()
	controller.Stop()
}

func TestWrongCidrPool(t *testing.T) {
	// var lastUpdate metav1.Time
	ctx, cancel := context.WithCancel(context.Background())
	cfg := &config.Config{Threads: 2}
	blendedset := blendedfake.NewSimpleClientset()
	informer := blendedinformers.NewSharedInformerFactory(blendedset, 0)

	controller := NewController(blendedset, informer.Inwinstack().V1().Pools())
	go informer.Start(ctx.Done())
	assert.Nil(t, controller.Run(ctx, cfg.Threads))

	pool := &blendedv1.Pool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pool",
		},
		Spec: blendedv1.PoolSpec{
			CIDR: "172.18.170.65/26",
			ExcludeRanges: []string{
				"172.18.170.81",
				"172.18.170.82-172.18.170.90",
			},
			Gateway: "172.18.170.65",
			IncludeRanges: []string{
				"172.18.170.92-172.18.170.110",
			},
			Nameservers: []string{
				"172.18.176.6",
			},
		},
	}

	// Create the pool
	_, err := blendedset.InwinstackV1().Pools().Create(pool)
	assert.Nil(t, err)

	failed := true
	for start := time.Now(); time.Since(start) < timeout; {
		p, err := blendedset.InwinstackV1().Pools().Get(pool.Name, metav1.GetOptions{})
		assert.Nil(t, err)

		if p.Status.Phase == blendedv1.PoolActive {
			assert.Equal(t, []string{}, p.Status.AllocatedIPs)
			assert.Equal(t, "172.18.170.64/26", p.Status.CIDR)
			assert.Equal(t, []string{"172.18.170.92-172.18.170.110"}, p.Status.Ranges)
			failed = false
			// lastUpdate = p.Status.LastUpdate
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	assert.Equal(t, false, failed, "The pool object failed to make status.")

	cancel()
	controller.Stop()
}
