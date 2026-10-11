# State

This project follows a W1 → W4 roadmap. Each row says what's done and what isn't; nothing in the "Roadmap" column is shipped.

## W1 — v0.1.0 (this tag)

| Block                         | Status | Evidence                                                 |
|-------------------------------|--------|----------------------------------------------------------|
| Bundle scan + JSON decode     | done   | `internal/ingest`, `tests/` cover empty / malformed / missing fields |
| Per-run aggregation (latency, tokens, citations, issues) | done | `internal/metrics`, `tests/` cover percentile + count math |
| Markdown + JSON renderers     | done   | `internal/report`, `tests/` cover snapshot + indent + tz |
| CLI entrypoint                | done   | `cmd/aggregator/main.go`, `tests/` cover flag validation |
| Distroless Docker image       | done   | `Dockerfile` (multi-stage CGO=0 + distroless/static)    |
| CI matrix (Go 1.22 / 1.23)    | done   | `.github/workflows/ci.yml` (vet + race tests + smoke run) |
| 4 fixtures + 1 malformed file | done   | `testdata/sample_runs/`                                   |
| Go module / license / gitignore | done | `go.mod`, `LICENSE`, `.gitignore`                        |

## Roadmap — NOT shipped yet

This list is the truthful backlog. The README does not advertise any of these.

| Block                                    | Target | Note                                  |
|------------------------------------------|--------|---------------------------------------|
| Anomaly detection (z-score + EWMA)        | v0.5   | W2 block; will live in `internal/anomaly/` |
| Prometheus exporter on `:9095/metrics`    | v0.5   | W2 block; will live in `internal/exporter/` |
| Watch mode (`--watch Ns`)                | v0.5   | W2 block; uses `fsnotify` if W2 picks it up — currently absent from `go.mod` |
| Cross-version claim-studio run replay    | v0.5   | W2 block; verifies the contract stays stable across claim-studio versions |
| Single-binary release on multiple OS      | v1.0   | Linux + macOS pre-built binaries; Windows build deferred |
| Integration test on real claim-studio runs | v1.0  | Requires claim-studio `runs/` fixture, only safe on Linux due to ffmpeg dependency |

## What this sidecar will not do (by design)

- It will not call any LLM. There is no keyring, no model adapter.
- It will not mutate claim-studio. The boundary is read-only.
- It will not silently swallow errors — every malformed file lands in `ScanReport.Problems` and is reported in markdown.
- It will not vendor claim-studio's Python code; the data contract is JSON only.
