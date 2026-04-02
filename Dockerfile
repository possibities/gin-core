FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/server ./cmd/server

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/server /bin/server
COPY config/ /etc/app/config/
COPY migrations/ /etc/app/migrations/
COPY locales/ /etc/app/locales/

ENV CONFIG_PATH=/etc/app/config

EXPOSE 8080
ENTRYPOINT ["/bin/server"]
