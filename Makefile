.PHONY: build test lint fmt completions install clean

build:
	@./scripts/build.sh

test:
	go test -race ./...

lint:
	go vet ./...
	@test -z "$$(gofmt -l internal cmd)" || (gofmt -l internal cmd; echo "run make fmt"; exit 1)
	@command -v golangci-lint >/dev/null && golangci-lint run ./... || echo "golangci-lint not installed, skipped"

fmt:
	gofmt -w internal cmd

completions:
	@./scripts/completions.sh

install:
	go install -trimpath -ldflags "-s -w -X main.version=$$(git describe --tags --always --dirty 2>/dev/null || echo dev)" ./cmd/scratchpad

clean:
	rm -f ./sp
