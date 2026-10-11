// Command aggregator is the v0.1 entrypoint for the agent-sidecar.
//
// Usage:
//
//	agent-sidecar --input <runs-dir> [--format=md|json] [--title <text>]
//
// Behaviour:
//
//  1. Walk <runs-dir> for audit_bundle.json files (claim-studio).
//  2. Summarise latency / token usage / citation coverage / issue mix.
//  3. Render to stdout as Markdown (default) or JSON.
//
// Stdlib-only. No external dependencies. The binary stays small
// enough to ship as a distroless container layer.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"fangyunok/agent-sidecar/internal/ingest"
	"fangyunok/agent-sidecar/internal/metrics"
	"fangyunok/agent-sidecar/internal/report"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		log.Fatalf("agent-sidecar: %v", err)
	}
}

// run is the testable entry point. The signature keeps main thin so
// go test ./cmd/aggregator can drive it without spawning a child
// process.
func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("aggregator", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "root directory containing claim-studio audit_bundle.json files")
	format := fs.String("format", "md", "render mode: md (default) or json")
	title := fs.String("title", "", "report title (markdown only)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *input == "" {
		return fmt.Errorf("--input is required (a directory of audit_bundle.json files)")
	}
	st, err := os.Stat(*input)
	if err != nil {
		return fmt.Errorf("stat --input: %w", err)
	}
	if !st.IsDir() {
		return fmt.Errorf("--input must be a directory, got %s", *input)
	}
	scan, err := ingest.Scan(*input)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	summary := metrics.Summarise(scan.Loaded)
	switch *format {
	case "md", "markdown":
		return report.WriteMarkdown(stdout, scan, summary, report.Options{Title: *title})
	case "json":
		return report.WriteJSON(stdout, scan, summary)
	default:
		return fmt.Errorf("unsupported --format=%s (md|json)", *format)
	}
}
