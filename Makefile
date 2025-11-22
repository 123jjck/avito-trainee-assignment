BINARY=pr-service

.PHONY: build run test docker-up docker-down

build:
	mkdir -p bin
	go build -o bin/$(BINARY) ./cmd/server

run:
	go run ./cmd/server

test:
	go test ./...

docker-up:
	docker compose up --build

docker-down:
	docker compose down -v
