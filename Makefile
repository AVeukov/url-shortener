.PHONY: run test lint up down

run:
	go run ./cmd/api

test:
	go test -race -cover ./...

lint:
	go vet ./...

up:
	docker compose up --build

down:
	docker compose down
