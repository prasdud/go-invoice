FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN if [ -f go.sum ]; then go mod download; fi
COPY . .
RUN CGO_ENABLED=0 go build -o server ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates \
      chromium \
      fonts-noto-cjk \
    && rm -rf /var/lib/apt/lists/*
ENV CHROME_PATH=/usr/bin/chromium
COPY --from=builder /app/server /server
EXPOSE 8080
CMD ["/server"]
