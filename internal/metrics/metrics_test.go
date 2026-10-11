package metrics

import (
	"math"
	"reflect"
	"sort"
	"testing"
	"time"

	"fangyunok/agent-sidecar/internal/ingest"
)

func TestPercentileEmpty(t *testing.T) {
	if got := Percentile(nil, 0.5); got != 0 {
		t.Fatalf("empty input should return 0, got %d", got)
	}
}

func TestPercentileNearestRank(t *testing.T) {
	cases := []struct {
		name   string
		values []int
		p      float64
		want   int
	}{
		{"single", []int{42}, 0.99, 42},
		{"two-p50", []int{1, 100}, 0.5, 1},
		{"two-p90", []int{1, 100}, 0.9, 100},
		{"even-p50", []int{1, 2, 3, 4, 5, 6, 7, 8}, 0.5, 4},
		{"even-p99", []int{1, 2, 3, 4, 5, 6, 7, 8}, 0.99, 8},
		{"unsorted-p90", []int{100, 1, 50, 25}, 0.9, 100},
		{"underflow", []int{1, 2, 3}, 0, 1},
		{"overflow-clamps", []int{1, 2, 3}, 1.5, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Percentile(c.values, c.p); got != c.want {
				t.Fatalf("Percentile(%v, %v) = %d, want %d", c.values, c.p, got, c.want)
			}
		})
	}
}

func TestSummariseLatencyEmpty(t *testing.T) {
	got := SummariseLatency(nil)
	if got.Count != 0 || got.MeanMS != 0 || got.P50MS != 0 || got.P99MS != 0 {
		t.Fatalf("expected zero value, got %+v", got)
	}
}

func TestSummariseLatencySingletons(t *testing.T) {
	stats := SummariseLatency([]int{500})
	if stats.Count != 1 || stats.P50MS != 500 || stats.P90MS != 500 || stats.P99MS != 500 {
		t.Fatalf("singleton should equal itself, got %+v", stats)
	}
	if math.Abs(stats.MeanMS-500) > 1e-6 {
		t.Fatalf("MeanMS=%v, want 500", stats.MeanMS)
	}
}

func TestSummariseCountsEmpty(t *testing.T) {
	got := SummariseCounts(nil)
	if got.Count != 0 || got.Mean != 0 {
		t.Fatalf("expected zero value, got %+v", got)
	}
}

func TestSummariseDeterministicOnSameInput(t *testing.T) {
	bundles := makeBundlesForSummarise()
	left := Summarise(bundles)
	right := Summarise(bundles)
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("Summarise is non-deterministic: %+v vs %+v", left, right)
	}
}

func TestSummariseBasicFields(t *testing.T) {
	bundles := makeBundlesForSummarise()
	got := Summarise(bundles)
	if got.RunCount != 3 {
		t.Fatalf("RunCount=%d, want 3", got.RunCount)
	}
	if got.StatusCounts.PendingReview != 1 {
		t.Fatalf("PendingReview=%d, want 1", got.StatusCounts.PendingReview)
	}
	if got.StatusCounts.InsufficientEvidence != 1 {
		t.Fatalf("InsufficientEvidence=%d, want 1", got.StatusCounts.InsufficientEvidence)
	}
	if got.StatusCounts.ModelError != 1 {
		t.Fatalf("ModelError=%d, want 1", got.StatusCounts.ModelError)
	}
	if got.StatusCounts.Other != 0 {
		t.Fatalf("Other=%d, want 0", got.StatusCounts.Other)
	}
	// Pass-rate: bundle 0 (passed=true), bundle 1 (passed=false),
	// bundle 2 (no proposal -> not counted as passed).
	if math.Abs(got.PassRate-1.0/3.0) > 1e-6 {
		t.Fatalf("PassRate=%v, want ~0.3333", got.PassRate)
	}
	if got.RunLatency.Count != 3 || got.RunLatency.P50MS == 0 {
		t.Fatalf("RunLatency unexpected: %+v", got.RunLatency)
	}
	if got.StepLatency.Count == 0 {
		t.Fatalf("expected at least one trace step with duration_ms")
	}
	if got.Tokens.RunCount != 2 {
		t.Fatalf("Tokens.RunCount=%d, want 2 (bundles 0 and 1 carry usage)",
			got.Tokens.RunCount)
	}
	if got.Tokens.SumInput != 3000 {
		t.Fatalf("SumInput=%d, want 3000", got.Tokens.SumInput)
	}
	if got.Tokens.SumOutput != 800 {
		t.Fatalf("SumOutput=%d, want 800", got.Tokens.SumOutput)
	}
	if got.IssueKindCounts["unsupported"] != 2 {
		t.Fatalf("IssueKindCounts[unsupported]=%d, want 2",
			got.IssueKindCounts["unsupported"])
	}
	wantStrats := []string{"agent", "rag"}
	if !reflect.DeepEqual(got.Strategies, wantStrats) {
		t.Fatalf("Strategies=%v, want %v", got.Strategies, wantStrats)
	}
	if len(got.Products) != 2 {
		t.Fatalf("Products=%v, want 2 products", got.Products)
	}
	if got.TimeRange.Start.IsZero() || got.TimeRange.End.IsZero() {
		t.Fatalf("TimeRange must be set, got %+v", got.TimeRange)
	}
}

