# Design: Trimming the Fat

## Context

The repository contains a working two-engine PDF renderer, an S3-compatible storage client,
and a health-check server. The async service layer described in `AGENTS.md` (queue, consumers,
status store, audit, admin) was never built. The workload is 500 invoices/minute driven by an
order engine, and the consumer of the result is a URL it can store and serve later. That
makes a synchronous, stateless pipeline both simpler and sufficient, with a clear growth path
(add replicas) and an explicit list of triggers that would justify the async design later.

## Goals / Non-Goals

**Goals**

- 500 invoices/minute sustained on one small replica, with measured proof.
- Zero shared runtime state: storage is the only persistence.
- Two endpoints with a contract that does not change when the async design eventually
  returns (the POST response would gain a `status_url`, nothing else).
- Masked downloads: clients never see the storage provider, bucket, region, or object path.
- Predictable overload behavior: shed load with `503` instead of collapsing.

**Non-Goals**

- Queueing, polling, DLQ, retries beyond a bounded upload retry, or delivery guarantees.
- Audit storage, admin UI, dashboards (structured logs + `/metrics` are the surface).
- Templates, multi-tenancy, per-request branding, webhooks, events, email.

## Capacity Math

Target: 500 invoices/min = 8.3 req/s. In-flight renders = arrival rate x pipeline latency.

Measured 2026-11-07 (Phase 0; host Apple M5, Chrome on host; upload not measured — MinIO was
not available, estimate kept):

| Engine | Render (measured) | + upload (est.) | In-flight @8.3/s | Single replica need |
|---|---|---|---|---|
| basic (gofpdf) | 2.5ms | ~100ms total | <1 | ~2% of one core |
| html (Chromium) | 381ms (browser launched per render; pooling is Phase 3) | ~450ms total | ~3.8 | ~2 cores, <1GB RAM |

Allocations: basic ~9.0MB/op (gofpdf parses the embedded font per document), html ~1.1MB/op
plus the browser process. `many_items` is within 2% of `standard_us` for the basic engine.

With `MAX_CONCURRENT_RENDERS=16` (default, tuned in task 6.2):

- html: 16 / 0.381s ≈ 42/s ≈ 2500/min, ~5x headroom at the target (pooling improves this).
- basic: ~16 / 0.0025s ≈ 6400/s, far beyond the target.

Scale-out is horizontal: replicas are identical and interchangeable.

## Architecture

```
                    +---------------- replica (stateless) ----------------+
POST /invoices --> auth --> validate --> semaphore --> render --> upload --> 200 {id,url}
                                             |  (admission wait 1s)
                                             +--> 503 + Retry-After

GET /invoices/{id} --> validate id --> storage.get(invoices/{id}.pdf) --> stream bytes
```

No shared runtime state. The only persistence is the object store. Any replica serves any
request, so rolling deploys and scale-out need no coordination.

## Decisions

### D1: Synchronous over asynchronous

