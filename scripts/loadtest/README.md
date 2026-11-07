# Load tests

k6 scripts that verify the acceptance criteria from
`openspec/changes/trimming-the-fat/specs/invoice-api/spec.md`.

## Prerequisites

- MinIO (or S3/R2) reachable and configured via environment
- The server running against a **disposable bucket** (never run load tests against production)
- [k6](https://k6.io) installed

```bash
make dev                             # or run the server against your infra
k6 run -e API_KEY=changeme scripts/loadtest/sustained.js
k6 run -e API_KEY=changeme scripts/loadtest/burst.js
ENGINE=html k6 run -e API_KEY=changeme -e ENGINE=html scripts/loadtest/sustained.js
```

## Acceptance

| Script | Target | Pass criteria |
|---|---|---|
| `sustained.js` | 500/min for 10 min | zero failures, p99 < 1s (basic) / < 3s (html) |
| `burst.js` | ramp to 2500/min | zero server errors; only `503 + Retry-After` during saturation; recovery after |

## Recorded results

Run on a single 2 vCPU replica and record the numbers here and in the design doc
(`openspec/changes/trimming-the-fat/design.md`, Capacity Math):

| Date | Engine | Result | p50 | p99 | Notes |
|---|---|---|---|---|---|
| _(pending)_ | | | | | |
