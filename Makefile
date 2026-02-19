.PHONY: run build test docker-up docker-down migrate-up migrate-down migrate-create

DATABASE_URL ?= postgres://relay:relay@localhost:5432/webhook_relay?sslmode=disable

run:
	go run ./cmd/relay

build:
	go build -o bin/relay ./cmd/relay

test:
	go test ./...

docker-up:
	docker compose up -d

docker-down:
	docker compose down

migrate-up:
	migrate -database "$(DATABASE_URL)" -path migrations up

migrate-down:
	migrate -database "$(DATABASE_URL)" -path migrations down

migrate-create:
	@read -p "Migration name: " name; \
	migrate create -ext sql -dir migrations -seq $$name
