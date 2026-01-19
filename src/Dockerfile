# CHROTE - Multi-stage Dockerfile
# Stage 1: Build the Go binary
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod ./

# Copy source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o chrote-server ./cmd/server

# Stage 2: Runtime
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache tmux ttyd bash curl

# Create working directories
RUN mkdir -p /code /workspace

# Copy binary from builder
COPY --from=builder /app/chrote-server /usr/local/bin/chrote-server

# Create terminal launch script
RUN echo '#!/bin/bash' > /usr/local/bin/terminal-launch.sh && \
    echo 'exec bash -l' >> /usr/local/bin/terminal-launch.sh && \
    chmod +x /usr/local/bin/terminal-launch.sh

# Set environment
ENV TMUX_TMPDIR=/tmp
ENV PORT=8080
ENV TTYD_PORT=7681

EXPOSE 8080

# Health check
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s \
    CMD curl -f http://localhost:8080/api/health || exit 1

CMD ["/usr/local/bin/chrote-server", "--port", "8080", "--ttyd-port", "7681"]
