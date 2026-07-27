# ── Stage 1: Builder ──────────────────────────────────────────────────────────
FROM golang:1.26.5-alpine AS builder

WORKDIR /app

# Download dependencies first (layer cache)
COPY go.mod go.sum ./
RUN go mod download

# Build the server binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /app/server \
    ./cmd/server

# ── Stage 2: Final minimal image ─────────────────────────────────────────────
FROM alpine:3.20

# Security: run as non-root user
RUN addgroup -S nonroot && adduser -S nonroot -G nonroot

WORKDIR /app

# Copy only the binary from the builder stage
COPY --from=builder /app/server /app/server

# TLS certificates (needed for outbound HTTPS to LLM providers and Neon PostgreSQL)
RUN apk add --no-cache ca-certificates

USER nonroot

EXPOSE 8080

ENTRYPOINT ["/app/server"]
