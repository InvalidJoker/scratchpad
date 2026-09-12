.PHONY: build test lint fmt install snapshot clean

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

install:
	go install -trimpath -ldflags "-s -w -X main.version=$$(git describe --tags --always --dirty 2>/dev/null || echo dev)" ./cmd/scratchpad

# Build the full set of release artefacts locally, without tagging or publishing.
snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -f ./sp
	rm -rf ./dist ./completions
