// Package ingest discovers and parses claim-studio audit_bundle.json
// files produced by the upstream Python audit pipeline.
//
// A scan walks the input directory recursively, materialises one Bundle
// per audit_bundle.json it finds, and returns a ScanReport that
// distinguishes loaded files from skipped / malformed ones so that a
// single corrupted artifact does not break the whole aggregation.
package ingest

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Bundle mirrors the audit_bundle.json contract emitted by
// claim-studio. Only the fields the sidecar actually consumes are
// decoded; everything else flows through Raw.
type Bundle struct {
	RunID         string          `json:"run_id"`
	Status        string          `json:"status"`
	Strategy      string          `json:"strategy"`
	Model         string          `json:"model"`
	PromptVersion string          `json:"prompt_version"`
	StartedAt     time.Time       `json:"started_at"`
	DurationMS    int             `json:"duration_ms"`
	Reviewed      bool            `json:"reviewed"`
	Request       BundleRequest   `json:"request"`
	Proposal      *BundleProposal `json:"proposal,omitempty"`
	RevisedChecks BundleChecks    `json:"revised_checks"`
	Usage         *BundleUsage    `json:"usage,omitempty"`
	Trace         []BundleStep    `json:"trace,omitempty"`
	SourcePath    string          `json:"-"`
	Raw           []byte          `json:"-"`
}

// BundleRequest is the user-provided audit brief.
type BundleRequest struct {
	ID           string `json:"id"`
	ProductID    string `json:"product_id"`
	ProductName  string `json:"product_name"`
	Locale       string `json:"locale"`
	Feature      string `json:"feature"`
	Channel      string `json:"channel"`
	OriginalCopy string `json:"original_copy"`
	Audience     string `json:"audience"`
}

// BundleProposal is the LLM-authored revised copy plus cited evidence.
type BundleProposal struct {
	RevisedCopy string             `json:"revised_copy"`
	Issues      []BundleIssue      `json:"issues"`
	Citations   []BundleCitation   `json:"citations"`
}

// BundleIssue is one flagged statement.
type BundleIssue struct {
	Quote    string   `json:"quote"`
	Kind     string   `json:"kind"`
	Reason   string   `json:"reason"`
	FactIDs  []string `json:"fact_ids"`
}

// BundleCitation pairs a piece of revised copy with a fact it cites.
type BundleCitation struct {
	Quote  string `json:"quote"`
	FactID string `json:"fact_id"`
}

// BundleChecks is the deterministic post-proposal check result.
type BundleChecks struct {
	Passed                    bool     `json:"passed"`
	UnknownFactIDs            []string `json:"unknown_fact_ids"`
	ForbiddenPhrases          []string `json:"forbidden_phrases"`
	UnsupportedNumbers        []string `json:"unsupported_numbers"`
	CheckedFactIDs            []string `json:"checked_fact_ids"`
	ManualReviewRequired      bool     `json:"manual_review_required"`
	Limitation                string   `json:"limitation"`
	StructuralErrors          []string `json:"structural_errors"`
	SemanticSupportVerified   bool     `json:"semantic_support_verified"`
}

