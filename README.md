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
- Multi-domain support with different modes:
  - `production` - normal delivery
  - `sandbox` - capture emails locally (for testing)
  - `redirect` - redirect all emails to specified addresses
  - `bcc` - normal delivery + copy to archive
- Rate limiting (per domain, sender, IP, API key)
- Prometheus metrics with persistence
- Bounce handling
- Graceful shutdown
- Structured JSON logging

## Installation

### From Package (recommended)

Download from [GitHub Releases](https://github.com/foxzi/sendry/releases):

```bash
# Debian/Ubuntu
wget https://github.com/foxzi/sendry/releases/latest/download/sendry_0.3.3-1_amd64.deb
sudo dpkg -i sendry_0.3.3-1_amd64.deb

# RHEL/CentOS
wget https://github.com/foxzi/sendry/releases/latest/download/sendry-0.3.3-1.x86_64.rpm
sudo rpm -i sendry-0.3.3-1.x86_64.rpm

# Alpine
wget https://github.com/foxzi/sendry/releases/latest/download/sendry_0.3.3-r1_x86_64.apk
sudo apk add --allow-untrusted sendry_0.3.3-r1_x86_64.apk
```

### From Binary

```bash
wget https://github.com/foxzi/sendry/releases/latest/download/sendry-linux-amd64
chmod +x sendry-linux-amd64
sudo mv sendry-linux-amd64 /usr/local/bin/sendry
```

### Docker

```bash
docker pull ghcr.io/foxzi/sendry:latest
docker run -p 25:25 -p 587:587 -p 8080:8080 \
  -v /path/to/config.yaml:/etc/sendry/config.yaml \
  ghcr.io/foxzi/sendry:latest
```

### Docker Compose

For running both Sendry MTA and Web management panel, see [Docker documentation](docs/docker.md).

```bash
git clone https://github.com/foxzi/sendry.git
cd sendry
cp configs/sendry.example.yaml configs/sendry.yaml
cp configs/web.example.yaml configs/web.yaml
# Edit configs
docker compose up -d
```

### Ansible

For automated deployment on multiple servers, see [Ansible documentation](docs/ansible.md).

```bash
cd ansible
cp inventory/hosts.yml.example inventory/hosts.yml
# Edit hosts.yml with your servers
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml
```

### Build from Source

Requires Go 1.24+

```bash
git clone https://github.com/foxzi/sendry.git
cd sendry
make build
```

## Quick Start

For detailed quick start guide see [docs/quickstart.md](docs/quickstart.md).

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
  "version": "0.3.3",
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
| `smtp.auth.max_failures` | `5` | Max auth failures before blocking |
| `smtp.auth.block_duration` | `15m` | How long to block after max failures |
| `smtp.auth.failure_window` | `5m` | Window for counting failures |
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
| `api.max_header_bytes` | `1048576` | Max HTTP header size (1MB) |
| `api.read_timeout` | `30s` | HTTP read timeout |
| `api.write_timeout` | `30s` | HTTP write timeout |
| `api.idle_timeout` | `60s` | HTTP idle timeout |
| `queue.workers` | `4` | Number of delivery workers |
| `queue.retry_interval` | `5m` | Base retry interval |
| `queue.max_retries` | `5` | Max delivery attempts |
| `storage.path` | `/var/lib/sendry/queue.db` | BoltDB file path |
| `storage.retention.delivered_max_age` | `0` | Delete delivered messages older than this |
| `storage.retention.cleanup_interval` | `1h` | Cleanup interval |
| `dlq.enabled` | `true` | Enable dead letter queue |
| `dlq.max_age` | `0` | Delete DLQ messages older than this |
| `dlq.max_count` | `0` | Max DLQ messages (0 = unlimited) |
| `dlq.cleanup_interval` | `1h` | DLQ cleanup interval |
| `logging.level` | `info` | Log level (debug/info/warn/error) |
| `logging.format` | `json` | Log format (json/text) |
| `metrics.enabled` | `false` | Enable Prometheus metrics |
| `metrics.listen_addr` | `:9090` | Metrics server port |
| `metrics.path` | `/metrics` | Metrics endpoint path |
| `metrics.flush_interval` | `10s` | Counter persistence interval |
| `metrics.allowed_ips` | `[]` | IPs/CIDRs allowed to access metrics |

See documentation:
- [HTTP API reference](docs/api.md)
- [TLS and DKIM](docs/tls-dkim.md)
- [Message retention and DLQ](docs/retention.md)
- [Rate limiting](docs/ratelimit.md)
- [Prometheus metrics](docs/metrics.md)
- [Ansible deployment](docs/ansible.md)

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
│   ├── metrics/         # Prometheus metrics
│   ├── queue/           # Message queue & storage
│   ├── smtp/            # SMTP server & client
│   └── tls/             # TLS/ACME support
├── configs/             # Example configurations
└── docs/                # Documentation
```

## License

GPL-3.0
