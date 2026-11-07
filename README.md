# The fastest invoice generation in the history of software

Self-hosted microservice that renders PDF invoices from JSON payloads and stores them in
S3-compatible object storage (AWS S3, Cloudflare R2, MinIO). Synchronous, stateless, and fast:
~2.5ms per invoice with the pure-Go engine, 500+ invoices/minute per small replica, scale out
by adding replicas.

## Quick start

```bash
make dev          # creates .env from .env.example, starts the S3 mock + server
curl http://localhost:8080/health
# → {"status":"ok"}
```

| Service | URL |
|---|---|
| Server | http://localhost:8080 |
| S3 mock | http://localhost:9090 |

The compose file uses [adobe/s3mock](https://hub.docker.com/r/adobe/s3mock) (in-memory) for
development; point `S3_ENDPOINT` at MinIO, AWS S3, or R2 for anything real.

## API

### Create an invoice

```bash
curl -X POST http://localhost:8080/invoices \
  -H "X-API-Key: changeme" -H "Content-Type: application/json" \
  -d '{
    "id": "01J8ZZZZZZZZZZZZZZZZZZZZZZ",
    "engine": "basic",
    "payload": {
      "invoice_number": "INV-2026-001",
      "date": "2026-11-07",
      "bill_to": "Wayne Enterprises",
      "items": [{"description": "Consulting", "quantity": 8, "rate": 150.00}],
      "tax_rate": 8.5
    }
  }'
# → {"invoice_id":"01J8ZZZZZZZZZZZZZZZZZZZZZZ","url":"http://localhost:8080/invoices/01J8ZZZZZZZZZZZZZZZZZZZZZZ"}
```

- The response is only sent after the PDF is durably stored.
- `id` is optional (a ULID is generated). When supplied it must match
  `[A-Za-z0-9_-]{22,64}` and makes retries idempotent: same id, same URL, one artifact.
- Overload returns `503` with `Retry-After` instead of queueing.

### Download an invoice

```bash
curl -L http://localhost:8080/invoices/01J8ZZZZZZZZZZZZZZZZZZZZZZ -o invoice.pdf
```

The service streams the bytes itself. Storage URLs, bucket names, and providers are never
exposed; there are no redirects.

### Errors

All errors are `{"error": "...", "code": "..."}` — see
`openspec/changes/trimming-the-fat/specs/invoice-api/spec.md` for the full mapping.

## Engines — which one to use

Every request picks an engine. **Rule of thumb: CJK text (Chinese, Japanese, Korean) must use
`html`; everything else should use `basic`.**

| | `basic` (default) | `html` |
|---|---|---|
| Use for | Latin, Greek, Cyrillic text | Chinese / Japanese / Korean text |
| Speed | ~2.5ms render — ~7,300 invoices/min on a 1 vCPU replica | ~74ms render on multi-core; needs 2+ vCPUs (~25x slower than `basic`) |
| Fonts | One embedded font (DejaVu) — no CJK glyphs | System fonts incl. Noto Sans CJK |
| Memory | trivial (~30MB) | browser process, hundreds of MB |

```jsonc
// Default — omit "engine" entirely, or set it explicitly:
{ "payload": { "bill_to": "Wayne Enterprises", "items": [/* ... */] } }

// Chinese / Japanese / Korean invoices:
{ "engine": "html", "payload": { "bill_to": "株式会社 田中商事", "items": [/* ... */] } }
```

The caller picks the engine; the service never sniffs payloads (a silent 25x cost switch would
make capacity unpredictable). Safety net: `basic` never prints blank glyphs — a CJK payload
sent to `basic` fails with `422 invalid_payload` and the message
`payload contains CJK text; use engine "html"`, so it is safe to retry the same invoice with
`engine: "html"`.

## Themes & branding

Layouts are **compile-time themes**: embedded in the binary, parsed once at startup, and free
per request — no template files at runtime, no cache, nothing to reload.

Branding is configuration:

| Variable | Purpose |
|---|---|
| `COMPANY_NAME` | Header text |
| `COMPANY_COLOR` | Header and table color (6-digit hex) |
| `COMPANY_LOGO` | Optional path to a PNG or JPEG logo (max 1 MiB), shown in the header on both engines |

Adding a theme = one Go file + one HTML asset in `internal/renderer/templates/` (see
`default.go` + `default.html`), then rebuild and select it with `TEMPLATE=<name>`; an unknown
name fails startup. Themes are compiled in on purpose — no runtime template files.

## Benchmarks

Measured on a **1 vCPU / 1 GiB** replica against a **real Cloudflare R2 bucket**, closed-loop
load ramp running in-network (`cmd/stress`, 15s per step):

| Engine | Best sustained | p50 | p99 at saturation | Render mean | Upload mean |
|---|---|---|---|---|---|
| `basic` | ~1,820 invoices/min | ~525ms | ~1.1s | 9ms | 507ms |
| `html` (CJK) | ~284 invoices/min | ~690ms | 4.3–7.7s | 1,617ms | 568ms |

The upload round-trip to storage dominates: throughput ≈ `MAX_CONCURRENT_RENDERS ÷ (render + upload)`.
On this box the basic engine has **~3.6x headroom over 500 invoices/min**; the html engine
needs at least 2 vCPUs. Raising `MAX_CONCURRENT_RENDERS` or using a bucket region closer to
your callers scales throughput proportionally.

Reproduce with a running server: `go run ./cmd/stress -steps 1,2,4,8,16,32,64 -duration 15s`
(full results and run instructions in `scripts/loadtest/README.md`).

## Payload fields

| Field | Type | Notes |
|---|---|---|
| `invoice_number`, `date`, `due_date` | string | meta rows |
| `bill_to` (or `bill_to_name`), `bill_to_address` | string | address supports newlines |
| `items` | array | `description`, `quantity`, `rate`, optional `amount` |
| `tax_rate` | number | percent |
| `notes` | string | free text |

Line amount = `amount` or `quantity x rate`; totals are computed from these.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `BASE_URL` | `http://localhost:8080` | Prefix for returned invoice URLs |
| `PORT` | `8080` | HTTP port |
| `API_KEY` | (required) | POST authentication |
| `COMPANY_NAME` / `COMPANY_COLOR` / `COMPANY_LOGO` | `Invoice` / `284878` / (none) | Header branding; logo is a PNG/JPEG path (≤1 MiB) |
| `TEMPLATE` | `default` | Compiled-in theme name; unknown values fail startup |
| `MAX_CONCURRENT_RENDERS` | `16` | Render semaphore size |
| `ADMISSION_WAIT_MS` | `1000` | Max wait for a render slot before 503 |
| `RENDER_TIMEOUT_SECONDS` | `10` | Per-render deadline |
| `LOG_LEVEL` | `info` | Log verbosity |
| `S3_ENDPOINT`, `S3_REGION`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_USE_PATH_STYLE` | s3mock defaults | Storage |
| `CHROME_PATH` | autodetect | Browser binary for the html engine |

Object lifecycle and expiry are configured in your storage console, not in this service.

## Deploying

A Render blueprint ships in `render.yaml`:

1. Render Dashboard → **New +** → **Blueprint** → select this repository.
2. Fill the prompted secrets (`API_KEY`, `S3_*`) and Apply.
3. Done — the service URL is picked up automatically (`RENDER_EXTERNAL_URL`). Set `BASE_URL`
   explicitly only for a custom domain.

The blueprint defaults to the `starter` plan (512 MB) which is fine for the `basic` engine;
switch to `standard` (2 GB) if you use the `html` engine (Chromium). Pushes to `main`
redeploy automatically.

## Development

```bash
make dev      # MinIO + server in docker compose
make build    # build the server binary
make test     # go test ./...
make vet      # go vet ./...

go test ./internal/renderer -bench . -run '^$'   # renderer benchmarks
```

Requirements and design live in `openspec/` — start with
`openspec/changes/trimming-the-fat/proposal.md`.
