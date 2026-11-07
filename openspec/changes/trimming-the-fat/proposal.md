# Proposal: Trimming the Fat — Synchronous, Stateless Invoice Generation

## Why

The current `AGENTS.md` design is an asynchronous distributed system: Redis Streams with
consumer groups and PEL recovery, a dead-letter queue, a Redis status/payload store with
TTLs, a SQLite audit log, status polling, a worker pool, pipeline stages, an admin UI, and a
template registry. It was written for burst absorption and operator recovery, but the actual
workload is 500 invoices/minute (~8.3/sec), and none of that machinery exists in code yet —
only the PDF renderer does.

At 8.3/sec a synchronous pipeline (render + upload inside the request, bounded by a
semaphore) has more than 5x headroom on a single small replica: the basic engine renders in
single-digit milliseconds and even the Chromium engine is ~150-300ms. The queue adds latency
(polling), failure modes (stuck PEL entries, TTL expiry, DLQ runbooks), and operational
surface (Redis) to a product that has not shipped. Meanwhile the contract the order engine
actually wants — "here is the URL for order N's invoice" — is naturally synchronous.

This change replaces the async design with a stateless, synchronous service, and explicitly
parks the distributed machinery until a real trigger appears.

**Revisit triggers (parked, not deleted):**

- Callers that cannot wait for a synchronous response (fire-and-forget) or batch generation
  (e.g. month-end runs).
- Engines with multi-second render times becoming the default.
- A requirement for guaranteed at-least-once generation or a customer-visible audit trail /
  admin dashboard.

## What Changes

**Add**

- `POST /invoices`: validate → acquire render slot → render → upload → respond
  `{invoice_id, url}`. Overload returns `503` + `Retry-After`.
- `GET /invoices/{id}`: stream PDF bytes from storage. URL masking is kept; the endpoint
  never redirects and never exposes the storage provider.
- Caller-selected engine (`basic` default, `html` opt-in), replacing content sniffing.
- Caller-supplied invoice id for idempotency and stable URLs; ULID generated when omitted.
- API-key authentication on POST; structured JSON logs; Prometheus `/metrics`; `/health`.
- Renderer benchmarks and a committed load-test script; measured capacity numbers.
- A single container image with Chromium + CJK fonts so the html engine works in production.
- Branding as deployment config (`COMPANY_NAME`, `COMPANY_COLOR`).

**Drop (explicitly out of scope now and in this change)**

- Redis entirely: job queue, consumer groups, PEL/XAUTOCLAIM, dead-letter queue, status
  store, payload store.
- SQLite audit log; `invoice.generated` events; email delivery.
- Status polling endpoint and the `processing/done/failed/dead` state machine.
- Worker pool and pipeline stages as separate components (until benchmarks justify them).
- Admin dashboard and `ADMIN_KEY`.
- Template registry, `template_uuid` indirection, and the template cache.
- CJK auto-sniffing, presigned URLs, and per-render browser launches.
- Config variables nothing reads (`REDIS_URL`, `SQLITE_PATH`, `ADMIN_KEY`, `R2_*`,
  `STORAGE_BACKEND`); the `redis` service in docker-compose.

**Simplify**

- One pipeline, one semaphore, one storage implementation, one engine interface.
- Two public data endpoints plus `/health` and `/metrics`.
- `AGENTS.md` becomes a short pointer to the OpenSpec docs; README describes the real
  product.

## Capabilities

### New Capabilities

- `invoice-api`: synchronous creation, validation, auth, idempotent identity, backpressure,
  error semantics, and masked download.
- `rendering`: engine selection and the basic/html engines, payload contract, sanitization,
  concurrency safety.
- `storage`: key convention, upload with bounded retries, streaming download, bucket
  bootstrap, provider configuration.
- `operations`: stateless deployment, graceful shutdown, logging, metrics, health,
  configuration surface, container image, capacity verification.

### Modified Capabilities

None. The repository has no existing OpenSpec specs; code-level impact is listed below.

## Impact

- **Code:** new request pipeline and handlers; renderer refactored behind an engine
  interface; config trimmed and extended; unused domain types removed; Dockerfile builder
  Go version aligned with `go.mod` and runtime gains Chromium; compose drops Redis; Makefile
  `dev` works without a committed `.env`.
- **Removed planning artifacts:** the async sections of `AGENTS.md` are superseded by this
  OpenSpec directory (task 5.4).
- **Dependencies:** adds `prometheus/client_golang`. Removes nothing (Redis and SQLite were
  never dependencies).
- **External services:** S3-compatible object storage only (AWS S3, Cloudflare R2, MinIO).
- **Public API:** this creates the first real API contract; the optimistic `202` response
  sketched in `AGENTS.md` is explicitly replaced by the synchronous `200`.
