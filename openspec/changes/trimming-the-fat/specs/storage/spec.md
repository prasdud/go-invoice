# Delta for Storage

## Purpose

Persist generated PDFs in S3-compatible object storage (AWS S3, Cloudflare R2, MinIO) and
stream them back on demand. Storage is the only persistence in the system; it MUST never be
exposed directly to clients.

## ADDED Requirements

### Requirement: Artifact Layout

Invoices MUST be stored at the flat key `invoices/{id}.pdf`. Keys MUST NOT include date
prefixes or any lookup-dependent structure: the key is a pure function of the invoice id.
Objects MUST be private. Expiry and lifecycle rules are configured in the storage console,
never in service code.

#### Scenario: Key derivation

- **WHEN** invoice `01J8Z...` is generated
- **THEN** the artifact is stored at `invoices/01J8Z....pdf`

#### Scenario: Lifecycle

- **WHEN** a lifecycle rule deletes an expired object
- **THEN** the download endpoint returns `404` and no code change is required

### Requirement: Upload

Uploads MUST set content type `application/pdf` and MUST be retried twice with short backoff
before failing. The service MUST respond success only after storage confirms the write.
Failures after retries MUST surface as `storage_unavailable` (502) and MUST NOT leave
dangling references.

#### Scenario: Transient blip

- **WHEN** the first upload attempt fails and the retry succeeds
- **THEN** the request completes normally

#### Scenario: Persistent failure

- **WHEN** all upload attempts fail
- **THEN** the API responds `502 storage_unavailable`

### Requirement: Streaming Download

The download path MUST fetch the object and stream its bytes to the client without loading
the entire object into memory, MUST always close the storage response body, and MUST pass
through `Content-Length` when the provider supplies it. It MUST NOT generate presigned URLs
and MUST NOT expose endpoint, host, bucket, or key to the client in any header or body.

#### Scenario: Large PDF

- **WHEN** a multi-megabyte PDF is downloaded
- **THEN** bytes stream to the client with stable memory usage

#### Scenario: No provider leakage

- **WHEN** any download response is inspected
- **THEN** it contains no storage host, bucket name, or provider identifier

### Requirement: Bucket Bootstrap

At startup the service MUST check the configured bucket with `HeadBucket` and attempt
creation when missing. Failure MUST be logged as a warning and MUST NOT prevent the service
from starting.

#### Scenario: Fresh MinIO

- **WHEN** the service starts against an empty MinIO with default configuration
- **THEN** bucket `invoices` exists after startup
- **AND** the first invoice request succeeds

#### Scenario: No permission to create

- **WHEN** bucket creation is denied
- **THEN** the service still starts and logs a warning

### Requirement: Provider Configuration

Storage MUST be configured entirely by environment (`S3_ENDPOINT`, `S3_REGION`, `S3_BUCKET`,
`S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_USE_PATH_STYLE`). Path-style MUST be supported for
MinIO. Credentials MUST never appear in logs or API responses.

#### Scenario: MinIO compatibility

- **WHEN** `S3_ENDPOINT` points at MinIO with path-style enabled
- **THEN** upload and download work without code changes

#### Scenario: Credential hygiene

- **WHEN** logs and error responses are inspected
- **THEN** no access key or secret appears
