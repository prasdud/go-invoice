# Taste (Continuously Learned by [CommandCode][cmd])

[cmd]: https://commandcode.ai/

# architecture
See [architecture/taste.md](architecture/taste.md)

# shell
- Avoid brace expansion in mkdir commands — shells may interpret braces literally, creating garbage directory names. Use explicit paths or multiple mkdir -p calls instead. Confidence: 0.70

# docker
- Docker Compose should orchestrate the full dev environment including the service itself, not just infrastructure dependencies — users expect `docker-compose up` to run everything in one command. Confidence: 0.70
