package ipallocator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateRangesFromCIDR(t *testing.T) {
	actual, _, _ := CalculateRanges("172.22.132.0/24", []string{}, true)
	assert.Equal(t,
		[]string{"172.22.132.0-172.22.132.255"},
		actual,
	)
	actual, _, _ = CalculateRanges("172.22.132.0/24", []string{}, false)
	assert.Equal(t,
		[]string{"172.22.132.1-172.22.132.254"},
		actual,
	)
}

func TestCalculateRangesFromInRanges(t *testing.T) {
	actual, _, _ := CalculateRanges("172.22.132.0/24", []string{
		"172.22.132.0-172.22.132.10",
		"172.22.132.20-172.22.132.30",
		"172.22.132.200-172.22.132.255",
	}, true)
	assert.Equal(t,
		[]string{
			"172.22.132.0-172.22.132.10",
			"172.22.132.20-172.22.132.30",
			"172.22.132.200-172.22.132.255",
		},
		actual,
	)
	actual, _, _ = CalculateRanges("172.22.132.0/24", []string{
		"172.22.132.0-172.22.132.10",
		"172.22.132.20-172.22.132.30",
		"172.22.132.200-172.22.132.255",
	}, false)
	assert.Equal(t,
		[]string{
			"172.22.132.1-172.22.132.10",
			"172.22.132.20-172.22.132.30",
			"172.22.132.200-172.22.132.254",
		},
		actual,
	)
	actual, _, _ = CalculateRanges("172.22.132.16/28", []string{
		"172.22.132.0-172.22.132.20",
		"172.22.132.25-172.22.132.36",
	}, true)
	assert.Equal(t,
		[]string{
			"172.22.132.16-172.22.132.20",
			"172.22.132.25-172.22.132.31",
		},
		actual,
	)
}

func TestCalculateRangesFromInRangesWithCutEdges(t *testing.T) {
	actual, _, _ := CalculateRanges("172.22.132.0/24", []string{
		"172.22.132.0-172.22.132.10",
		"172.22.132.20-172.22.132.30",
		"172.22.132.200-172.22.132.255",
	}, false)
	assert.Equal(t,
		[]string{
			"172.22.132.1-172.22.132.10",
			"172.22.132.20-172.22.132.30",
			"172.22.132.200-172.22.132.254",
		},
		actual,
	)
	actual, _, _ = CalculateRanges("172.22.132.0/24", []string{
		"172.22.132.0-172.22.132.10",
		"172.22.132.20-172.22.132.30",
		"172.22.132.200-172.22.132.255",
	}, true)
	assert.Equal(t,
		[]string{
			"172.22.132.0-172.22.132.10",
			"172.22.132.20-172.22.132.30",
			"172.22.132.200-172.22.132.255",
		},
		actual,
	)
}

func TestCalculateRangesWithExcludes(t *testing.T) {
	cidr := "172.22.132.0/24"
	includeRanges := []string{
		"172.22.132.10-172.22.132.30",
		"172.22.132.40-172.22.132.60",
		"172.22.132.70-172.22.132.90",
		"172.22.132.162-172.22.132.164",
		"172.22.132.200-172.22.132.255",
	}
	excludeRanges1 := []string{
		"172.22.132.5",
		"172.22.132.15-172.22.132.20",
		"172.22.132.28-172.22.132.42",
		"172.22.132.110-172.22.132.150",
	}
	excludeRanges2 := []string{
		"8.8.8.8",
		"8.8.4.4",
		"172.22.132.160-172.22.132.170",
	}
	excludeRanges3 := []string{
		"172.22.132.87-172.22.132.88",
	}
	partialExpected := []string{
		"172.22.132.10-172.22.132.14",
		"172.22.132.21-172.22.132.27",
		"172.22.132.43-172.22.132.60",
		"172.22.132.70-172.22.132.86",
		"172.22.132.89-172.22.132.90",
	}
	// whole network enabled
	actual, _, _ := CalculateRanges(cidr, includeRanges, true, excludeRanges1, excludeRanges2, excludeRanges3)
	assert.Equal(t,
		append(partialExpected, "172.22.132.200-172.22.132.255"),
		actual,
	)
	// whole network disabled
	actual, _, _ = CalculateRanges(cidr, includeRanges, false, excludeRanges1, excludeRanges2, excludeRanges3)
	assert.Equal(t,
		append(partialExpected, "172.22.132.200-172.22.132.254"),
		actual,
	)
}
