#!/usr/bin/env bash
# Regenerate shell completions into completions/.
set -euo pipefail

cd "$(dirname "$0")/.."

BIN="$(mktemp -d)/sp"
go build -o "${BIN}" ./cmd/scratchpad

mkdir -p completions
for shell in bash zsh fish; do
  "${BIN}" completion "${shell}" > "completions/sp.${shell}"
  echo "wrote completions/sp.${shell}"
done
