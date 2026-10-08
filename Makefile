.PHONY: up down logs test fmt migrate demo load

up:
	docker compose up --build

down:
	docker compose down

logs:
	docker compose logs -f api worker scheduler otel-collector

test:
	docker compose run --rm api go test ./...

fmt:
	docker compose run --rm api gofmt -w ./cmd ./internal

migrate:
	docker compose run --rm migrate

demo:
	powershell -ExecutionPolicy Bypass -File scripts/demo.ps1

load:
	go run ./cmd/loadtest -jobs 5000 -concurrency 100 -timeout 5m
