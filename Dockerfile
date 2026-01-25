# Universal Dockerfile for sendry and sendry-web
# Usage:
#   docker build --build-arg TARGET=sendry -t sendry .
#   docker build --build-arg TARGET=sendry-web -t sendry-web .

# Build stage
FROM golang:1.24-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    git make ca-certificates gcc libc6-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build arguments
ARG TARGET=sendry
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Build binary
# sendry-web requires CGO for SQLite, sendry doesn't
RUN if [ "$TARGET" = "sendry-web" ]; then \
        CGO_ENABLED=1 GOOS=linux go build \
            -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
            -o /app ./cmd/sendry-web; \
    else \
        CGO_ENABLED=0 GOOS=linux go build \
            -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
            -o /app ./cmd/sendry; \
    fi

# Final stage
FROM debian:bookworm-slim

LABEL maintainer="sendry"
LABEL org.opencontainers.image.source="https://github.com/foxzi/sendry"

ARG TARGET=sendry

# Install ca-certificates for TLS and tzdata for timezones
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd -g 1000 sendry && \
    useradd -u 1000 -g sendry -d /var/lib/sendry -s /sbin/nologin sendry

# Copy binary from builder
COPY --from=builder /app /usr/bin/${TARGET}
RUN chmod +x /usr/bin/${TARGET}

# Create directories
RUN mkdir -p /etc/sendry /var/lib/sendry /var/lib/sendry-web /var/log/sendry && \
    chown -R sendry:sendry /var/lib/sendry /var/lib/sendry-web /var/log/sendry

# Copy example configs
COPY configs/sendry.example.yaml /etc/sendry/config.yaml.example
COPY configs/web.example.yaml /etc/sendry/web.yaml.example

# Expose ports
# sendry: 25 (SMTP), 465 (SMTPS), 587 (Submission), 8080 (API), 9090 (Metrics)
# sendry-web: 8088
EXPOSE 25 465 587 8080 8088 9090

# Volumes
VOLUME ["/var/lib/sendry", "/var/lib/sendry-web", "/etc/sendry"]

# Switch to non-root user
USER sendry

# Set target as environment variable for entrypoint
ENV TARGET=${TARGET}

# Default command (override in docker-compose or docker run)
CMD ["sh", "-c", "exec /usr/bin/${TARGET} serve --config /etc/sendry/config.yaml"]
