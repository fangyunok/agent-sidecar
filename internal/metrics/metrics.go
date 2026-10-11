package metrics

import (
	"math"
	"sort"
	"time"

	"fangyunok/agent-sidecar/internal/ingest"
)

// Summary is the per-aggregation snapshot the sidecar outputs. Every
// field is deterministic so two runs against the same input produce
// the same bytes.
type Summary struct {
	RunCount        int                    `json:"run_count"`
	StatusCounts    ingest.StatusCounts    `json:"status_counts"`
	PassRate        float64                `json:"pass_rate"`
	ReviewedCount   int                    `json:"reviewed_count"`
	RunLatency      LatencyStats           `json:"run_latency"`
	StepLatency     LatencyStats           `json:"step_latency"`
	Tokens          TokenStats             `json:"tokens"`
	CitationsPerRun CountStats             `json:"citations_per_run"`
	IssuesPerRun    CountStats             `json:"issues_per_run"`
	IssueKindCounts map[string]int         `json:"issue_kind_counts"`
	Strategies      []string               `json:"strategies"`
	Products        []string               `json:"products"`
	Models          []string               `json:"models"`
	TimeRange       TimeRange              `json:"time_range"`
}

// TimeRange records the spread of run timestamps.
type TimeRange struct {
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	SpanMS int64     `json:"span_ms"`
}

// Summarise folds every bundle in a scan into the deterministic
// summary the markdown / JSON reporter renders.
func Summarise(bundles []ingest.Bundle) Summary {
	out := Summary{
		IssueKindCounts: make(map[string]int),
	}
	out.RunCount = len(bundles)
	out.StatusCounts = ingest.CountByStatus(bundles)
	if out.RunCount == 0 {
		return out
	}
	var runLatencySamples []int
	var stepLatencySamples []int
	var citationSamples []int
	var issueSamples []int
	var inputTokens []int
	var outputTokens []int
	strategies := make(map[string]struct{})
	products := make(map[string]struct{})
	models := make(map[string]struct{})
	startedAt := make([]time.Time, 0, len(bundles))
	var passed int
	for _, b := range bundles {
		if b.RevisedChecks.Passed {
			passed++
		}
		if b.Reviewed {
			out.ReviewedCount++
		}
		if b.DurationMS > 0 {
			runLatencySamples = append(runLatencySamples, b.DurationMS)
		}
		for _, step := range b.Trace {
			if step.DurationMS != nil && *step.DurationMS > 0 {
				stepLatencySamples = append(stepLatencySamples, *step.DurationMS)
			}
		}
		if b.Proposal != nil {
			citationSamples = append(citationSamples, len(b.Proposal.Citations))
			issueSamples = append(issueSamples, len(b.Proposal.Issues))
			for _, issue := range b.Proposal.Issues {
				out.IssueKindCounts[issue.Kind]++
			}
		}
		if b.Usage != nil {
			if b.Usage.InputTokens > 0 || b.Usage.OutputTokens > 0 {
				inputTokens = append(inputTokens, b.Usage.InputTokens)
				outputTokens = append(outputTokens, b.Usage.OutputTokens)
			}
		}
		if s := ingest.NormaliseStrategy(b.Strategy); s != "" {
			strategies[s] = struct{}{}
		}
		if b.Request.ProductID != "" {
			products[b.Request.ProductID] = struct{}{}
		}
		if b.Model != "" {
			models[b.Model] = struct{}{}
		}
		startedAt = append(startedAt, b.StartedAt)
	}
	out.RunLatency = SummariseLatency(runLatencySamples)
	out.StepLatency = SummariseLatency(stepLatencySamples)
	out.CitationsPerRun = SummariseCounts(citationSamples)
	out.IssuesPerRun = SummariseCounts(issueSamples)
	out.Tokens = summariseTokens(inputTokens, outputTokens)
	out.PassRate = float64(passed) / float64(out.RunCount)
	out.Strategies = keys(strategies)
	out.Products = keys(products)
	out.Models = keys(models)
	if len(startedAt) > 0 {
		sort.Slice(startedAt, func(i, j int) bool { return startedAt[i].Before(startedAt[j]) })
		out.TimeRange = TimeRange{
			Start:   startedAt[0],
			End:     startedAt[len(startedAt)-1],
			SpanMS:  startedAt[len(startedAt)-1].Sub(startedAt[0]).Milliseconds(),
		}
	}
	return out
}

// summariseTokens builds the TokenStats view. The function is package
// private to avoid leaking the input / output slice separation into
// the public API.
func summariseTokens(input, output []int) TokenStats {
	out := TokenStats{}
	if len(input) == 0 || len(output) == 0 {
		return out
	}
	out.RunCount = len(input)
	for _, v := range input {
		out.SumInput += v
	}
	for _, v := range output {
		out.SumOutput += v
	}
	out.MeanInput = round2(float64(out.SumInput) / float64(out.RunCount))
	out.MeanOutput = round2(float64(out.SumOutput) / float64(out.RunCount))
	out.P50Input = Percentile(input, 0.50)
	out.P50Output = Percentile(output, 0.50)
	out.P99Input = Percentile(input, 0.99)
	out.P99Output = Percentile(output, 0.99)
	return out
}

func round2(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*100) / 100
}

func keys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
