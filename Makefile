.PHONY: dev build run test clean

dev:
	docker compose up -d minio
	@echo "MinIO:  localhost:9000 (API), localhost:9001 (Console)"

dev-full:
	docker compose up -d --profile full
	@echo "Redis:  localhost:6380"
	@echo "MinIO:  localhost:9000 (API), localhost:9001 (Console)"

down:
	docker compose down

build:
	go build -o bin/server ./cmd/server

run: build
	./bin/server

test:
	go test ./...

tidy:
	go mod tidy

vet:
	go vet ./...

clean:
	rm -rf bin/
	docker compose down -v
