# Architecture

## Boundary

The sidecar is **read-only** with respect to claim-studio. It never writes back to the upstream pipeline and never opens an LLM connection. The contract surface is a single on-disk artifact: `runs/<run-id>/audit_bundle.json`.

## Pipeline

```
                 ┌──────────────────────┐
 claim-studio    │ writes audit bundles │   ← Python (upstream, untouched)
 (Python)        │     to ./runs/       │
                 └──────────┬───────────┘
                            │
                            ▼
                ┌────────────────────────┐
                │  filepath.WalkDir      │
                │  → ingest.Scan         │   ← internal/ingest
                │   · decode JSON        │
                │   · tolerate failures │
                └──────────┬─────────────┘
                           │ []ingest.Bundle
                           ▼
                ┌────────────────────────┐
                │ metrics.Summarise      │   ← internal/metrics
                │   · latency P50/90/99  │
                │   · token Σ/mean/P99   │
                │   · citations / issues │
                └──────────┬─────────────┘
                           │ metrics.Summary
                           ▼
                ┌────────────────────────┐
                │ report.WriteMarkdown / │   ← internal/report
                │ report.WriteJSON       │
                └────────────────────────┘
                           │
                           ▼
                stdout → CI bot / dashboard
```

## Module boundaries

| Package        | Visibility          | Owns                                        |
|----------------|--------------------|---------------------------------------------|
| `ingest`       | internal           | filesystem walk + JSON decode contract      |
| `metrics`      | internal           | percentile / count / token math             |
| `report`       | internal           | markdown + JSON rendering                   |
| `cmd/aggregator` | main              | flag parsing + orchestration                |

There are **no external dependencies** in `go.mod`. Every helper used by the sidecar comes from the Go standard library (encoding/json, flag, filepath, io, log, math, sort, strings, time).

## Determinism guarantees

- All percentile calculations use nearest-rank on a sorted copy of the input, so the same JSON files produce the same numbers byte-for-byte.
- Output keys in JSON are sorted (`encoding/json` preserves struct order, which is hand-curated to read top-down).
- Strategy names are normalised (`""` / `"unknown"` / `"none"` → `"unspecified"`).
- The markdown header embeds a generation timestamp so two identical scans rendered a second apart still differ by exactly that one timestamp; everything else is byte-stable.

## Failure model

- A malformed JSON file lands in `ScanReport.Problems` and the scan continues.
- A missing required field (`run_id` or `status`) likewise lands in `Problems`.
- An empty `runs/` directory renders a markdown file with the "no audit bundles found" branch instead of crashing.
- An unknown `--format` is rejected at the CLI layer.

## Future surfaces (planned, not yet built)

| Surface           | Path                                                    |
|-------------------|---------------------------------------------------------|
| Anomaly detection | `internal/anomaly/` — z-score + EWMA over a sliding window. |
| Prometheus       | `internal/exporter/` — separate `:9095/metrics` endpoint. |
| Watch mode        | `cmd/aggregator --watch` — re-scan every N seconds.    |
