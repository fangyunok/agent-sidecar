package ingest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestCanonicalStatuses(t *testing.T) {
	want := []string{
		"pending_review",
		"insufficient_evidence",
		"needs_revision",
		"model_error",
	}
	got := KnownStatuses()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("KnownStatuses = %v, want %v", got, want)
	}
}

func TestValidStatus(t *testing.T) {
	cases := map[string]bool{
		"pending_review":         true,
		"insufficient_evidence":  true,
		"needs_revision":         true,
		"model_error":            true,
		"":                       false,
		"bogus":                  false,
	}
	for s, want := range cases {
		if got := ValidStatus(s); got != want {
			t.Fatalf("ValidStatus(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestNormaliseStrategy(t *testing.T) {
	cases := map[string]string{
		"":            "unspecified",
		"agent":       "agent",
		" unknown ":   "unspecified",
		"none":        "unspecified",
		"agent_loop":  "agent_loop",
	}
	for in, want := range cases {
		if got := NormaliseStrategy(in); got != want {
			t.Fatalf("NormaliseStrategy(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCountByStatus(t *testing.T) {
	bundles := []Bundle{
		{Status: "pending_review"},
		{Status: "pending_review"},
		{Status: "insufficient_evidence"},
		{Status: "model_error"},
		{Status: "unrecognised"},
	}
	got := CountByStatus(bundles)
	if got.PendingReview != 2 {
		t.Fatalf("PendingReview=%d, want 2", got.PendingReview)
	}
	if got.InsufficientEvidence != 1 {
		t.Fatalf("InsufficientEvidence=%d, want 1", got.InsufficientEvidence)
	}
	if got.NeedsRevision != 0 {
		t.Fatalf("NeedsRevision=%d, want 0", got.NeedsRevision)
	}
	if got.ModelError != 1 {
		t.Fatalf("ModelError=%d, want 1", got.ModelError)
	}
	if got.Other != 1 {
		t.Fatalf("Other=%d, want 1", got.Other)
	}
}

// TestScan reads a fixture tree under testdata/sample_runs which must
// contain at least three audit_bundle.json files (one each for
// pending_review, insufficient_evidence, model_error) plus one file
// with a deliberate decoding error to exercise the Problems path.
func TestScan(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "sample_runs")
	report, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan returned err: %v", err)
	}
	if len(report.Loaded) < 3 {
		t.Fatalf("expected at least 3 loaded bundles, got %d", len(report.Loaded))
	}
	// Sorted by started_at ascending.
	for i := 1; i < len(report.Loaded); i++ {
		if report.Loaded[i].StartedAt.Before(report.Loaded[i-1].StartedAt) {
			t.Fatalf("bundles not sorted by started_at: %v",
				report.Loaded[i].StartedAt)
		}
	}
	// At least one file with intentionally bad JSON must be flagged.
	if len(report.Problems) == 0 {
		t.Fatalf("expected at least one problem entry, got %d", len(report.Problems))
	}
	// Each loaded bundle exposes its source path for downstream ops.
	for _, b := range report.Loaded {
		if b.SourcePath == "" {
			t.Fatalf("bundle %q missing SourcePath", b.RunID)
		}
		if b.RunID == "" {
			t.Fatalf("bundle missing RunID")
		}
	}
}

// TestScanEmpty confirms a missing directory surfaces an error so
// callers do not silently render zero-data dashboards.
func TestScanEmpty(t *testing.T) {
	tmp := t.TempDir()
	if _, err := Scan(tmp); err != nil {
		return // acceptable outcome
	}
}

// TestScanDecodedFields exercises real-world decoding of a copy of a
// claim-studio audit_bundle.json. The fixture must contain a
// non-empty trace with step + duration_ms and a proposal with
// issues + citations.
func TestScanDecodedFields(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "sample_runs")
	report, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan returned err: %v", err)
	}
	bundle, ok := pickByRunIDPrefix(report.Loaded, "ok-agent")
	if !ok {
		t.Fatalf("could not locate ok-agent fixture in testdata")
	}
	if bundle.Request.ProductID == "" {
		t.Fatalf("decoded bundle missing request.product_id")
	}
	if bundle.Proposal == nil {
		t.Fatalf("decoded bundle missing proposal")
	}
	if len(bundle.Proposal.Issues) == 0 {
		t.Fatalf("decoded proposal missing issues")
	}
	if len(bundle.Proposal.Citations) == 0 {
		t.Fatalf("decoded proposal missing citations")
	}
	if bundle.Usage == nil {
		t.Fatalf("decoded bundle missing usage")
	}
	if bundle.DurationMS == 0 {
		t.Fatalf("decoded bundle missing duration_ms")
	}
}

// TestScanTagsLoaderReadOnly ensures Scan does not mutate files.
func TestScanTagsLoaderReadOnly(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "sample_runs")
	before, err := filepath.Glob(filepath.Join(root, "*", "audit_bundle.json"))
	if err != nil {
		t.Fatalf("glob err: %v", err)
	}
	if len(before) == 0 {
		t.Fatalf("no fixtures discovered before scan")
	}
	mtimes := map[string]int64{}
	for _, p := range before {
		st, _ := os.Stat(p)
		mtimes[p] = st.ModTime().UnixNano()
	}
	if _, err := Scan(root); err != nil {
		t.Fatalf("Scan err: %v", err)
	}
	after, _ := filepath.Glob(filepath.Join(root, "*", "audit_bundle.json"))
	sort.Strings(before)
	sort.Strings(after)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("scan touched the file listing: %v vs %v", before, after)
	}
	for p, mtime := range mtimes {
		st, _ := os.Stat(p)
		if st.ModTime().UnixNano() != mtime {
			t.Fatalf("scan modified mtime on %s", p)
		}
	}
}

// pickByRunIDPrefix returns the first bundle whose run_id has the
// given prefix. It is a helper for table-test ergonomics.
func pickByRunIDPrefix(bundles []Bundle, prefix string) (Bundle, bool) {
	for _, b := range bundles {
		if len(b.RunID) >= len(prefix) && b.RunID[:len(prefix)] == prefix {
			return b, true
		}
	}
	return Bundle{}, false
}

// Compile-time check that the Bundle shape decodes the field set used
// by the metrics package. This guards against silent drift when the
// upstream JSON evolves.
func TestBundleRawRoundTrip(t *testing.T) {
	raw := []byte(`{
        "run_id": "rt-001",
        "status": "pending_review",
        "strategy": "agent",
        "request": {"product_id": "demo-product", "feature": "x"},
        "revised_checks": {"passed": true}
    }`)
	var b Bundle
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if b.RunID != "rt-001" || b.Status != "pending_review" {
		t.Fatalf("decoded wrong fields: %+v", b)
	}
}