// BundleUsage summarises LLM cost for one run.
type BundleUsage struct {
	ModelCalls   int `json:"model_calls"`
	ToolCalls    int `json:"tool_calls"`
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// BundleStep is one entry in the agent trace.
type BundleStep struct {
	Step            string   `json:"step"`
	Turn            int      `json:"turn,omitempty"`
	Tool            string   `json:"tool,omitempty"`
	DurationMS      *int     `json:"duration_ms,omitempty"`
	ToolCalls       int      `json:"tool_calls,omitempty"`
	Attempt         int      `json:"attempt,omitempty"`
	Passed          *bool    `json:"passed,omitempty"`
	Issues          int      `json:"issues,omitempty"`
	SelectedFactIDs []string `json:"selected_fact_ids,omitempty"`
}

// FileProblem describes one ingest-time issue.
type FileProblem struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// ScanReport is the result of scanning an input tree.
type ScanReport struct {
	Loaded    []Bundle      `json:"loaded"`
	Problems  []FileProblem `json:"problems"`
	WalkedAt  time.Time     `json:"walked_at"`
	RootPath  string        `json:"root_path"`
}

// StatusCounts counts runs by status.
type StatusCounts struct {
	PendingReview       int `json:"pending_review"`
	InsufficientEvidence int `json:"insufficient_evidence"`
	NeedsRevision       int `json:"needs_revision"`
	ModelError          int `json:"model_error"`
	Other               int `json:"other"`
}

// canonicalStatuses is the set of statuses the upstream pipeline emits.
// Anything else flows into Other.
var canonicalStatuses = []string{
	"pending_review",
	"insufficient_evidence",
	"needs_revision",
	"model_error",
}

// CountByStatus tallies the bundle status histogram.
func CountByStatus(bundles []Bundle) StatusCounts {
	var c StatusCounts
	for _, b := range bundles {
		switch b.Status {
		case "pending_review":
			c.PendingReview++
		case "insufficient_evidence":
			c.InsufficientEvidence++
		case "needs_revision":
			c.NeedsRevision++
		case "model_error":
			c.ModelError++
		default:
			c.Other++
		}
	}
	return c
}

// Scan walks root recursively and loads every audit_bundle.json it
// finds. Files that fail to decode are recorded in Problems but do not
// abort the scan.
func Scan(root string) (ScanReport, error) {
	report := ScanReport{
		WalkedAt: time.Now().UTC(),
		RootPath: root,
	}
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			report.Problems = append(report.Problems, FileProblem{
				Path:   path,
				Reason: fmt.Sprintf("walk error: %v", err),
			})
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !isAuditBundle(path) {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			report.Problems = append(report.Problems, FileProblem{
				Path:   path,
				Reason: fmt.Sprintf("read error: %v", readErr),
			})
			return nil
		}
		var bundle Bundle
		if decErr := json.Unmarshal(raw, &bundle); decErr != nil {
			report.Problems = append(report.Problems, FileProblem{
				Path:   path,
				Reason: fmt.Sprintf("decode error: %v", decErr),
			})
			return nil
		}
		if bundle.RunID == "" {
			report.Problems = append(report.Problems, FileProblem{
				Path:   path,
				Reason: "missing run_id",
			})
			return nil
		}
		if bundle.Status == "" {
			report.Problems = append(report.Problems, FileProblem{
				Path:   path,
				Reason: "missing status",
			})
			return nil
		}
		bundle.SourcePath = path
		bundle.Raw = raw
		report.Loaded = append(report.Loaded, bundle)
		return nil
	})
	if walkErr != nil {
		return report, fmt.Errorf("walk %s: %w", root, walkErr)
	}
	sort.Slice(report.Loaded, func(i, j int) bool {
		return report.Loaded[i].StartedAt.Before(report.Loaded[j].StartedAt)
	})
	return report, nil
}

// isAuditBundle matches the canonical filename emitted by claim-studio.
func isAuditBundle(path string) bool {
	return filepath.Base(path) == "audit_bundle.json"
}

// KnownStatuses returns the canonical status labels. Useful for tests
// and for downstream tooling that wants to enumerate what the pipeline
// actually emits.
func KnownStatuses() []string {
	out := make([]string, len(canonicalStatuses))
	copy(out, canonicalStatuses)
	return out
}

// IssueKindLabels returns the canonical issue-kind labels claim-studio
// assigns inside AuditIssue.
func IssueKindLabels() []string {
	return []string{"unsupported", "overstated", "needs_review"}
}

// ValidStatus reports whether s is one of the canonical pipeline
// statuses.
func ValidStatus(s string) bool {
	for _, k := range canonicalStatuses {
		if k == s {
			return true
		}
	}
	return false
}

// NormaliseStrategy folds blank / unknown strategy names into a stable
// bucket so dashboards are not polluted with one-off strings.
func NormaliseStrategy(s string) string {
	s = strings.TrimSpace(s)
	switch s {
	case "", "unknown", "none":
		return "unspecified"
	}
	return s
}
