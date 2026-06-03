# Taste (Continuously Learned by [CommandCode][cmd])

[cmd]: https://commandcode.ai/

# architecture
See [architecture/taste.md](architecture/taste.md)

# shell
- Avoid brace expansion in mkdir commands — shells may interpret braces literally, creating garbage directory names. Use explicit paths or multiple mkdir -p calls instead. Confidence: 0.70

# docker
- Docker Compose should orchestrate the full dev environment including the service itself, not just infrastructure dependencies — users expect `docker-compose up` to run everything in one command. Confidence: 0.70

# workflow
- Do not start or stop services (Docker containers, Redis, the server binary) — the user manages their own infrastructure and will run services themselves. Confidence: 0.85
- Favor minimal, simple implementations — if the user says a design is overcomplicated, strip it down to the essentials. One endpoint, one function, one API — nothing speculative. Confidence: 0.75

# architecture
- Use a workflow/orchestrator pattern where the request flow (validation, rendering, side effects) is composed in a single file — adding or removing steps should only require changes in one place. Confidence: 0.70

# go
- Separate type/struct definitions into their own package and utility functions (validators, helpers) into a separate lib/util package — follow idiomatic Go project layout. Confidence: 0.70
