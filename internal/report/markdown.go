// Package report renders a metrics.Summary as Markdown or JSON.
// Markdown is the primary human-facing surface; JSON mirrors the
// same struct so downstream tooling (CI bots, Grafana JSON
// datasources) can consume the same output without a second mode.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"fangyunok/agent-sidecar/internal/ingest"
	"fangyunok/agent-sidecar/internal/metrics"
)

// Options controls how a report is rendered.
type Options struct {
	Title    string
	JSON     bool
	Footer   string
	TimeZone *time.Location
}

// WriteMarkdown renders a markdown snapshot of the scan + summary to w.
func WriteMarkdown(w io.Writer, scan ingest.ScanReport, summary metrics.Summary, opts Options) error {
	if opts.Title == "" {
		opts.Title = "agent-sidecar · audit aggregation"
	}
	tz := time.UTC
	if opts.TimeZone != nil {
		tz = opts.TimeZone
	}
	now := time.Now().In(tz).Format(time.RFC3339)
	fmt.Fprintf(w, "# %s\n\n", opts.Title)
	fmt.Fprintf(w, "_generated %s · root=`%s`_\n\n", now, scan.RootPath)
	writeSnapshot(w, summary)
	writeLatency(w, summary)
	writeTokens(w, summary)
	writeCitations(w, summary)
	writeIssues(w, summary)
	writeCorpus(w, scan, summary)
	if opts.Footer != "" {
		fmt.Fprintf(w, "\n> %s\n", opts.Footer)
	}
	return nil
}

