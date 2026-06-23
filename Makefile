.PHONY: build run test generate-batch clean

build:
	go build -o bin/server ./cmd/server

run: build
	./bin/server

test:
	go test ./... -race -cover

generate-batch:
	go run ./scripts/generate_batch.go 1000

clean:
	rm -rf bin coverage.out
	go clean -testcache
