# Sendry Quick Start Guide

## Installation

### From Binary

```bash
# Download latest release
curl -LO https://github.com/foxzi/sendry/releases/latest/download/sendry-linux-amd64
chmod +x sendry-linux-amd64
sudo mv sendry-linux-amd64 /usr/local/bin/sendry
```

### From Source

```bash
git clone https://github.com/foxzi/sendry.git
cd sendry
make build
sudo cp build/sendry /usr/local/bin/
```

### Using Docker

```bash
docker pull ghcr.io/foxzi/sendry:latest
```

## Quick Test (Sandbox Mode)

Create a test config `test.yaml`:

```yaml
server:
  hostname: "localhost"

smtp:
  listen_addr: ":2525"
  submission_addr: ":2587"
  domain: "test.local"
  auth:
    required: false

domains:
  test.local:
    mode: sandbox

api:
  listen_addr: ":8080"
  api_key: "test-api-key"

queue:
  workers: 2

storage:
  path: "./data/queue.db"

logging:
  level: "debug"
  format: "text"
```

Run the server:

```bash
mkdir -p data
sendry serve -c test.yaml
```

## Sending Test Emails

### Via SMTP

```bash
# Using netcat
echo -e "EHLO test\nMAIL FROM:<test@test.local>\nRCPT TO:<user@example.com>\nDATA\nSubject: Test\n\nHello World\n.\nQUIT" | nc localhost 2525

# Using swaks (if installed)
swaks --to user@example.com --from test@test.local --server localhost:2525
```

### Via HTTP API

```bash
curl -X POST http://localhost:8080/api/v1/send \
  -H "X-API-Key: test-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "from": "api@test.local",
    "to": ["user@example.com"],
    "subject": "Test Email",
    "body": "Hello from Sendry!"
  }'
```

## Viewing Captured Emails (Sandbox Mode)

```bash
# List all messages
curl -H "X-API-Key: test-api-key" http://localhost:8080/api/v1/sandbox/messages

# Get specific message
curl -H "X-API-Key: test-api-key" http://localhost:8080/api/v1/sandbox/messages/{id}

# Get statistics
curl -H "X-API-Key: test-api-key" http://localhost:8080/api/v1/sandbox/stats
```

## Health Check

```bash
curl http://localhost:8080/health
```

## Production Configuration

For production use, see [full configuration example](../configs/sendry.example.yaml).

Key steps for production:
1. Configure TLS certificates or enable ACME
2. Set up DKIM signing
3. Enable authentication
4. Configure proper rate limits
5. Set domain mode to `production`

### Generate DKIM Key

```bash
sendry dkim generate --domain example.com --selector mail --out /var/lib/sendry/dkim/
```

Add the DNS TXT record shown in the output.

## Ports

| Port | Service | Description |
|------|---------|-------------|
| 25 | SMTP | Server-to-server mail transfer |
| 587 | Submission | Client mail submission (STARTTLS) |
| 465 | SMTPS | Client mail submission (implicit TLS) |
| 8080 | HTTP API | REST API for sending and management |

## Domain Modes

| Mode | Description |
|------|-------------|
| `production` | Normal delivery to recipients |
| `sandbox` | Capture emails locally (for testing) |
| `redirect` | Redirect all emails to specified addresses |
| `bcc` | Normal delivery + copy to archive |

## Next Steps

- [TLS and DKIM Setup](tls-dkim.md)
- [API Reference](api.md)
- [Configuration Reference](configuration.md)
