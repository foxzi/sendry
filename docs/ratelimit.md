# Rate Limiting

Sendry provides multi-level rate limiting to protect against abuse and ensure fair resource usage.

## Overview

Rate limiting operates at multiple levels, each with independent counters:

| Level | Description | Example Key |
|-------|-------------|-------------|
| `global` | Server-wide limit | `global` |
| `domain` | Per sending domain | `example.com` |
| `sender` | Per sender email | `user@example.com` |
| `ip` | Per client IP | `192.168.1.1` |
| `api_key` | Per API key | `key-abc123` |

When a message is sent, all applicable limits are checked. If any limit is exceeded, the message is rejected.

## Configuration

```yaml
rate_limit:
  enabled: true

  # Global server limits
  global:
    messages_per_hour: 50000
    messages_per_day: 500000

  # Default limits per sending domain
  default_domain:
    messages_per_hour: 1000
    messages_per_day: 10000

  # Default limits per sender email
  default_sender:
    messages_per_hour: 100
    messages_per_day: 1000

  # Default limits per client IP
  default_ip:
    messages_per_hour: 500
    messages_per_day: 5000

  # Default limits per API key
  default_api_key:
    messages_per_hour: 1000
    messages_per_day: 10000
```

### Per-Domain Overrides

Override default limits for specific domains:

```yaml
domains:
  example.com:
    rate_limit:
      messages_per_hour: 5000
      messages_per_day: 50000
      recipients_per_message: 100

  newsletter.example.com:
    rate_limit:
      messages_per_hour: 10000
      messages_per_day: 100000
```

## How It Works

### Counter Windows

- **Hourly counter**: Resets every hour from when the first message was sent
- **Daily counter**: Resets every 24 hours from when the first message was sent

### Limit Evaluation Order

When a message is sent, limits are checked in this order:

1. Global limit
2. Domain limit
3. Sender limit
4. IP limit
5. API key limit

The first limit that is exceeded causes rejection. All counters for allowed messages are incremented atomically.

### Zero Value

Setting a limit to `0` means unlimited:

```yaml
rate_limit:
  global:
    messages_per_hour: 0     # Unlimited hourly
    messages_per_day: 100000 # But limited daily
```

### Persistence

Rate limit counters are persisted to BoltDB and survive server restarts. The flush interval is configurable:

```yaml
rate_limit:
  flush_interval: 10s  # Default: 10s
```

## API Endpoints

### Get Rate Limit Configuration

```bash
curl http://localhost:8080/api/v1/ratelimits \
  -H "Authorization: Bearer YOUR_API_KEY"
```

Response:
```json
{
  "enabled": true,
  "global": {
    "messages_per_hour": 50000,
    "messages_per_day": 500000
  },
  "default_domain": {
    "messages_per_hour": 1000,
    "messages_per_day": 10000
  },
  "domains": {
    "example.com": {
      "messages_per_hour": 5000,
      "messages_per_day": 50000,
      "recipients_per_message": 100
    }
  }
}
```

### Get Rate Limit Stats

Get current counter values for a specific level and key:

```bash
# Global stats
curl http://localhost:8080/api/v1/ratelimits/global/global \
  -H "Authorization: Bearer YOUR_API_KEY"

# Domain stats
curl http://localhost:8080/api/v1/ratelimits/domain/example.com \
  -H "Authorization: Bearer YOUR_API_KEY"

# Sender stats
curl http://localhost:8080/api/v1/ratelimits/sender/user@example.com \
  -H "Authorization: Bearer YOUR_API_KEY"

# IP stats
curl http://localhost:8080/api/v1/ratelimits/ip/192.168.1.1 \
  -H "Authorization: Bearer YOUR_API_KEY"

# API key stats
curl http://localhost:8080/api/v1/ratelimits/api_key/key-123 \
  -H "Authorization: Bearer YOUR_API_KEY"
```

Response:
```json
{
  "level": "domain",
  "key": "example.com",
  "hourly_count": 150,
  "daily_count": 1200,
  "hourly_limit": 5000,
  "daily_limit": 50000,
  "hour_start": "2024-01-15T10:00:00Z",
  "day_start": "2024-01-15T00:00:00Z"
}
```

### Update Domain Rate Limits

```bash
curl -X PUT http://localhost:8080/api/v1/ratelimits/example.com \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "messages_per_hour": 2000,
    "messages_per_day": 20000,
    "recipients_per_message": 50
  }'
```

## Error Responses

When rate limit is exceeded, API returns:

```json
{
  "error": "rate limit exceeded",
  "denied_by": "sender",
  "denied_key": "user@example.com",
  "retry_after": 1800
}
```

- `denied_by`: Which level triggered the rejection
- `denied_key`: The specific key that was limited
- `retry_after`: Seconds until the limit resets

## Monitoring

### Prometheus Metrics

```promql
# Rate limit rejections by level
rate(sendry_ratelimit_denied_total[5m])

# Current usage percentage
sendry_ratelimit_usage_ratio{level="global"}
```

### Logs

Rate limit events are logged:

```json
{"level":"warn","component":"ratelimit","msg":"rate limit exceeded","denied_by":"sender","key":"user@example.com","retry_after":"30m"}
```

## Example Configurations

### High-Volume Transactional

```yaml
rate_limit:
  enabled: true
  global:
    messages_per_hour: 100000
    messages_per_day: 1000000
  default_domain:
    messages_per_hour: 10000
    messages_per_day: 100000
  default_sender:
    messages_per_hour: 1000
    messages_per_day: 10000
```

### Newsletter/Marketing

```yaml
rate_limit:
  enabled: true
  global:
    messages_per_hour: 50000
  default_domain:
    messages_per_hour: 5000
  # Relaxed sender limits for bulk sending
  default_sender:
    messages_per_hour: 5000
    messages_per_day: 50000
```

### Shared Hosting

```yaml
rate_limit:
  enabled: true
  global:
    messages_per_hour: 10000
  # Strict per-domain limits
  default_domain:
    messages_per_hour: 100
    messages_per_day: 500
  # Very strict per-sender limits
  default_sender:
    messages_per_hour: 20
    messages_per_day: 100
```

### Development (Disabled)

```yaml
rate_limit:
  enabled: false
```
