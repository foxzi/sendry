# Prometheus Metrics

Sendry MTA provides built-in Prometheus metrics for monitoring and alerting.

## Configuration

Add to your config.yaml:

```yaml
metrics:
  enabled: true
  listen_addr: ":9090"    # Default: :9090
  path: "/metrics"        # Default: /metrics
  flush_interval: 10s     # Default: 10s
  allowed_ips:            # IP addresses/CIDRs allowed to access metrics
    - "127.0.0.1"
    - "::1"
    - "10.0.0.0/8"
    - "192.168.0.0/16"
```

### IP Access Control

The `allowed_ips` field restricts access to the metrics endpoint. Supports:
- Single IP addresses: `192.168.1.100`
- CIDR notation: `10.0.0.0/8`, `192.168.0.0/16`
- IPv6: `::1`, `fe80::/10`

If `allowed_ips` is empty or not specified, all IPs are allowed.

The `/health` endpoint is always accessible (useful for load balancers).

IP detection order:
1. `X-Forwarded-For` header (first IP)
2. `X-Real-IP` header
3. `RemoteAddr`

## Available Metrics

### Message Counters

| Metric | Labels | Description |
|--------|--------|-------------|
| `sendry_messages_sent_total` | domain | Successfully delivered messages |
| `sendry_messages_failed_total` | domain, error_type | Permanently failed messages |
| `sendry_messages_bounced_total` | domain | Bounce messages sent |
| `sendry_messages_deferred_total` | domain | Messages deferred for retry |

**Error types:**
- `connection_refused` - Target server refused connection
- `timeout` - Connection or delivery timeout
- `dns_error` - DNS resolution failed
- `recipient_rejected` - Recipient address rejected (550)
- `spam_rejected` - Message rejected as spam (554)
- `relay_denied` - Relay not permitted
- `auth_failed` - Authentication failed
- `tls_error` - TLS/certificate error
- `other` - Other errors

### Queue Gauges

| Metric | Description |
|--------|-------------|
| `sendry_queue_size` | Pending + deferred messages |
| `sendry_queue_oldest_seconds` | Age of oldest message |
| `sendry_queue_active` | Currently processing |
| `sendry_queue_deferred` | Awaiting retry |

### SMTP Metrics

| Metric | Labels | Type | Description |
|--------|--------|------|-------------|
| `sendry_smtp_connections_total` | server_type | counter | Total SMTP connections |
| `sendry_smtp_connections_active` | - | gauge | Active connections |
| `sendry_smtp_auth_success_total` | - | counter | Successful authentications |
| `sendry_smtp_auth_failed_total` | - | counter | Failed authentications |
| `sendry_smtp_tls_connections_total` | - | counter | TLS connections |

**Server types:**
- `smtp` - Port 25
- `submission` - Port 587
- `smtps` - Port 465

### API Metrics

| Metric | Labels | Type | Description |
|--------|--------|------|-------------|
| `sendry_api_requests_total` | method, path, status | counter | Total API requests |
| `sendry_api_request_duration_seconds` | method, path | histogram | Request duration |
| `sendry_api_errors_total` | error_type | counter | API errors |

**Error types:**
- `server_error` - 5xx errors
- `rate_limited` - 429 errors
- `auth_error` - 401/403 errors
- `not_found` - 404 errors
- `bad_request` - 400 errors
- `client_error` - Other 4xx errors

### Rate Limiting

| Metric | Labels | Type | Description |
|--------|--------|------|-------------|
| `sendry_ratelimit_exceeded_total` | level | counter | Rate limit exceeded events |

**Levels:**
- `global` - Server-wide limit
- `domain` - Per-domain limit
- `sender` - Per-sender limit
- `ip` - Per-IP limit
- `api_key` - Per-API-key limit

### System Metrics

| Metric | Description |
|--------|-------------|
| `sendry_uptime_seconds` | Server uptime |
| `sendry_goroutines` | Active goroutines |
| `sendry_storage_used_bytes` | BoltDB file size |

## Persistence

Counter values are persisted to BoltDB every `flush_interval` (default 10s) and restored on startup. This ensures metrics survive restarts.

## Prometheus Configuration

Add to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'sendry'
    static_configs:
      - targets: ['localhost:9090']
    scrape_interval: 15s
```

## Example Queries

### Message delivery rate (last hour)
```promql
rate(sendry_messages_sent_total[1h])
```

### Failed message percentage
```promql
sum(rate(sendry_messages_failed_total[1h])) /
sum(rate(sendry_messages_sent_total[1h]) + rate(sendry_messages_failed_total[1h])) * 100
```

### Queue backlog
```promql
sendry_queue_size
```

### Active SMTP connections
```promql
sendry_smtp_connections_active
```

### API request latency (p99)
```promql
histogram_quantile(0.99, rate(sendry_api_request_duration_seconds_bucket[5m]))
```

### Rate limit events by level
```promql
sum by (level) (rate(sendry_ratelimit_exceeded_total[1h]))
```

## Alerting Examples

### High queue size
```yaml
- alert: SendryHighQueueSize
  expr: sendry_queue_size > 1000
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Sendry queue size is high"
```

### High failure rate
```yaml
- alert: SendryHighFailureRate
  expr: |
    sum(rate(sendry_messages_failed_total[5m])) /
    (sum(rate(sendry_messages_sent_total[5m])) + sum(rate(sendry_messages_failed_total[5m]))) > 0.1
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "Sendry message failure rate > 10%"
```

### Service down
```yaml
- alert: SendryDown
  expr: up{job="sendry"} == 0
  for: 1m
  labels:
    severity: critical
  annotations:
    summary: "Sendry is down"
```
