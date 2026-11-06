.PHONY: generate build test vet run
generate:
	go generate .
build:
	go build -o bin/orchestrator ./cmd/orchestrator
test:
	go test -race -p 1 ./...
vet:
	go vet ./...
run:
	go run ./cmd/orchestrator
