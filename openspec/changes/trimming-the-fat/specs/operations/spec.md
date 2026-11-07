# Delta for Operations

## Purpose

Run the service as a stateless, observable, horizontally scalable binary with a small
documented configuration surface, in a container image that supports both engines, verified
against the capacity target.

## ADDED Requirements

### Requirement: Stateless Deployment

The running process MUST NOT keep request-relevant runtime state in memory or on local disk.
Any replica MUST be able to serve any request, including downloads for invoices created by
another replica. Scale-out MUST be adding replicas behind a load balancer; rolling deploys
MUST NOT require draining. This MUST hold without external coordination (no Redis, no
shared database).

#### Scenario: Cross-replica download

- **WHEN** invoice N is created on replica A and requested from replica B
- **THEN** the download succeeds

#### Scenario: Rolling deploy

- **WHEN** one replica is restarted while others stay up
- **THEN** in-flight requests on the restarting replica drain up to the shutdown deadline and
  new requests succeed on the remaining replicas

### Requirement: Graceful Shutdown

On SIGTERM/SIGINT the service MUST stop accepting new requests, MUST allow in-flight requests
to finish within a bounded deadline (default 10s), and MUST then exit.

#### Scenario: Shutdown during load

- **WHEN** the process receives SIGTERM while requests are in flight
- **THEN** those requests complete or fail within the deadline
- **AND** the process exits cleanly

### Requirement: Structured Logging

The service MUST emit one JSON log line per request with at least: `time`, `level`,
`request_id` (honoring an inbound `X-Request-ID` when present), `invoice_id`, `engine`,
`status`, `http_status`, `wait_ms`, `render_ms`, `upload_ms`, `total_ms`, `bytes`, and
`error` when applicable. Payload contents MUST NOT be logged.

#### Scenario: Request log shape

- **WHEN** any invoice request completes
- **THEN** exactly one structured line describes it with the fields above

#### Scenario: No PII

- **WHEN** logs are inspected after rendering an invoice with customer data
- **THEN** no invoice field values appear

### Requirement: Metrics

`/metrics` MUST expose Prometheus-format metrics: `http_requests_total{endpoint,status}`,
`render_duration_seconds{engine}`, `upload_duration_seconds`, `render_wait_seconds`, and
`renders_in_flight`. The endpoint MUST NOT require authentication.

#### Scenario: Scrape

- **WHEN** an operator scrapes `/metrics` during load
- **THEN** request counts, latency histograms, and in-flight renders are present and current

### Requirement: Health

`GET /health` MUST return `200` with `{"status":"ok"}` whenever the process is serving. It
MUST NOT depend on storage reachability so that storage blips cannot flap load balancers.

#### Scenario: Storage down, service up

- **WHEN** storage is unreachable but the process is running
- **THEN** `/health` still returns `200`

### Requirement: Configuration Surface

The service MUST read only the variables documented in the design doc. It MUST refuse to
start when `API_KEY` is empty. Legacy variables from the superseded async design
(`REDIS_URL`, `SQLITE_PATH`, `ADMIN_KEY`, `R2_*`, `STORAGE_BACKEND`) MUST be removed from
code, `.env.example`, and compose.

#### Scenario: Missing API key

- **WHEN** the service starts without `API_KEY`
- **THEN** it exits immediately with a clear error

#### Scenario: No dead config

- **WHEN** the configuration code and docs are inspected
- **THEN** every documented variable is read somewhere and every read variable is documented

### Requirement: Container and Dev Environment

The repository MUST ship a single runtime image containing Chromium and CJK fonts so both
engines work in production. `docker compose up` MUST provide MinIO plus the service, with no
Redis. `make dev` MUST work on a fresh clone without a committed `.env` file.

#### Scenario: Fresh clone

- **WHEN** a developer runs `make dev` on a fresh clone
- **THEN** MinIO and the service start
- **AND** `/health` responds

#### Scenario: Container CJK

- **WHEN** an `engine: html` CJK invoice is requested against the container
- **THEN** it renders correctly

### Requirement: Capacity Verification

The repository MUST commit load-test scripts and record measured results for: sustained
500 invoices/minute for 10 minutes with zero failures, and a burst at 5x target with only
`503` degradation, no crash, and full recovery. Results MUST be recorded in the design doc
along with the replica spec used.

#### Scenario: Sustained proof

- **WHEN** the sustained script runs against a 2 vCPU replica
- **THEN** results are committed with latency percentiles and failure counts

#### Scenario: Burst proof

- **WHEN** the burst script runs
- **THEN** the results show bounded degradation (503s only) and recovery to normal latency

### Requirement: Documentation

`README.md` MUST describe the trimmed product and its three commands (dev, build, test).
`AGENTS.md` MUST become a short pointer to `openspec/` plus working rules, and MUST mark the
async design as superseded and parked.

#### Scenario: Docs match reality

- **WHEN** a new contributor reads README and AGENTS
- **THEN** the documented endpoints, configuration, and workflow match the code
