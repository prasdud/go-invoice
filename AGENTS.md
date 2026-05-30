---
description: Behavioral guidelines to reduce common LLM coding mistakes. Use when writing, reviewing, or refactoring code to avoid overcomplication, make surgical changes, surface assumptions, and define verifiable success criteria.
alwaysApply: true
---

# Karpathy behavioral guidelines

Behavioral guidelines to reduce common LLM coding mistakes. Merge with project-specific instructions as needed.

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

## 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

## 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

---

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.

===

# Invoice Generation Microservice

## What This Is

A self-hosted, open source microservice written in Go that generates PDF invoices. It is a standalone service separate from the order processing engine. Callers send an invoice payload + template reference, the service renders a PDF, uploads it to R2/S3, and returns a presigned URL. The response is optimistic — returned immediately after payload validation, before rendering completes.

---

## Architecture

### Why a Separate Microservice
- PDF rendering is CPU-heavy and would contend with order engine throughput
- Invoice logic (templates, tax, formatting) changes independently of order logic
- Different scaling axes — order engine scales for TPS, invoice service scales for burst PDF generation
- Failure isolation — invoice failures must not degrade order flow

### Request Flow
```
POST /invoices
  { template_uuid, engine, payload }
→ validate payload
→ return { invoice_id, status_url }   ← optimistic, immediate
→ enqueue job to Redis Stream
→ worker picks up job
→ render PDF
→ upload to R2/S3
→ update Redis key with status + presigned URL
→ write audit record to SQLite
```

### URL Masking
S3/R2 URLs are never exposed externally — not even via redirect. A 302 redirect would leak the real URL in the `Location` header, defeating the purpose.

The service streams PDF bytes directly to the client:
- Operator points their domain at the service
- Client requests `GET /invoices?id=xxx` on that domain
- Service resolves `id` → S3 key via Redis, fetches the object from S3 internally, streams bytes to the client
- Client only ever sees the operator's domain

This hides cloud provider, region, bucket name, and internal path structure. URLs remain stable across storage migrations.

`BASE_URL` is set via env var and used to construct the URL returned after job creation.

---

## Technology Choices

| Concern | Choice | Reason |
|---|---|---|
| Language | Go | Low memory, fast startup, excellent concurrency, easy horizontal scaling |
| Queue | Redis Streams | Already used for status polling, supports consumer groups, DLQ pattern, no extra infra |
| Status store | Redis | TTL-based, runtime state only |
| Audit log | SQLite | Zero operational overhead, file-based, permanent history |
| Object storage | R2 or S3 (configurable) | Lifecycle rules handled via storage provider console, not in code |
| PDF engine | Pluggable (see below) | |

---

## PDF Rendering Engines

Engine is specified per-request in the payload:
```json
{ "engine": "basic", "template": "acme-uuid", ... }
```

- `basic` — pure Go, default, fast, low memory, suitable for simple layouts
- Additional engines can be added without changing the API contract

Template complexity determines engine choice. Start with `basic`. Chromedp/headless Chrome is available as a future option for pixel-perfect HTML→CSS layouts but costs ~150-300ms and ~200MB per instance.

---

## Templates

- Stored in R2/S3, keyed by `template_uuid`
- Uploaded via API or storage console
- `template_uuid` is immutable per version — new upload = new UUID
- Hot templates cached in-memory (`sync.Map`) per worker instance, or Redis if multi-instance
- Already-rendered PDFs are unaffected by template changes — no retroactive re-render

---

## API

### `POST /invoices`
Accepts invoice payload + template UUID + engine. Validates payload integrity. Returns immediately with `invoice_id` and a polling URL. Enqueues render job.

### `GET /invoices/{id}/status`
Polls Redis for job status: `processing | done | failed | dead`. Returns the masked URL when `done`.

### `GET /invoices?id=xxx`
URL masking endpoint. Resolves `id` → S3 key, fetches PDF from S3 internally, streams bytes to client. Never redirects. S3 is never exposed.

