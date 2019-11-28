package ipallocator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAllocate(t *testing.T) {
	ipRanges := []string{
		"192.168.1.2-192.168.1.20",
		"192.168.1.30-192.168.1.40",
		"192.168.1.80-192.168.1.100",
		"192.168.1.111",
		"192.168.1.140-192.168.1.180",
	}
	allocatedIPs := []string{
		"192.168.1.2", "192.168.1.3", "192.168.1.4",
		"192.168.1.38", "192.168.1.39", "192.168.1.40", "192.168.1.41", "192.168.1.79", "192.168.1.80", "192.168.1.81", "192.168.1.82",
		"192.168.1.178", "192.168.1.179", "192.168.1.180",
	}
	wantedIP1 := "192.168.1.18"
	resultIP1 := wantedIP1
	ip, capa, free, err := Allocate(ipRanges, allocatedIPs, wantedIP1)
	assert.Nil(t, err)
	assert.Equal(t, 93, capa)
	assert.Equal(t, 78, free)
	assert.Equal(t,
		resultIP1,
		ip,
	)

	wantedIP2 := "192.168.1.38"
	resultIP2 := "192.168.1.83"
	ip, capa, free, err = Allocate(ipRanges, allocatedIPs, wantedIP2)
	assert.Nil(t, err)
	assert.Equal(t, resultIP2, ip)

	wantedIP3 := "192.168.1.178" // this and all next IPs are buisy. 2nd pass from first range should be run
	resultIP3 := "192.168.1.5"
	ip, capa, free, err = Allocate(ipRanges, allocatedIPs, wantedIP3)
	assert.Nil(t, err)
	assert.Equal(t, resultIP3, ip)

	wantedIP4 := "10.0.0.1" // outside from any ranges
	resultIP4 := "192.168.1.5"
	ip, capa, free, err = Allocate(ipRanges, allocatedIPs, wantedIP4)
	assert.Nil(t, err)
	assert.Equal(t, resultIP4, ip)
}
