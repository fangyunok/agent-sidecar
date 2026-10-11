# agent-sidecar

Go observability sidecar for the **claim-studio** Python Agent / RAG audit pipeline. Reads the `audit_bundle.json` artifacts claim-studio already writes, computes latency / token / citation / issue aggregations, and renders a Markdown or JSON summary a CI bot can pin to a PR.

- **No live LLM call.** No API key. Pure offline scan over on-disk JSON.
- **Stdlib only.** No third-party Go dependencies.
- **Distroless container.** ~14 MB image.

## Why it exists

claim-studio v0.5 emits a SHA256-bound `audit_bundle.json` after every audit (see [fangyunok/claim-studio](https://github.com/fangyunok/claim-studio)). When you have dozens of runs across strategies (BM25, vector, hybrid, rerank) and across model sizes (4B vs 14B), you want a fast, deterministic way to see which runs regressed and which improved. Claim-studio's own /healthz surfaces process-level counters; this sidecar surfaces **audit-level** metrics.

## Layout

```
agent-sidecar/
├── cmd/aggregator           # CLI entrypoint
├── internal/
│   ├── ingest               # audit_bundle.json scan + decode
│   ├── metrics              # P50/P90/P99 latency, token usage, citations, issues
│   └── report               # Markdown + JSON renderers
├── testdata/sample_runs     # 4 fixtures + 1 malformed file
├── docs/                    # ARCHITECTURE.md, STATE.md
├── Dockerfile               # distroless, ~14 MB
├── Makefile                 # build / test / vet / run
└── .github/workflows/ci.yml # Go 1.22 + 1.23 matrix
```

## Quick start

```bash
make build
./bin/agent-sidecar --input testdata/sample_runs --title "Quick start"
```

Or in CI / Docker:

```bash
docker build -t agent-sidecar:dev .
docker run --rm -v "$PWD/runs:/runs" agent-sidecar:dev --input /runs --format json
```

## Status (v0.1.0)

Done:

- Discover `audit_bundle.json` recursively under `--input`.
- Decode the full audit bundle, tolerate per-file failures.
- Per-run aggregation: latency P50/P90/P99 (whole-run + per-step), token usage (mean / Σ / P50 / P99), pass rate, citations per run, issues per run, issue-kind breakdown.
- Markdown and JSON output.
- Distroless container image, ~14 MB.
- CI matrix on Go 1.22 / 1.23 with gofmt + vet + race tests + smoke run.

Roadmap (tracked in [docs/STATE.md](docs/STATE.md)):

- v0.5 — anomaly detection (z-score + EWMA) over a sliding window.
- v0.5 — Prometheus exporter on `:9095/metrics`.
- v1.0 — integration test against real claim-studio `runs/` tree (Linux + macOS).

## Data contract

The sidecar mirrors the bundle shape claim-studio already emits. It does not require any upstream change. See [`internal/ingest/ingest.go`](internal/ingest/ingest.go) for the field-by-field contract.

## License

MIT. See [LICENSE](LICENSE).
