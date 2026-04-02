#!/usr/bin/env bash
set -euo pipefail

echo "==> Running go vet..."
go vet ./...

echo "==> Running tests..."
go test ./...

echo "==> Checking service coverage..."
go test ./internal/service/... -coverprofile=service.cover.out
total=$(go tool cover -func=service.cover.out | awk '/total:/ {gsub("%", "", $3); print $3}')
awk -v total="$total" 'BEGIN { if (total < 70) { printf("FAIL: service coverage %.2f%% < 70%%\n", total); exit 1 } else { printf("OK: service coverage %.2f%%\n", total) } }'
rm -f service.cover.out

echo "==> Checking wire generation..."
go run github.com/google/wire/cmd/wire@v0.7.0 ./internal
git diff --exit-code -- internal/wire_gen.go

echo "==> Checking swagger generation..."
go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/server/main.go -o docs --parseDependency --parseInternal
git diff --exit-code -- docs

echo "==> All checks passed."
