package ipallocator

import (
	"fmt"
	"net"

	"github.com/inwinstack/ipam/pkg/cidr32"
)

// Allocate ip address from IP ranges.
// Returns:
// - IP address as string (or "" if can't allocate)
// - capacity and amount of unallocated addresses AFTER THIS allocation
// - error if happens
func Allocate(ranges, allocated []string, wanted string) (rv string, capacity, free int, err error) {
	var (
		ipRanges cidr32.IPRangeList
		wantedIP uint32
	)
	for _, rs := range ranges {
		tmp, err := cidr32.NewRange(rs)
		if err != nil {
			return "", 0, 0, fmt.Errorf("Can't create IPRange for '%s': %w", rs, err)
		}
		ipRanges = append(ipRanges, *tmp)
	}
	ipRanges = ipRanges.Arranged()

	allocatedIPs := cidr32.NewIPList(allocated)

	rIDX := -1
	if tmp := net.ParseIP(wanted); wanted != "" && tmp != nil {
		wantedIP = cidr32.IPtoUint32(tmp)
		// Search Range index, which include wantedIP
		tmpRange, _ := cidr32.New32Range(wantedIP, wantedIP)
		for i, rr := range ipRanges {
			if rr.IsIntersect(tmpRange) {
				rIDX = i
				break
			}
		}
		rIDX = 0 // only if not found
	} else if len(ipRanges) > 0 {
		// no need to search some specific IP, use first
		wantedIP = ipRanges[0].First32()
		rIDX = 0
	} else {
		// No able to allocate
		return "", 0, 0, fmt.Errorf("Can't allocate IP '%s' for empty range list", wanted)
	}

	// try to find 1st non-allocated address after (or equal) of wantedIP if it possible
	// if inpossible -- will be processd full search from 1st range
	lastRangeIDX := len(ipRanges) - 1
	firstPass := true
exLoop:
	for iR := rIDX; iR <= lastRangeIDX; iR++ {
		// list existing IP rangs by index, start from wanted's IP range, from wanted IP
		startBy := ipRanges[iR].First32()
		if firstPass && startBy < wantedIP {
			startBy = wantedIP
		}
		for ip := startBy; ip <= ipRanges[iR].Last32(); ip++ {
			if allocatedIPs.Index(ip) < 0 {
				// found unallocated IP
				rv = fmt.Sprintf("%s", cidr32.Uint32toIP(ip))
				break exLoop
			}
		}
		if firstPass && iR == lastRangeIDX { // should be last
			firstPass = false
			iR = -1 // because i++ wil be executed after FOR body
		}
	}

	return rv, ipRanges.Capacity(), ipRanges.Capacity() - len(*allocatedIPs) - 1, nil
}

// ----------------------------------------------------------------------------
