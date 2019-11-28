package ipallocator

import (
	"fmt"
	"net"
	"strings"

	"github.com/inwinstack/ipam/pkg/cidr32"
)

// ----------------------------------------------------------------------------

// CalculateRanges -- got CIDR, more specific `includeRanges` and `useWholeCidr` flag allows to use network
// address and broadcast as valid IP addresses and makes a set of ranges.
// From this set it makes the result set by subtracting an arbitrary number excludes
// CIDR should be in `A.B.C.D/NN` format
// ranges may be in the `A.B.C.D-E.F.G.H` or `A.B.C.D` for single address format
func CalculateRanges(cidrS string, includeRanges []string, useWholeCidr bool, excls ...[]string) ([]string, int, error) {
	var ranges cidr32.IPRangeList

	cidrS, cidrN, err := CleanupCidr(cidrS)
	if err != nil {
		return []string{}, 0, fmt.Errorf("Wrong CIDR '%s': %w", cidrS, err)
	}

	if len(includeRanges) == 0 {
		// calculate ranges from CIDR
		tmp, _ := cidr32.CidrToRange(cidrN, !useWholeCidr)
		ranges = append(ranges, tmp)
	} else {
		for _, r := range includeRanges {
			rng, err := cidr32.NewRange(r)
			if err != nil {
				return []string{}, 0, fmt.Errorf("Wrong includeRange for CIDR '%s': %w", cidrS, err)
			}
			tmp, _ := rng.CutToCidr(cidrN, !useWholeCidr)
			ranges = append(ranges, tmp)
		}
	}

	for _, allExcludes := range excls {
		for _, subExcludes := range allExcludes {
			if subExcludes != "" {
				rng, err := cidr32.NewRange(subExcludes)
				if err != nil {
					return []string{}, 0, fmt.Errorf("Wrong excludeRange for CIDR '%s': %w", cidrS, err)
				}
				if tmp, r := ranges.ExcludeRange(rng); r != 0 {
					ranges = *tmp
				}
			}
		}
	}

	rv := ranges.Arranged()
	return rv.Strings(), rv.Capacity(), nil
}

// CleanupCidr -- cleanup cidr from spaces and validate
func CleanupCidr(cidr string) (string, *net.IPNet, error) {
	cidrS := strings.TrimSpace(cidr)
	_, cidrN, err := net.ParseCIDR(cidrS)
	return cidrS, cidrN, err
}

// ----------------------------------------------------------------------------
