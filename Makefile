.PHONY: tidy fmt lint test vuln coverage-service run wire wire-check swagger swagger-check migrate-up migrate-down migrate-version migrate-step

tidy:
	go mod tidy

fmt:
	gofmt -w ./cmd ./internal ./pkg ./migrations ./docs

lint:
	golangci-lint run --timeout=5m

test:
	go test ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...

coverage-service:
	go test ./internal/service/... -coverprofile=service.cover.out
	go tool cover -func=service.cover.out

run:
	go run ./cmd/server

wire:
	go run github.com/google/wire/cmd/wire@v0.7.0 ./internal

wire-check:
	go run github.com/google/wire/cmd/wire@v0.7.0 ./internal
	git diff --exit-code -- internal/wire_gen.go

swagger:
	go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/server/main.go -o docs --parseDependency --parseInternal

swagger-check:
	go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/server/main.go -o docs --parseDependency --parseInternal
	git diff --exit-code -- docs

migrate-up:
	go run ./cmd/migrate -action up

migrate-down:
	go run ./cmd/migrate -action down

migrate-version:
	go run ./cmd/migrate -action version

migrate-step:
	go run ./cmd/migrate -action steps -steps 1