func TestSummariseEmpty(t *testing.T) {
	got := Summarise(nil)
	if got.RunCount != 0 || got.PassRate != 0 {
		t.Fatalf("empty summarisation should be all-zero, got %+v", got)
	}
	if got.IssueKindCounts == nil {
		t.Fatalf("IssueKindCounts must be initialised so JSON omitempty works")
	}
}

func TestSummariseTokensEmpty(t *testing.T) {
	got := summariseTokens(nil, nil)
	if got.RunCount != 0 || got.SumInput != 0 || got.SumOutput != 0 {
		t.Fatalf("expected zero token summary, got %+v", got)
	}
}

func TestSummariseTokensOneSidedRejected(t *testing.T) {
	// If only input tokens were observed, we still want the summary
	// to refuse to lie about the output side, so it stays at zero.
	got := summariseTokens([]int{100, 200}, nil)
	if got.RunCount != 0 {
		t.Fatalf("RunCount=%d, want 0 (single-sided samples rejected)", got.RunCount)
	}
}

func TestRound2(t *testing.T) {
	cases := map[float64]float64{
		0:              0,
		1.234:          1.23,
		-1.236:         -1.24,
		math.NaN():     0,
		math.Inf(1):    0,
		math.Inf(-1):   0,
	}
	for in, want := range cases {
		if got := round2(in); got != want {
			t.Fatalf("round2(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestKeysSorted(t *testing.T) {
	set := map[string]struct{}{"b": {}, "a": {}, "c": {}}
	got := keys(set)
	sorted := append([]string(nil), got...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(got, sorted) {
		t.Fatalf("keys not sorted: %v", got)
	}
}

// makeBundlesForSummarise builds a small, deterministic set of
// bundles used by the summarisation tests above. Durations are
// hand-picked so P50 / P90 line up with predictable integers without
// pulling in additional float rounding logic.
func makeBundlesForSummarise() []ingest.Bundle {
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	b := []ingest.Bundle{
		{
			RunID:      "ok-agent",
			Status:     "pending_review",
			Strategy:   "agent",
			Model:      "qwen3:4b-instruct",
			StartedAt:  base,
			DurationMS: 5000,
			Reviewed:   true,
			Request:    ingest.BundleRequest{ProductID: "demo-product", Feature: "f1"},
			Proposal: &ingest.BundleProposal{
				Citations: []ingest.BundleCitation{{FactID: "f-1"}, {FactID: "f-2"}},
				Issues:    []ingest.BundleIssue{{Kind: "unsupported"}, {Kind: "overstated"}},
			},
			RevisedChecks: ingest.BundleChecks{Passed: true},
			Usage: &ingest.BundleUsage{
				InputTokens: 1000, OutputTokens: 300,
			},
			Trace: []ingest.BundleStep{
				{Step: "model_response", DurationMS: intPtr(1500)},
				{Step: "proposal_checked", DurationMS: intPtr(400)},
			},
		},
		{
			RunID:      "insufficient-rag",
			Status:     "insufficient_evidence",
			Strategy:   "rag",
			Model:      "qwen3:14b-instruct",
			StartedAt:  base.Add(time.Minute),
			DurationMS: 12000,
			Reviewed:   false,
			Request:    ingest.BundleRequest{ProductID: "demo-product", Feature: "f2"},
			RevisedChecks: ingest.BundleChecks{Passed: false},
			Usage: &ingest.BundleUsage{
				InputTokens: 2000, OutputTokens: 500,
			},
			Trace: []ingest.BundleStep{
				{Step: "model_response", DurationMS: intPtr(3000)},
				{Step: "model_response", DurationMS: intPtr(6000)},
			},
		},
		{
			RunID:     "model-error",
			Status:    "model_error",
			Strategy:  "agent",
			StartedAt: base.Add(2 * time.Minute),
			DurationMS: 8000,
			Request:   ingest.BundleRequest{ProductID: "demo-other", Feature: "f3"},
			// No proposal, no usage, no trace -> exercises the
			// nil-safe branches.
		},
	}
	return b
}

func intPtr(v int) *int { return &v }