Auth: API key via `X-API-Key` header. Configured as an env var, stored hashed.

---

## Go Concurrency Model

### Worker Pool
Fixed pool of N workers pulling from a buffered channel. N tuned to CPU cores and render time, not arbitrary. Prevents goroutine explosion under burst.
```
jobs (buffered channel, size ~1000)
  ← ingestion goroutine writes
  → N worker goroutines read
```

### Pipeline Stages
Render and upload are separate pipeline stages connected by channels. Render is CPU-bound, upload is I/O-bound — they scale independently with different worker counts.

### Atomicity — No Double Processing
Redis Streams consumer groups guarantee exactly-one delivery. `XREADGROUP` claims a message to a specific consumer and moves it to the Pending Entries List (PEL). Other workers cannot claim it. If a worker dies without calling `XACK`, the message stays in PEL and is reclaimed via `XAUTOCLAIM` after a configurable timeout. No extra locking needed.

### Object Pooling
PDF generation is allocation-heavy. Use `sync.Pool` for reusable byte buffers to reduce GC pressure at sustained throughput.

### Semaphore Pattern
If using chromedp, limit concurrent browser instances via a buffered channel acting as a semaphore. Prevents OOM under burst load.

---

## Invoice Payload Durability

Full invoice payload is stored in Redis alongside the status key:
```
invoice:{id} → { status, payload, error, attempts, url }
```
On manual retry from admin UI, payload is re-read from Redis and re-enqueued — no need for the caller to resend. Payload also written to SQLite at job creation, so it survives Redis TTL expiry for audit and recovery purposes.

---

## Queue and Retry

- Redis Streams with consumer groups
- Worker pulls jobs, renders, uploads
- On failure: retry up to 3 times
- After 3 failures: move to DLQ
- DLQ retried up to 2 more times
- After that: marked `dead`, error logged to SQLite
- Full error + payload logged at each failure stage

---

## Storage

- R2 or S3, configured via env vars
- Key structure: `year/month/invoice_id.pdf`
- Object lifecycle (expiry) configured via R2/S3 console — not handled in code
- Objects accessed internally only — never via presigned URLs exposed to clients

---

## Data Stores

### Redis
- Invoice status keys: `invoice:{id}` → `{ status, payload, url, attempts, error }`
- Keys TTL'd after a configurable period (e.g. 7 days)
- Redis Streams for job queue and DLQ

### SQLite
- Permanent audit log only
- Schema: `id, template_uuid, engine, status, attempts, error, created_at, completed_at`
- Not used for runtime state

---

## Admin Dashboard

Minimal operator UI. Not a SaaS dashboard — for self-hosters to operate the service.

Features:
- Queue depth and processing rate
- Invoice list filterable by status (processing / done / failed / dead)
- Invoice detail: payload, error logs, retry history
- Manual retry button (re-queues dead jobs)

Implementation: server-rendered HTML or single `index.html` with vanilla JS hitting internal admin API endpoints. Protected by a separate `ADMIN_KEY` env var.

Observability (metrics, alerting) is out of scope — operators plug in their own Grafana/Prometheus stack via the `/metrics` endpoint the service exposes.

---

## Configuration (env vars)

```
API_KEY             # hashed API key for /invoices auth
ADMIN_KEY           # separate key for admin UI
BASE_URL            # public-facing base URL, e.g. https://invoices.acme.com
REDIS_URL
SQLITE_PATH
R2_BUCKET / S3_BUCKET
R2_ACCOUNT_ID / AWS_REGION
STORAGE_BACKEND     # r2 or s3
```

---

## Scale Target

1000 invoices/minute (~17/sec). Achieved via horizontal worker scaling, not vertical. Redis Streams consumer groups distribute load across workers. PDF rendering is the bottleneck — profile and pool accordingly.

---

## What This Is Not

- Not a SaaS product
- No multi-tenancy
- No built-in observability UI (use Grafana)
- No email delivery (out of scope, can be added as a downstream consumer of `invoice.generated` events)
- No synchronous PDF response — always async via polling
