# Message Retention and DLQ

Sendry provides configurable retention policies for managing disk space and message lifecycle.

## Dead Letter Queue (DLQ)

When a message fails delivery after all retry attempts, it's moved to the Dead Letter Queue (DLQ) instead of being deleted. This allows:

- **Manual review** of failed messages
- **Retry** via API: `POST /api/v1/dlq/{id}/retry`
- **Analysis** of delivery failures

### Configuration

```yaml
dlq:
  enabled: true              # Enable DLQ (default: true)
  max_age: 30d               # Delete messages older than this (0 = keep forever)
  max_count: 10000           # Max messages in DLQ (0 = unlimited)
  cleanup_interval: 1h       # How often to run cleanup (default: 1h)
```

### Behavior

| Setting | Effect |
|---------|--------|
| `enabled: true` | Failed messages moved to DLQ |
| `enabled: false` | Failed messages deleted immediately |
| `max_age: 30d` | Messages older than 30 days auto-deleted |
| `max_count: 10000` | Oldest messages deleted when limit exceeded (FIFO) |

### API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/dlq` | List DLQ messages with stats |
| `GET /api/v1/dlq/{id}` | Get message details |
| `POST /api/v1/dlq/{id}/retry` | Move back to pending queue |
| `DELETE /api/v1/dlq/{id}` | Delete permanently |

## Delivered Messages Retention

By default, delivered messages are kept forever. Configure retention to automatically clean up old messages.

### Configuration

```yaml
storage:
  path: "/var/lib/sendry/queue.db"
  retention:
    delivered_max_age: 7d    # Delete delivered messages older than this (0 = keep forever)
    cleanup_interval: 1h     # How often to run cleanup (default: 1h)
```

### Example Configurations

#### Production (keep history)

```yaml
storage:
  retention:
    delivered_max_age: 30d   # Keep 30 days of history
    cleanup_interval: 6h

dlq:
  enabled: true
  max_age: 90d               # Keep failed messages 90 days
  max_count: 50000
  cleanup_interval: 6h
```

#### High-volume (aggressive cleanup)

```yaml
storage:
  retention:
    delivered_max_age: 24h   # Keep only 24 hours
    cleanup_interval: 1h

dlq:
  enabled: true
  max_age: 7d
  max_count: 1000
  cleanup_interval: 1h
```

#### Development (no DLQ)

```yaml
storage:
  retention:
    delivered_max_age: 1h
    cleanup_interval: 10m

dlq:
  enabled: false             # Delete failed messages immediately
```

## Message Flow

```
Message received
       │
       ▼
   [pending]
       │
       ▼
   [sending] ──success──► [delivered] ──(max_age)──► deleted
       │
       ▼
   [deferred] (retry with backoff)
       │
       ▼ (max retries exceeded)
       │
   dlq.enabled?
       │
  ┌────┴────┐
  │         │
 yes        no
  │         │
  ▼         ▼
[DLQ]    deleted
  │
  ├──(max_age)──► deleted
  ├──(max_count)──► oldest deleted
  └──(retry API)──► [pending]
```

## Monitoring

Track cleanup activity in logs:

```json
{"level":"info","component":"cleaner","msg":"cleaned up delivered messages","deleted":150}
{"level":"info","component":"cleaner","msg":"cleaned up DLQ messages","deleted":25}
```

Prometheus metrics for queue monitoring:

```promql
# Current DLQ size
sendry_queue_size{status="failed"}

# Messages in DLQ over time
rate(sendry_messages_failed_total[1h])
```
