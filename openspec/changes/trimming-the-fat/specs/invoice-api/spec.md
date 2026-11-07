# Delta for Invoice API

## Purpose

Accept invoice requests over HTTP, produce a durable PDF artifact, and serve it back through a
stable, masked URL. Provide authentication, idempotent identity, and predictable behavior
under overload. This is the public contract of the service.

## ADDED Requirements

### Requirement: Synchronous Creation

`POST /invoices` MUST complete validation, rendering, and upload within the request, and MUST
respond `200` with `{"invoice_id": "...", "url": "<BASE_URL>/invoices/<id>"}` only after the
object exists in storage. The service MUST NOT defer any of this work to background
processing.

#### Scenario: Successful creation

- **WHEN** a valid, authenticated request arrives
- **THEN** the service renders and uploads the PDF
- **AND** responds `200` with the invoice id and absolute URL
- **AND** the object `invoices/<id>.pdf` exists before the response is sent

#### Scenario: No deferred work

- **WHEN** the client has received the `200` response
- **THEN** no rendering, upload, or cleanup work for that request remains

#### Scenario: Storage failure

- **WHEN** upload fails after internal retries
- **THEN** the service responds `502` with code `storage_unavailable`

### Requirement: Request Format

The service MUST require `Content-Type: application/json` for POST, MUST reject malformed
JSON with `400`, SHOULD ignore unknown fields for forward compatibility, and MUST reject
request bodies larger than 1 MiB with `413`.

#### Scenario: Wrong content type

- **WHEN** a POST arrives without a JSON content type
- **THEN** the service responds `415` with code `unsupported_media_type`

#### Scenario: Oversized body

- **WHEN** the request body exceeds 1 MiB
- **THEN** the service responds `413` with code `payload_too_large`

#### Scenario: Unknown fields

- **WHEN** the payload contains fields the service does not know
- **THEN** the request is still processed normally

### Requirement: Authentication

POST MUST require a valid `X-API-Key` header, compared in constant time. The service MUST
refuse to start when `API_KEY` is empty. `GET /invoices/{id}` is intentionally
unauthenticated: the unguessable URL is the capability.

#### Scenario: Missing or wrong key

- **WHEN** the `X-API-Key` header is absent or does not match
- **THEN** the service responds `401` with code `unauthorized`
- **AND** no render is attempted

#### Scenario: Unauthenticated download

- **WHEN** a valid invoice id is requested without credentials
- **THEN** the service streams the PDF

### Requirement: Invoice Identity and Idempotency

The optional `id` field MUST match `^[A-Za-z0-9_-]{22,64}$`; invalid values MUST be rejected
with `400 invalid_id`. When `id` is omitted the service MUST generate a ULID. The id MUST be
used as the storage key. A repeated POST with the same id MUST produce the same URL and a
single artifact (last write wins).

#### Scenario: Generated id

- **WHEN** a request omits `id`
- **THEN** the service generates a ULID and returns it as `invoice_id`

#### Scenario: Repeated post with the same id

- **WHEN** the caller repeats a POST with an id it has used before
- **THEN** the same URL is returned
- **AND** exactly one object exists at that key

#### Scenario: Short or malformed id

- **WHEN** the supplied id is shorter than 22 characters or contains invalid characters
- **THEN** the service responds `400` with code `invalid_id`

### Requirement: Engine Selection

The optional `engine` field MUST default to `basic` and MUST accept only `basic` and `html`.
The service MUST NOT change engine behavior based on payload content.

#### Scenario: Default engine

- **WHEN** a request omits `engine`
- **THEN** the basic engine renders the invoice

#### Scenario: Unknown engine

- **WHEN** `engine` is neither `basic` nor `html`
- **THEN** the service responds `400` with code `unknown_engine`

#### Scenario: Engine unavailable

- **WHEN** `engine: html` is requested on a deployment without a usable browser
- **THEN** the service responds `503` with code `engine_unavailable`

### Requirement: Backpressure

The service MUST bound concurrent renders with a semaphore of `MAX_CONCURRENT_RENDERS`
slots. A request MUST wait at most `ADMISSION_WAIT_MS` for a slot and MUST otherwise receive
`503` with code `overloaded` and a `Retry-After` header. The service MUST NOT allow
unbounded goroutine or render growth, and MUST recover without intervention once load
subsides.

#### Scenario: Saturation

- **WHEN** all render slots are busy for longer than the admission wait
- **THEN** the service responds `503` with `Retry-After`
- **AND** process memory stays within steady-state bounds

#### Scenario: Recovery

- **WHEN** the burst subsides and slots free up
- **THEN** subsequent requests succeed again without a restart

### Requirement: Error Semantics

All error responses MUST be JSON objects with `error` (human readable) and `code`
(machine readable) fields. The service MUST map every failure condition to the exact status
and code shown in the scenarios below, and MUST NOT emit codes outside this set.

#### Scenario: Error code mapping

- **WHEN** a request fails for any reason
- **THEN** the response status and code are exactly:

| Condition | Status | Code |
|---|---|---|
| Malformed JSON | 400 | `invalid_json` |
| Invalid id | 400 | `invalid_id` |
| Unknown engine | 400 | `unknown_engine` |
| Missing/invalid API key | 401 | `unauthorized` |
| Body over 1 MiB | 413 | `payload_too_large` |
| Wrong content type | 415 | `unsupported_media_type` |
| Structurally invalid payload | 422 | `invalid_payload` |
| Engine failure | 500 | `render_failed` |
| Render timeout | 500 | `render_timeout` |
| Upload failed after retries | 502 | `storage_unavailable` |
| Overload | 503 | `overloaded` |
| Engine unavailable | 503 | `engine_unavailable` |

#### Scenario: Structurally invalid payload

- **WHEN** `items` is not an array of objects or contains non-numeric amounts
- **THEN** the service responds `422` with code `invalid_payload`
- **AND** no render is attempted

#### Scenario: Render failure

- **WHEN** the selected engine returns an error or exceeds `RENDER_TIMEOUT_SECONDS`
- **THEN** the service responds `500` with `render_failed` or `render_timeout`
- **AND** no object is written to storage

### Requirement: Masked Download

`GET /invoices/{id}` MUST stream the PDF bytes from storage with
`Content-Type: application/pdf` and without buffering the whole object in memory. It MUST NOT
redirect, MUST NOT emit a `Location` header, and MUST NOT expose the storage provider, host,
bucket, or object path to the client. Missing, expired, or deleted objects MUST yield `404`
with code `not_found`. Invalid ids MUST yield `400 invalid_id`.

#### Scenario: Successful download

- **WHEN** an existing invoice id is requested
- **THEN** the response body is the PDF bytes
- **AND** `Content-Type` is `application/pdf`
- **AND** no redirect or storage URL appears in the response

#### Scenario: Expired or unknown invoice

- **WHEN** the object was expired by a lifecycle rule or never existed
- **THEN** the service responds `404` with code `not_found`

### Requirement: Performance Budget

At 500 invoices/minute sustained on a single 2 vCPU replica, p99 POST latency MUST stay
below 1s for the basic engine and below 3s for the html engine, with zero failures for valid
payloads.

#### Scenario: Sustained target

- **WHEN** the committed load test runs 500 invoices/minute for 10 minutes
- **THEN** all requests succeed
- **AND** p99 latency stays within the per-engine budget
