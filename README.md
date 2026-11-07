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

## Engines

| Engine | Use for | Performance |
|---|---|---|
| `basic` (default) | Latin/Greek/Cyrillic invoices | ~2.5ms render, pure Go |
| `html` | CJK (Han/Kana/Hangul) and rich layouts | ~74ms render, headless Chromium |

The caller picks the engine; the service never sniffs payloads. The `basic` engine rejects CJK
payloads with `422` (the embedded font has no CJK glyphs) instead of emitting blank text —
use `html` for those.

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
| `COMPANY_NAME` / `COMPANY_COLOR` | `Invoice` / `284878` | PDF header branding |
| `MAX_CONCURRENT_RENDERS` | `16` | Render semaphore size |
| `ADMISSION_WAIT_MS` | `1000` | Max wait for a render slot before 503 |
| `RENDER_TIMEOUT_SECONDS` | `10` | Per-render deadline |
| `LOG_LEVEL` | `info` | Log verbosity |
| `S3_ENDPOINT`, `S3_REGION`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_USE_PATH_STYLE` | s3mock defaults | Storage |
| `CHROME_PATH` | autodetect | Browser binary for the html engine |

Object lifecycle and expiry are configured in your storage console, not in this service.

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