// WriteJSON encodes scan + summary as a single JSON document.
func WriteJSON(w io.Writer, scan ingest.ScanReport, summary metrics.Summary) error {
	payload := struct {
		Scan    ingest.ScanReport `json:"scan"`
		Summary metrics.Summary   `json:"summary"`
		Header  reportHeader      `json:"header"`
	}{scan, summary, reportHeader{SchemaVersion: schemaVersion, Tool: "agent-sidecar"}}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

const schemaVersion = "v0.1.0"

type reportHeader struct {
	Tool          string `json:"tool"`
	SchemaVersion string `json:"schema_version"`
}

func writeSnapshot(w io.Writer, s metrics.Summary) {
	fmt.Fprintln(w, "## Snapshot")
	fmt.Fprintf(w, "- Runs scanned: **%d**\n", s.RunCount)
	if s.RunCount == 0 {
		fmt.Fprintln(w, "- (no audit bundles found)")
		return
	}
	counts := []string{
		fmt.Sprintf("`pending_review`=%d", s.StatusCounts.PendingReview),
		fmt.Sprintf("`insufficient_evidence`=%d", s.StatusCounts.InsufficientEvidence),
		fmt.Sprintf("`needs_revision`=%d", s.StatusCounts.NeedsRevision),
		fmt.Sprintf("`model_error`=%d", s.StatusCounts.ModelError),
	}
	if s.StatusCounts.Other > 0 {
		counts = append(counts, fmt.Sprintf("`other`=%d", s.StatusCounts.Other))
	}
	fmt.Fprintf(w, "- Status mix: %s\n", strings.Join(counts, " · "))
	fmt.Fprintf(w, "- Pass rate: **%.1f%%** (%d reviewed / %d total)\n",
		s.PassRate*100, s.ReviewedCount, s.RunCount)
	if !s.TimeRange.Start.IsZero() {
		fmt.Fprintf(w, "- Span: `%s` → `%s` (%s)\n",
			s.TimeRange.Start.Format(time.RFC3339),
			s.TimeRange.End.Format(time.RFC3339),
			formatSpan(s.TimeRange.SpanMS),
		)
	}
	fmt.Fprintln(w)
}

func writeLatency(w io.Writer, s metrics.Summary) {
	fmt.Fprintln(w, "## Latency")
	fmt.Fprintln(w, "### Whole-run wall-clock")
	writeLatencyTable(w, s.RunLatency)
	if s.StepLatency.Count > 0 {
		fmt.Fprintln(w, "\n### Trace step latency")
		writeLatencyTable(w, s.StepLatency)
	}
	fmt.Fprintln(w)
}

func writeLatencyTable(w io.Writer, stats metrics.LatencyStats) {
	if stats.Count == 0 {
		fmt.Fprintln(w, "_no samples_")
		return
	}
	rows := [][]string{
		{"count", fmt.Sprintf("%d", stats.Count)},
		{"min", fmt.Sprintf("%d ms", stats.MinMS)},
		{"P50", fmt.Sprintf("%d ms", stats.P50MS)},
		{"P90", fmt.Sprintf("%d ms", stats.P90MS)},
		{"P99", fmt.Sprintf("%d ms", stats.P99MS)},
		{"max", fmt.Sprintf("%d ms", stats.MaxMS)},
		{"mean", fmt.Sprintf("%.1f ms", stats.MeanMS)},
	}
	for _, r := range rows {
		fmt.Fprintf(w, "- %s: %s\n", r[0], r[1])
	}
}

func writeTokens(w io.Writer, s metrics.Summary) {
	fmt.Fprintln(w, "## Token usage")
	if s.Tokens.RunCount == 0 {
		fmt.Fprintln(w, "_no usage samples (bundles did not record token usage)_")
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintf(w, "- Runs with usage: **%d**\n", s.Tokens.RunCount)
	fmt.Fprintf(w, "- Input:  Σ=%d · mean=%.1f · P50=%d · P99=%d\n",
		s.Tokens.SumInput, s.Tokens.MeanInput, s.Tokens.P50Input, s.Tokens.P99Input)
	fmt.Fprintf(w, "- Output: Σ=%d · mean=%.1f · P50=%d · P99=%d\n",
		s.Tokens.SumOutput, s.Tokens.MeanOutput, s.Tokens.P50Output, s.Tokens.P99Output)
	fmt.Fprintln(w)
}

func writeCitations(w io.Writer, s metrics.Summary) {
	fmt.Fprintln(w, "## Citations per run")
	writeCountTable(w, s.CitationsPerRun)
	fmt.Fprintln(w)
}

func writeIssues(w io.Writer, s metrics.Summary) {
	fmt.Fprintln(w, "## Issues per run")
	writeCountTable(w, s.IssuesPerRun)
	if len(s.IssueKindCounts) > 0 {
		fmt.Fprintln(w, "\n### Issue-kind breakdown")
		keys := make([]string, 0, len(s.IssueKindCounts))
		for k := range s.IssueKindCounts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(w, "- `%s`: %d\n", k, s.IssueKindCounts[k])
		}
	}
	fmt.Fprintln(w)
}

func writeCountTable(w io.Writer, c metrics.CountStats) {
	if c.Count == 0 {
		fmt.Fprintln(w, "_no samples_")
		return
	}
	fmt.Fprintf(w, "- count: %d\n", c.Count)
	fmt.Fprintf(w, "- mean:  %.2f\n", c.Mean)
	fmt.Fprintf(w, "- median: %d\n", c.Median)
	fmt.Fprintf(w, "- P90:   %d\n", c.P90)
	fmt.Fprintf(w, "- max:   %d\n", c.Max)
}

func writeCorpus(w io.Writer, scan ingest.ScanReport, s metrics.Summary) {
	fmt.Fprintln(w, "## Corpus")
	if len(s.Strategies) > 0 {
		fmt.Fprintf(w, "- Strategies: %s\n", strings.Join(s.Strategies, " · "))
	}
	if len(s.Models) > 0 {
		fmt.Fprintf(w, "- Models: %s\n", strings.Join(s.Models, " · "))
	}
	if len(s.Products) > 0 {
		fmt.Fprintf(w, "- Products: %s\n", strings.Join(s.Products, " · "))
	}
	if len(scan.Problems) > 0 {
		fmt.Fprintln(w, "\n### Ingest problems")
		for _, p := range scan.Problems {
			fmt.Fprintf(w, "- `%s`: %s\n", p.Path, p.Reason)
		}
	}
	fmt.Fprintln(w)
}

func formatSpan(ms int64) string {
	if ms <= 0 {
		return "0s"
	}
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	if ms < 60_000 {
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	}
	if ms < 3_600_000 {
		return fmt.Sprintf("%.1fmin", float64(ms)/60_000)
	}
	if ms < 86_400_000 {
		return fmt.Sprintf("%.1fh", float64(ms)/3_600_000)
	}
	return fmt.Sprintf("%.1fd", float64(ms)/86_400_000)
}
