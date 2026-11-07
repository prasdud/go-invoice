# Tasks

## 0. Baseline Measurements

- [x] 0.1 Add `go test -bench` benchmarks for both engines (minimal, standard, many-items, CJK payloads) and record p50/p95 render times in `design.md`.
- [ ] 0.2 Benchmark the full pipeline path (render + MinIO upload) against local compose. (blocked: requires MinIO; docker daemon not running)
  - Verify: measured numbers replace the estimates in the capacity table; `go test ./...` green.

## 1. Repository Trimming

- [x] 1.1 Trim `internal/config`: remove `REDIS_URL`, `SQLITE_PATH`, `ADMIN_KEY`; add `MAX_CONCURRENT_RENDERS`, `ADMISSION_WAIT_MS`, `RENDER_TIMEOUT_SECONDS`, `COMPANY_NAME`, `COMPANY_COLOR`, `LOG_LEVEL`, `CHROME_PATH`.
- [x] 1.2 Trim `internal/domain` to the API surface: request gains optional `id`; `CreateResponse` becomes `{invoice_id, url}`; drop `Status` and `StatusResponse`.
- [x] 1.3 Refactor `internal/renderer` behind an engine registry: `Generate(ctx, engine, Options, payload)`; remove `hasCJK` sniffing; register `basic` and `html`.
- [x] 1.4 Extend `internal/validate`: payload structure checks (items array shape, numeric quantity/rate/amount, item cap), id format, engine values.
- [x] 1.5 Align `Dockerfile` builder Go version with `go.mod` (currently `golang:1.24-alpine` vs `go 1.26`).
- [x] 1.6 `docker-compose.yml`: drop the `redis` service and volume; keep MinIO + server. Trim `.env.example`. Make `make dev` work without a committed `.env` (copy `.env.example` when missing).
  - Verify: `go build ./...`, `go vet ./...`, `go test ./...`; `docker compose up`; `curl /health` returns ok.

## 2. Synchronous API

- [x] 2.1 Add `internal/api` with the request pipeline composed in one place (orchestrator file): validate -> semaphore -> render -> upload -> respond.
- [x] 2.2 Implement id handling: ULID generation when omitted; regex validation; id = storage key.
- [x] 2.3 Implement API-key middleware (`X-API-Key`, constant-time compare); fail fast at startup when `API_KEY` is empty.
- [x] 2.4 Implement the error mapping layer exactly per design D8, JSON body `{"error","code"}`.
- [x] 2.5 Implement `GET /invoices/{id}`: streaming download, `application/pdf`, 404 mapping, no redirect, no provider leakage.
- [x] 2.6 Implement the render semaphore: admission wait, `503` + `Retry-After` on exhaustion.
- [x] 2.7 Wire the server: routes, request body limit (1 MiB), read/write timeouts, graceful shutdown (drain up to 10s).
- [x] 2.8 Unit tests: validation, id rules, error mapping, admission behavior with a fake engine and fake storage.
  - Verify: end-to-end smoke against MinIO (POST -> 200 -> GET returns `%PDF` bytes, response has no `Location` header); 100-concurrent burst with zero failures; `503` when slots are exhausted. (MinIO smoke + 100-concurrent burst blocked: docker daemon not running; covered by unit tests with fake storage.)

## 3. HTML Engine Hardening

- [x] 3.1 Browser pool: one allocator per replica, one page per render, concurrency bounded by the semaphore, per-render context timeout, relaunch after a browser crash.
- [x] 3.2 Runtime image: Chromium + `fonts-noto-cjk` installed; `CHROME_PATH` respected; keep `CGO_ENABLED=0`.
- [ ] 3.3 Integration test: CJK invoice rendered end-to-end inside the container. (blocked: docker daemon not running; local CJK e2e + crash-recovery covered by renderer tests)
  - Verify: CJK PDF through the API in the container; memory ceiling holds under N concurrent html renders; crash-recovery test passes.

## 4. Storage Hardening

- [x] 4.1 Upload: content type `application/pdf`, two bounded retries with backoff, errors mapped to `502`.
- [x] 4.2 Download: streaming get, body always closed, `Content-Length` passed through when known.
- [x] 4.3 Bucket bootstrap: `HeadBucket` then create when missing; warn-only on failure.
  - Verify: integration tests green; stopping MinIO mid-run yields `502` for POST and `404`/`502` posture for GET per design. (Fake-S3 tests cover retries, not-found mapping, bootstrap; live MinIO check blocked: docker daemon not running.)

## 5. Operations

- [x] 5.1 Structured JSON request logs with the exact field set from design D10 (no payload contents).
- [x] 5.2 Prometheus `/metrics` with the counters/histograms/gauges from design D10 (`prometheus/client_golang`).
- [x] 5.3 `/health` liveness endpoint that never touches storage.
- [x] 5.4 Rewrite `README.md` for the trimmed product; slim `AGENTS.md` to a pointer at `openspec/` plus working rules.
  - Verify: log line shape asserted in a test; metrics scrape returns all series; docs review against the spec.

## 6. Load and Acceptance

- [ ] 6.1 Commit `scripts/loadtest` k6 scripts: sustained (500/min, 10 min) and burst (ramp to 5x) for both engines.
- [ ] 6.2 Run sustained load on a 2 vCPU replica; tune `MAX_CONCURRENT_RENDERS`; record results.
- [ ] 6.3 Run the burst scenario; confirm only `503` degradation, no crash, full recovery.
- [ ] 6.4 Record measured capacity and scale-out guidance in `design.md` and `README.md`.
  - Verify: acceptance criteria met (zero failures sustained, p99 budgets per invoice-api spec, recovery after burst).

## 7. Validation

- [ ] 7.1 Run OpenSpec validation for this change (strict) and resolve all findings.
- [ ] 7.2 Run `go build ./...`, `go vet ./...`, `go test ./...` on the final tree.
