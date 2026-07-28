# ── Stage 1: Builder ──────────────────────────────────────────────────────────
FROM golang:1.26.5-alpine AS builder

WORKDIR /app

# Download dependencies first (layer cache)
COPY go.mod go.sum ./
RUN go mod download

# Commit SHA of the build, stamped into the binary so the running image can be
# traced back to its source. Defaults to "dev" for local builds.
ARG VERSION=dev

# Build the server binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X github.com/kidsaber/kidsaber-play-api/internal/version.Version=${VERSION}" \
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
