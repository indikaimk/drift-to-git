.PHONY: all build test clean run

BINARY_NAME=bin/drift-to-git

all: build test

build:
	@mkdir -p bin
	go build -o $(BINARY_NAME) ./cmd/drift-to-git

test:
	go test ./... -v

clean:
	rm -rf bin/

run-scan:
	go run ./cmd/drift-to-git scan --dry-run
