# Scaling: Large Mailings

This document describes bottlenecks in the current architecture for mailings of ~100,000 messages and a plan to address them.

## Bottlenecks in the Current Code

### 1. `sendry-web/worker` — limited parallelism

File: `internal/web/worker/worker.go`.

- Single worker instance per process.
- Defaults: `batchSize = 10`, `pollInterval = 5s`, `concurrency = 5`.
- `processJob` fetches `GetPendingItems(batchSize)` and sends each item as a separate HTTP POST to `sendry`.
- Concurrency inside a batch is capped at `concurrency = 5`.

For 100k messages at ~50ms latency this is tens of minutes; at ~500ms — hours.

### 2. `trackQueuedItems` — N+1 pull

Every iteration polls `GetStatus` for all queued items. With 10k in flight this is 10k HTTP calls every 5 seconds.

### 3. SQLite writer lock

`send_job_items` with 100k rows per job; each `UpdateItemStatus` is a separate UPDATE. Under high worker + UI + background concurrency this produces contention.

### 4. `sendry` API — per-message HTTP

`POST /api/v1/send` accepts only a single message. No batch endpoint.

### 5. BoltDB — single writer

`internal/queue/BoltStorage` uses BoltDB, which allows only one writer per database, capping queue throughput.

### 6. Processor — no recipient-domain grouping

`internal/queue/processor.go` processes one message at a time. 10k messages to one domain become 10k separate SMTP sessions instead of reusing a connection with multiple `RCPT TO`.

### 7. Logging

`slog.Info("message delivered", ...)` per message means 100k JSON log lines per mailing.

## Plan, from Simple to Complex

### Stage 1. Remove bottlenecks without adding a new service

Expected gain: x10–x50 throughput.

1. **Batch send API in `sendry`**
   - `POST /api/v1/send/batch` accepts an array of messages.
   - Validate and enqueue in a single Bolt transaction.
2. **Batch enqueue in `sendry-web` worker**
   - Worker sends batches of 500–1000.
3. **Webhook delivery events instead of pull**
   - `sendry` pushes events (`delivered`, `failed`, `bounced`, `deferred`) to `sendry-web`.
   - HMAC-signed.
   - Removes the `trackQueuedItems` N+1 problem.
4. **Batch item-status updates in SQLite**
   - `UpdateItemStatusBatch` instead of one-by-one UPDATEs.
   - Enable WAL, `PRAGMA synchronous = NORMAL`.
5. **SMTP connection reuse by recipient domain**
   - In the processor, group messages with the same MX into one SMTP session.
6. **Worker pool in `sendry-web`**
   - N workers with non-blocking item selection (`UPDATE ... WHERE id IN (SELECT ... LIMIT N)` inside a transaction).

### Stage 2. Extract `sendry-dispatcher`

Needed when the web worker cannot keep up even with batching.

Responsibilities:
- read jobs and items from the web DB;
- render templates and substitute variables;
- send batches to `sendry`;
- write statuses back.

Properties:
- stateless, horizontally scalable (N replicas);
- separate binary `cmd/sendry-dispatcher`, reuses `internal/web/worker` and `internal/web/sendry`;
- `sendry-web` stays a thin UI + API;
- deployed close to `sendry` (LAN).

State stays in the `sendry-web` DB or moves to a broker (see stage 3).

### Stage 3. Job broker

Justified when there is more than one dispatcher replica and/or more than 1M messages per day.

Options:
- NATS JetStream;
- Redis Streams.

Layout:
- `sendry-web` enqueues `send_item` into a stream;
- dispatchers consume concurrently with ack/nack;
- statuses flow back through the same broker;
- retry/DLQ/observability included.

For 100k/day a broker is overkill. Makes sense at 10M+/day or under strict SLAs.

### Stage 4. Scaling `sendry` itself

- Multiple `sendry` instances behind `sendry-web` routing (already supported via `sendry.Servers`).
- For very large volumes replace the BoltDB queue with SQLite/PostgreSQL or shard Bolt.
- Dedicated outbound pools per IP for warming and reputation isolation.

## Target Volumes and Stage Selection

| Mailing volume | Recommended stage |
|----------------|-------------------|
| 100k / day | Stage 1, items 1–4 |
| 100k / hour | Full stage 1 |
| 100k / 10 min | Stage 1 + stage 2 |
| 1M / hour | Stage 1 + 2 + 3 |
| 10M+ / day | All stages, including 4 |

## What to Implement First

1. Batch send API (`POST /api/v1/send/batch`) in `sendry`.
2. Batch enqueue in the `sendry-web` worker.
3. Webhook delivery events (overlaps with the delivery-tracking task).
4. Batch item-status updates + WAL in SQLite.

Then measure on real load and decide whether to keep work inside `sendry-web` worker or extract `sendry-dispatcher`.

## Open Questions

Needed to finalize the plan:

1. Target mailing volume and time window (100k in what time?).
2. Is 100k a single campaign or the sum of parallel campaigns?
3. Acceptable latency from `send` to the recipient MX.
4. Planned number of `sendry` instances.
5. Whether external dependencies (Redis/NATS) are acceptable.
