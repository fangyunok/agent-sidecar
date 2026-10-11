package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRequiresInput(t *testing.T) {
	if err := run([]string{}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatalf("expected --input error when flag is omitted")
	}
}

func TestRunRequiresDirectory(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	if err := run([]string{"--input", "definitely-not-a-real-file"}, out, errOut); err == nil {
		t.Fatalf("expected an error for a non-existent --input")
	}
}

func TestRunRejectsUnknownFormat(t *testing.T) {
	tmp := t.TempDir()
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	err := run([]string{"--input", tmp, "--format", "yaml"}, out, errOut)
	if err == nil {
		t.Fatalf("expected --format=yaml to be rejected")
	}
	if !strings.Contains(err.Error(), "unsupported --format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunMarkdownEmptyDir(t *testing.T) {
	tmp := t.TempDir()
	out := &bytes.Buffer{}
	if err := run([]string{"--input", tmp, "--title", "Empty"}, out, &bytes.Buffer{}); err != nil {
		t.Fatalf("run err: %v", err)
	}
	if !strings.Contains(out.String(), "Empty") {
		t.Fatalf("title missing")
	}
	if !strings.Contains(out.String(), "(no audit bundles found)") {
		t.Fatalf("empty branch missing")
	}
}

func TestRunJSONEmptyDir(t *testing.T) {
	tmp := t.TempDir()
	out := &bytes.Buffer{}
	if err := run([]string{"--input", tmp, "--format", "json"}, out, &bytes.Buffer{}); err != nil {
		t.Fatalf("run err: %v", err)
	}
	if !strings.Contains(out.String(), "\"tool\": \"agent-sidecar\"") {
		t.Fatalf("json header missing: %s", out.String())
	}
}
