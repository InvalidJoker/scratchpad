#!/usr/bin/env bash
# Build the sp binary with version information stamped in.
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
OUTPUT="${OUTPUT:-./sp}"

go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o "${OUTPUT}" ./cmd/scratchpad

echo "built ${OUTPUT} (${VERSION})"
