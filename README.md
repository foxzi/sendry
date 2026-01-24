# Sendry

MTA (Mail Transfer Agent) server for sending emails.

[Русская версия](docs/README.ru.md)

## Features

- SMTP server (ports 25, 587) with AUTH support
- SMTPS server (port 465) with implicit TLS
- STARTTLS support for secure connections
- Let's Encrypt (ACME) automatic certificate management
- DKIM signing for outgoing emails
- HTTP API for sending emails
- Persistent queue with BoltDB
- Retry logic with exponential backoff
- Graceful shutdown
- Structured JSON logging

## Requirements

- Go 1.23+

## Quick Start

### Build

```bash
go build -o sendry ./cmd/sendry
```

### Configuration

Copy and edit the example configuration:

```bash
cp configs/sendry.example.yaml config.yaml
```

Minimal configuration:

```yaml
server:
  hostname: "mail.example.com"

smtp:
  domain: "example.com"
  auth:
    required: true
    users:
      myuser: "mypassword"

api:
  api_key: "your-secret-api-key"

storage:
  path: "./data/queue.db"
```

### Run

```bash
./sendry serve -c config.yaml
```

### Validate Configuration

```bash
./sendry config validate -c config.yaml
```

## API

### Health Check

```bash
curl http://localhost:8080/health
```

Response:
```json
{
  "status": "ok",
  "version": "0.1.0",
  "uptime": "1h30m",
  "queue": {
    "pending": 5,
    "sending": 1,
    "delivered": 100,
    "failed": 2,
    "deferred": 3,
    "total": 111
  }
}
```

### Send Email

```bash
curl -X POST http://localhost:8080/api/v1/send \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "from": "sender@example.com",
    "to": ["recipient@example.com"],
    "subject": "Test",
    "body": "Hello, World!",
    "html": "<p>Hello, <b>World</b>!</p>"
  }'
```

Response:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending"
}
```

### Check Status

```bash
curl http://localhost:8080/api/v1/status/{message_id} \
  -H "Authorization: Bearer YOUR_API_KEY"
```

Response:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "delivered",
  "from": "sender@example.com",
  "to": ["recipient@example.com"],
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:30:05Z",
  "retry_count": 0
}
```

### Queue Stats

```bash
curl http://localhost:8080/api/v1/queue \
  -H "Authorization: Bearer YOUR_API_KEY"
```

### Delete Message

```bash
curl -X DELETE http://localhost:8080/api/v1/queue/{message_id} \
  -H "Authorization: Bearer YOUR_API_KEY"
```

## Configuration Reference

| Parameter | Default | Description |
|-----------|---------|-------------|
| `server.hostname` | OS hostname | Server FQDN |
| `smtp.listen_addr` | `:25` | SMTP relay port |
| `smtp.submission_addr` | `:587` | SMTP submission port |
| `smtp.smtps_addr` | `:465` | SMTPS port (implicit TLS) |
| `smtp.domain` | *required* | Mail domain |
| `smtp.max_message_bytes` | `10485760` | Max message size (10MB) |
| `smtp.max_recipients` | `100` | Max recipients per message |
| `smtp.auth.required` | `false` | Require authentication |
| `smtp.auth.users` | `{}` | Username -> password map |
| `smtp.tls.cert_file` | `""` | TLS certificate file path |
| `smtp.tls.key_file` | `""` | TLS private key file path |
| `smtp.tls.acme.enabled` | `false` | Enable Let's Encrypt |
| `smtp.tls.acme.email` | `""` | ACME account email |
| `smtp.tls.acme.domains` | `[]` | Domains for certificate |
| `smtp.tls.acme.cache_dir` | `/var/lib/sendry/certs` | Certificate cache |
| `dkim.enabled` | `false` | Enable DKIM signing |
| `dkim.selector` | `""` | DKIM selector |
| `dkim.domain` | `""` | DKIM domain |
| `dkim.key_file` | `""` | DKIM private key path |
| `api.listen_addr` | `:8080` | HTTP API port |
| `api.api_key` | `""` | API key (empty = no auth) |
| `queue.workers` | `4` | Number of delivery workers |
| `queue.retry_interval` | `5m` | Base retry interval |
| `queue.max_retries` | `5` | Max delivery attempts |
| `storage.path` | `/var/lib/sendry/queue.db` | BoltDB file path |
| `logging.level` | `info` | Log level (debug/info/warn/error) |
| `logging.format` | `json` | Log format (json/text) |

See [TLS and DKIM documentation](docs/tls-dkim.md) for detailed setup instructions.

## Project Structure

```
sendry/
├── cmd/sendry/          # CLI entry point
├── internal/
│   ├── api/             # HTTP API server
│   ├── app/             # Application orchestration
│   ├── config/          # Configuration
│   ├── dkim/            # DKIM signing
│   ├── dns/             # MX resolver
│   ├── queue/           # Message queue & storage
│   ├── smtp/            # SMTP server & client
│   └── tls/             # TLS/ACME support
├── configs/             # Example configurations
└── docs/                # Documentation
```

## License

MIT