The response contract is the status: if the client holds the URL, the artifact exists. Async
buys burst absorption and recovery the workload does not need (8.3/s sustained), at the cost
of Redis, polling latency (typically 0.5-1s of the user's time), and DLQ/PEL failure modes.
Revisit when a trigger in the proposal fires; the API shape is forward-compatible.

### D2: No Redis, no SQLite

Redis's three roles (queue, status, payload store) only exist in an async design. SQLite's
roles (audit, replay, admin listing) are replaced by structured JSON logs; a durable audit
store is added only when operators ask for a dashboard or replay, and it can be added
without touching the request path.

### D3: Masked, streamed downloads

`GET /invoices/{id}` never redirects (a `302` would leak provider, region, and bucket in the
`Location` header) and never returns presigned URLs (same leak, plus expiry). The object key
is a pure function of the id, so the download path needs no lookup store. Object lifecycle
(expiry) stays configured in the storage console, not in code.

### D4: Identity and idempotency

- `id` is optional. When omitted, the service generates a ULID.
- When supplied, it MUST match `^[A-Za-z0-9_-]{22,64}$`. The length floor (~128 bits of
  entropy) keeps public GET URLs unguessable; short human keys like `ord_12345` are rejected
  with `400`.
- The id IS the storage key (`invoices/{id}.pdf`). A repeated POST with the same id
  re-renders and overwrites: same URL, no duplicate artifacts, last write wins. No existence
  check is performed (kept simple; duplicate work is bounded and harmless at this scale).
- GET is unauthenticated because the unguessable URL is the capability. If that becomes
  unacceptable, an HMAC download token is the parked alternative (no store needed).

### D5: Engine selection is explicit

`engine`: `basic` (default) | `html`. No content sniffing: the cost difference is ~40x and
detection makes latency unpredictable. Unknown value -> `400 unknown_engine`. `html`
requested on a replica without a browser -> `503 engine_unavailable`.

### D6: Backpressure

One buffered-channel semaphore (default 16 concurrent renders). A request waits up to
`ADMISSION_WAIT_MS` (default 1000) for a slot; otherwise `503` + `Retry-After: 1`. Renders
are never queued unboundedly and goroutine growth is bounded by in-flight connections. The
Chromium page count is bounded by the same semaphore; the browser process is reused and
relaunched on crash.

### D7: Branding is deployment configuration

`COMPANY_NAME` and `COMPANY_COLOR` configure the invoice header. Templates and per-request
branding are parked; the renderer's `Options` struct stays as the seam for both.

### D8: Error mapping

| Condition | Status | Code |
|---|---|---|
| Malformed JSON, invalid id format, unknown engine | 400 | `invalid_json`, `invalid_id`, `unknown_engine` |
| Missing/invalid API key | 401 | `unauthorized` |
| Body over 1 MiB | 413 | `payload_too_large` |
| Wrong content type | 415 | `unsupported_media_type` |
| Structurally invalid payload (items not objects, non-numeric amounts, too many items) | 422 | `invalid_payload` |
| Engine failure or render timeout | 500 | `render_failed`, `render_timeout` |
| Upload failed after retries | 502 | `storage_unavailable` |
| Engine unavailable on this deployment, or overloaded | 503 | `engine_unavailable`, `overloaded` (+ `Retry-After`) |
| Download miss (never generated, expired, or deleted) | 404 | `not_found` |

All error bodies are JSON: `{"error": "<human readable>", "code": "<machine readable>"}`.

### D9: Retries are the caller's job, except upload

Upload is retried twice in-process (50ms/250ms backoff) because a blip should not fail a
request that already rendered. Everything else fails fast and explicitly; idempotent ids
make caller retries safe.

### D10: Observability

- One JSON log line per request: `time, level, request_id, invoice_id, engine, status,
  http_status, wait_ms, render_ms, upload_ms, total_ms, bytes, error, remote_ip`.
  Payload contents are never logged (invoice data is PII).
- Metrics: `http_requests_total{endpoint,status}`,
  `render_duration_seconds{engine}` (histogram), `upload_duration_seconds` (histogram),
  `render_wait_seconds` (histogram), `renders_in_flight` (gauge).
- `/health` is liveness only and never touches storage, so a storage blip cannot flap load
  balancers. Readiness for storage is visible via metrics and logs.
- On SIGTERM: stop accepting, drain in-flight requests up to 10s, then exit.

### D11: Container image

Single runtime image: Debian slim + Chromium + `fonts-noto-cjk` (~350MB), `CGO_ENABLED=0`.
The html engine must work in the container exactly as it does on a developer machine.
`CHROME_PATH` can override browser autodetection.

### D12: Bucket bootstrap

At startup, check the bucket with `HeadBucket`; create it when missing. Failure is a warning,
never a crash: production buckets are usually provisioned outside the service.

## Configuration Surface

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | HTTP listen port |
| `BASE_URL` | `http://localhost:8080` | Absolute URL prefix for returned invoice URLs |
| `API_KEY` | (required) | POST authentication; service refuses to start when empty |
| `COMPANY_NAME` | `Invoice` | Header text on the PDF |
| `COMPANY_COLOR` | `284878` | Header/table hex color |
| `MAX_CONCURRENT_RENDERS` | `16` | Render semaphore size |
| `ADMISSION_WAIT_MS` | `1000` | Max wait for a render slot before 503 |
| `RENDER_TIMEOUT_SECONDS` | `10` | Per-render context deadline |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `S3_ENDPOINT` | `http://localhost:9000` | S3-compatible endpoint |
| `S3_REGION` | `us-east-1` | Region |
| `S3_BUCKET` | `invoices` | Bucket name |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | `minioadmin` | Credentials |
| `S3_USE_PATH_STYLE` | `true` | Required by MinIO, off for AWS |
| `CHROME_PATH` | (autodetect) | Browser binary for the html engine |

Removed: `REDIS_URL`, `SQLITE_PATH`, `ADMIN_KEY`, `R2_*`, `STORAGE_BACKEND`.

## Testing and Verification

- Unit tests: validation, id rules, error mapping, semaphore/admission behavior with fake
  renderer and storage.
- Integration: end-to-end against MinIO from docker-compose (render, fetch, byte check,
  no-`Location` assertion); upload-failure mapping (stop MinIO mid-test); html engine CJK
  render inside the container.
- Load (`scripts/loadtest`, k6):
  - Sustained: 500/min for 10 minutes, zero failures, p99 < 1s (basic) / < 3s (html) on a
    2 vCPU replica.
  - Burst: ramp to 5x target; only `503` degradation, no crash, full recovery after.
- OpenSpec validation of this change before archive.

## Parked (with revisit triggers)

- Async queue + polling or webhook callbacks: fire-and-forget or batch callers.
- Audit store / dashboard: operator demand for history or replay.
- Templates / per-request branding: more than one layout or brand per deployment.
- Events/email: a downstream consumer exists.
- HMAC download tokens: public GET becomes unacceptable.
