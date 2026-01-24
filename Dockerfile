# Build stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git make ca-certificates tzdata

WORKDIR /src

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /sendry ./cmd/sendry

# Final stage
FROM alpine:3.19

LABEL maintainer="sendry"
LABEL description="Sendry - Outbound Mail Transfer Agent"
LABEL org.opencontainers.image.source="https://github.com/foxzi/sendry"

# Install ca-certificates for TLS and tzdata for timezones
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 sendry && \
    adduser -u 1000 -G sendry -h /var/lib/sendry -s /sbin/nologin -D sendry

# Copy binary from builder
COPY --from=builder /sendry /usr/bin/sendry
RUN chmod +x /usr/bin/sendry

# Create directories
RUN mkdir -p /etc/sendry /var/lib/sendry /var/log/sendry && \
    chown -R sendry:sendry /var/lib/sendry /var/log/sendry

# Copy example config
COPY configs/sendry.example.yaml /etc/sendry/config.yaml.example

# Expose ports
# 25   - SMTP
# 465  - SMTPS (implicit TLS)
# 587  - Submission (STARTTLS)
# 8080 - HTTP API
# 9090 - Metrics
EXPOSE 25 465 587 8080 9090

# Volumes
VOLUME ["/var/lib/sendry", "/etc/sendry"]

# Switch to non-root user
USER sendry

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ["/usr/bin/sendry", "version"]

# Default command
ENTRYPOINT ["/usr/bin/sendry"]
CMD ["serve", "--config", "/etc/sendry/config.yaml"]
