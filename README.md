# Sendry

MTA (Mail Transfer Agent) server for sending emails.

## Features

- SMTP server (ports 25, 587)
- HTTP API for sending emails
- Persistent queue with BoltDB
- Retry logic with exponential backoff
- SMTP AUTH support

## Quick Start

### Build

```bash
go build -o sendry ./cmd/sendry
```

### Configuration

Copy and edit the example configuration:

```bash
cp configs/sendry.example.yaml config.yaml
# Edit config.yaml with your settings
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

### Send Email

```bash
curl -X POST http://localhost:8080/api/v1/send \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "from": "sender@example.com",
    "to": ["recipient@example.com"],
    "subject": "Test",
    "body": "Hello, World!"
  }'
```

### Check Status

```bash
curl http://localhost:8080/api/v1/status/{message_id} \
  -H "Authorization: Bearer YOUR_API_KEY"
```

### Queue Stats

```bash
curl http://localhost:8080/api/v1/queue \
  -H "Authorization: Bearer YOUR_API_KEY"
```

## License

MIT
