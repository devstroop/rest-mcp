# Multi-stage build for minimal image
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=docker" -o /rest-mcp ./cmd/rest-mcp

# --- Runtime ---
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /rest-mcp /usr/local/bin/rest-mcp

# Non-root user
RUN adduser -D -h /app restmcp
USER restmcp
WORKDIR /app

ENTRYPOINT ["rest-mcp"]
