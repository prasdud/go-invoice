# architecture
- Invoice Generation Microservice: Go-based standalone service, separate from order processing engine. Confidence: 0.70
- Use Redis Streams with consumer groups for job queue (DLQ pattern, no extra infra). Confidence: 0.70
- Use SQLite for permanent audit log only, not for runtime state. Confidence: 0.70
- PDF generation is always asynchronous — POST returns immediately with invoice_id and polling URL, rendering happens via worker pool. Confidence: 0.70
- Never expose S3/R2 URLs to clients — always proxy PDF bytes through the service via URL masking endpoint. Confidence: 0.70
- Invoice payload is stored in Redis (with TTL) and SQLite (durable) at job creation for recovery and audit. Confidence: 0.70

# workflow
- Deliver incrementally using a phase-by-phase MVP approach — keep each phase small, simple, and verifiable before moving to the next. Confidence: 0.85

# storage
- Use a single S3-compatible API for all object storage backends (S3, R2, MinIO) — configure via endpoint URL and credentials, no separate code paths per provider. Confidence: 0.75
