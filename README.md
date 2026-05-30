Any agents assisting on this project strictly must read AGENTS.md in the project root.
# The fastest invoice generation in the history of software
<img width="800" height="800" alt="hero" src="https://github.com/user-attachments/assets/6b69487b-1d49-4a67-825c-f43933c2e0d4" />

## Getting Started

### Quick Start

```bash
# 1. Clone
git clone https://github.com/prasdud/go-invoice
cd go-invoice

# 2. Start everything (server + Redis + MinIO)
make dev

# 3. Test
curl http://localhost:8080/health
# → {"status":"ok"}
```

That's it. One command.

| Service | URL |
|---|---|
| Server | http://localhost:8080 |
| MinIO API | http://localhost:9000 |
| MinIO Console | http://localhost:9001 |

MinIO login: `minioadmin` / `minioadmin`

### Configuration

The repo ships with a `.env` that works out of the box. For production, edit it to point at your own S3/R2/MinIO instance. See `.env.example` for all supported variables.
