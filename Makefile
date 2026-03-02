.PHONY: build-server build-client build-all clean test fmt lint

build-server:
	go build -o bin/ogrok-server ./cmd/server

build-client:
	go build -o bin/ogrok ./cmd/client

build-all: build-server build-client

clean:
	rm -rf bin/
	go clean

test:
	go test ./...

fmt:
	go fmt ./...

lint:
	golangci-lint run

docker:
	docker build -t ogrok:latest .

dev-server: build-server
	./bin/ogrok-server -config configs/server.yaml

dev-client: build-client
	./bin/ogrok http 3000 --server localhost:8080 --token dev-token-123
