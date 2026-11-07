.PHONY: dev dev-build down build test tidy vet clean

dev:
	@[ -f .env ] || cp .env.example .env
	docker compose up -d
	@echo "Server:   http://localhost:8080"
	@echo "S3 mock:  http://localhost:9090"

dev-build:
	@[ -f .env ] || cp .env.example .env
	docker compose up -d --build
	@echo "Server:   http://localhost:8080"
	@echo "S3 mock:  http://localhost:9090"

down:
	docker compose down

build:
	go build -o bin/server ./cmd/server

test:
	go test ./...

tidy:
	go mod tidy

vet:
	go vet ./...

clean:
	rm -rf bin/
	docker compose down -v
