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

| Date | Box | Engine | Result | p50 | p99 | Notes |
|---|---|---|---|---|---|---|
| 2026-11-07 | 1 vCPU / 1 GiB container | basic | 1,820/min best; zero errors to c=32; 503 only at c=64 | ~525ms | ~1.1s at saturation | real R2; upload mean 507ms dominates |
| 2026-11-07 | 1 vCPU / 1 GiB container, `MAX_CONCURRENT_RENDERS=64` | basic | 7,319/min; zero errors to c=128 | ~475–986ms | ~1.0–1.5s | CPU-bound at ~122/s; ~60MB RSS |
| 2026-11-07 | 1 vCPU / 1 GiB container | html + CJK | 284/min best (c=4); declines under load | ~690ms | 4.3-7.7s | Chromium CPU-bound on 1 vCPU; 503 from c=32 |
