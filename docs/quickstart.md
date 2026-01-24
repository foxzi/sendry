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

## Configuration Wizard

The easiest way to create a configuration is using the init command:

```bash
# Interactive mode - prompts for values
sendry init

# Non-interactive with flags
sendry init --domain example.com --hostname mail.example.com --dkim

# Quick sandbox setup for testing
sendry init --domain test.local --mode sandbox -o test.yaml
```

The wizard will:
- Generate a complete configuration file
- Optionally create DKIM keys
- Show all DNS records you need to add (SPF, DKIM, DMARC)
- Generate secure API key and SMTP password

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

### Via CLI (Recommended)

```bash
# Send test email through local server
sendry test send -c config.yaml --to user@example.com

# With custom subject and body
sendry test send -c config.yaml --to user@example.com --subject "Test" --body "Hello!"

# Skip TLS for local testing without certificates
sendry test send -c config.yaml --to user@example.com --no-tls

# Use specific port
sendry test send -c config.yaml --to user@example.com --port 2525
```

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

## Email Templates

### Create a Template

```bash
# Via CLI
sendry template create -c config.yaml --name welcome --subject "Hello {{.Name}}" --text welcome.txt

# Via API
curl -X POST http://localhost:8080/api/v1/templates \
  -H "X-API-Key: test-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "welcome",
    "subject": "Hello {{.Name}}",
    "text": "Welcome {{.Name}}!\nYour order #{{.OrderID}} is confirmed.",
    "html": "<p>Welcome <b>{{.Name}}</b>!</p>"
  }'
```

### Preview Template

```bash
# Via CLI
sendry template preview -c config.yaml welcome --data '{"Name":"John","OrderID":"12345"}'

# Via API
curl -X POST http://localhost:8080/api/v1/templates/{id}/preview \
  -H "X-API-Key: test-api-key" \
  -H "Content-Type: application/json" \
  -d '{"data":{"Name":"John","OrderID":"12345"}}'
```

### Send Email by Template

```bash
curl -X POST http://localhost:8080/api/v1/send/template \
  -H "X-API-Key: test-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "template_id": "{id}",
    "from": "noreply@example.com",
    "to": ["user@example.com"],
    "data": {"Name": "John", "OrderID": "12345"}
  }'
```

See [Templates Guide](templates.md) for more details.

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

### Using Init Wizard (Recommended)

```bash
# Full production setup with DKIM and Let's Encrypt
sendry init --domain example.com --dkim --acme --acme-email admin@example.com

# Or interactive mode
sendry init
```

### Manual Setup

For manual configuration, see [full configuration example](../configs/sendry.example.yaml).

Key steps for production:
1. Configure TLS certificates or enable ACME
2. Set up DKIM signing
3. Enable authentication
4. Configure proper rate limits
5. Set domain mode to `production`

### Generate DKIM Key (Manual)

```bash
sendry dkim generate --domain example.com --selector sendry --out /var/lib/sendry/dkim/
```

Add the DNS TXT record shown in the output.

### Check IP Reputation

Before going to production, check if your server IP is blacklisted:

```bash
sendry ip check <your-server-ip>
```

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
- [Templates Guide](templates.md)
- [API Reference](api.md)
- [Configuration Reference](configuration.md)
