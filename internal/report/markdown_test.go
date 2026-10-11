package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"fangyunok/agent-sidecar/internal/ingest"
	"fangyunok/agent-sidecar/internal/metrics"
)

func TestMarkdownContainsSnapshot(t *testing.T) {
	scan := ingest.ScanReport{RootPath: "/tmp/anywhere"}
	summary := metrics.Summary{RunCount: 0}
	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, scan, summary, Options{Title: "demo"}); err != nil {
		t.Fatalf("WriteMarkdown returned err: %v", err)
	}
	rendered := buf.String()
	if !strings.Contains(rendered, "# demo") {
		t.Fatalf("title missing:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Runs scanned") {
		t.Fatalf("snapshot header missing:\n%s", rendered)
	}
	if !strings.Contains(rendered, "(no audit bundles found)") {
		t.Fatalf("empty branch missing:\n%s", rendered)
	}
}

func TestMarkdownEmptyIsDeterministic(t *testing.T) {
	scan := ingest.ScanReport{RootPath: "/tmp/anywhere"}
	summary := metrics.Summary{}
	var a, b bytes.Buffer
	if err := WriteMarkdown(&a, scan, summary, Options{Title: "demo"}); err != nil {
		t.Fatalf("WriteMarkdown a err: %v", err)
	}
	if err := WriteMarkdown(&b, scan, summary, Options{Title: "demo"}); err != nil {
		t.Fatalf("WriteMarkdown b err: %v", err)
	}
	// Time stamps differ by second inside the same minute, so just
	// confirm the structural prefix (everything before the timestamp)
	// matches.
	prefix := func(s string) string {
		if idx := strings.Index(s, "generated "); idx >= 0 {
			end := strings.Index(s[idx:], " ·")
			if end >= 0 {
				return s[:idx+end]
			}
		}
		return s
	}
	if prefix(a.String()) != prefix(b.String()) {
		t.Fatalf("structural prefix diverges:\nA=%q\nB=%q",
			prefix(a.String()), prefix(b.String()))
	}
}

func TestJSONOutputHasTool(t *testing.T) {
	scan := ingest.ScanReport{RootPath: "/tmp/x"}
	summary := metrics.Summary{RunCount: 1, StatusCounts: ingest.StatusCounts{PendingReview: 1}}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, scan, summary); err != nil {
		t.Fatalf("WriteJSON err: %v", err)
	}
	var decoded struct {
		Header  reportHeader      `json:"header"`
		Summary metrics.Summary   `json:"summary"`
		Scan    ingest.ScanReport `json:"scan"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("decode err: %v\nraw:\n%s", err, buf.String())
	}
	if decoded.Header.Tool != "agent-sidecar" {
		t.Fatalf("Header.Tool=%q, want agent-sidecar", decoded.Header.Tool)
	}
	if decoded.Header.SchemaVersion == "" {
		t.Fatalf("Header.SchemaVersion must not be empty")
	}
	if decoded.Summary.RunCount != 1 {
		t.Fatalf("RunCount round-trip lost: %d", decoded.Summary.RunCount)
	}
	if decoded.Scan.RootPath != "/tmp/x" {
		t.Fatalf("RootPath lost: %q", decoded.Scan.RootPath)
	}
}

func TestJSONOutputIndentStable(t *testing.T) {
	// Indented output is a contract for downstream tools; check the
	// leading indent matches what we promise.
	var buf bytes.Buffer
	if err := WriteJSON(&buf, ingest.ScanReport{}, metrics.Summary{}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.HasPrefix(buf.String(), "{\n  \"header\"") {
		t.Fatalf("unexpected leading bytes: %q", buf.String()[:32])
	}
}

func TestFormatSpan(t *testing.T) {
	cases := map[int64]string{
		0:        "0s",
		500:      "500ms",
		1500:     "1.5s",
		90_000:   "1.5min",
		3_600_000: "1.0h",
		86_400_000: "1.0d",
	}
	for in, want := range cases {
		if got := formatSpan(in); got != want {
			t.Fatalf("formatSpan(%d)=%q, want %q", in, got, want)
		}
	}
}

func TestWriteMarkdownTimezone(t *testing.T) {
	tz, err := time.LoadLocation("UTC")
	if err != nil {
		t.Fatalf("load tz: %v", err)
	}
	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, ingest.ScanReport{RootPath: "/tmp/anywhere"}, metrics.Summary{}, Options{TimeZone: tz}); err != nil {
		t.Fatalf("WriteMarkdown err: %v", err)
	}
	if !strings.Contains(buf.String(), " · root=`/tmp/anywhere`") {
		t.Fatalf("root missing:\n%s", buf.String())
	}
}
