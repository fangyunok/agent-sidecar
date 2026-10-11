package metrics

import (
	"math"
	"sort"
)

// LatencyStats groups latency observations together with summary
// percentile metrics. Percentiles use the nearest-rank rule on the
// sorted sample set so the result is monotone in the input.
type LatencyStats struct {
	Count  int     `json:"count"`
	SumMS  int     `json:"sum_ms"`
	MeanMS float64 `json:"mean_ms"`
	P50MS  int     `json:"p50_ms"`
	P90MS  int     `json:"p90_ms"`
	P99MS  int     `json:"p99_ms"`
	MinMS  int     `json:"min_ms"`
	MaxMS  int     `json:"max_ms"`
}

// CountStats groups discrete counts (citations, issues) into a small
// descriptive summary that is useful for spotting run-by-run variance
// without pulling a histogram into the report.
type CountStats struct {
	Count  int     `json:"count"`
	Sum    int     `json:"sum"`
	Mean   float64 `json:"mean"`
	Median int     `json:"median"`
	P90    int     `json:"p90"`
	Max    int     `json:"max"`
}

// TokenStats groups per-run token observations.
type TokenStats struct {
	RunCount   int     `json:"run_count"` // bundles with non-nil usage
	SumInput   int     `json:"sum_input"`
	SumOutput  int     `json:"sum_output"`
	MeanInput  float64 `json:"mean_input"`
	MeanOutput float64 `json:"mean_output"`
	P50Input   int     `json:"p50_input"`
	P50Output  int     `json:"p50_output"`
	P99Input   int     `json:"p99_input"`
	P99Output  int     `json:"p99_output"`
}

// SummariseLatency builds a LatencyStats from raw millisecond
// observations. Empty input yields a zero-valued struct so callers
// can render "no data" branches without nil-checks.
func SummariseLatency(samples []int) LatencyStats {
	if len(samples) == 0 {
		return LatencyStats{}
	}
	sorted := append([]int(nil), samples...)
	sort.Ints(sorted)
	var sum int
	for _, v := range sorted {
		sum += v
	}
	return LatencyStats{
		Count:  len(sorted),
		SumMS:  sum,
		MeanMS: float64(sum) / float64(len(sorted)),
		P50MS:  Percentile(sorted, 0.50),
		P90MS:  Percentile(sorted, 0.90),
		P99MS:  Percentile(sorted, 0.99),
		MinMS:  sorted[0],
		MaxMS:  sorted[len(sorted)-1],
	}
}

// SummariseCounts builds a CountStats from raw observation values.
// Used for citation counts, issue counts, etc.
func SummariseCounts(samples []int) CountStats {
	if len(samples) == 0 {
		return CountStats{}
	}
	sorted := append([]int(nil), samples...)
	sort.Ints(sorted)
	var sum int
	for _, v := range sorted {
		sum += v
	}
	return CountStats{
		Count:  len(sorted),
		Sum:    sum,
		Mean:   float64(sum) / float64(len(sorted)),
		Median: Percentile(sorted, 0.50),
		P90:    Percentile(sorted, 0.90),
		Max:    sorted[len(sorted)-1],
	}
}

// Percentile returns the nearest-rank percentile from values. The
// input is copied and sorted, so the caller does not need to pre-sort
// the slice it passes in.
//
// p is given as a fraction in [0,1] (e.g. 0.99 for P99). Out-of-range
// p clamps to 1.0 (the maximum sample).
//
// The rank is computed via ceil(p * N) - 1 in 0-indexed coordinates,
// which gives the canonical nearest-rank behaviour:
//
//	N=2 p=0.5 -> ceil(1) - 1 = 0 -> sorted[0] (lower of the pair)
//	N=8 p=0.5 -> ceil(4) - 1 = 3 -> sorted[3] (lower-middle of the 8)
//
// Empty input returns 0 so callers can render "no data" branches
// without nil-checks.
func Percentile(values []int, p float64) int {
	if len(values) == 0 {
		return 0
	}
	if p < 0 {
		p = 0
	}
	if p > 1 {
		p = 1
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	n := len(sorted)
	// Ceil(p * N). Use math.Ceil to keep the function honest about
	// "nearest rank" rather than "0-indexed linear interpolation".
	rank := int(math.Ceil(p*float64(n))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= n {
		rank = n - 1
	}
	return sorted[rank]
}
