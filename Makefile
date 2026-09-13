.PHONY: test build run docker-up docker-down

test:
	go test ./...

build:
	go build -o bin/wick ./cmd/wick

run: build
	WICK_DATA=data WICK_LISTEN=:2222 ./bin/wick

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down
