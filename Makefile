.PHONY: up down logs demo dev-api dev-web fmt lint test mocks

up:
	docker compose up --build -d --wait

down:
	docker compose down -v

logs:
	docker compose logs -f

demo:
	./scripts/demo.sh

dev-api:
	go run ./cmd/api

dev-web:
	cd web && npm install && npm run dev

fmt:
	golangci-lint fmt

lint:
	golangci-lint run

test:
	go test -race ./...

mocks:
	mockery
