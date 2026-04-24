# Delivery and Engagement Analytics

This document explains why the current Sendry telemetry is not suitable as
analytics, and proposes an architecture for a dedicated analytics service.

## What Exists Today

- **Prometheus metrics** (`internal/metrics/`): counters for `sent`/`failed`/`bounced`/`deferred` per domain, SMTP connections, API requests, queue. In-memory plus a Bolt snapshot.
- **Per-message status** in `internal/queue/storage.go`: `pending`/`sending`/`delivered`/`failed`/`deferred` in Bolt.
- **`sendry-web`**: `send_jobs`, `send_job_items`, `sends` tables in SQLite for per-campaign aggregates.

## Fundamental Gaps

1. **Event log** — no stream of events (`accepted`, `attempted`, `deferred`, `delivered`, `bounced`, `opened`, `clicked`, `unsubscribed`) with timestamp and context. Only the final status is stored.
2. **Per-message attempt timeline** — attempts, MX, SMTP response. `queue.DeliveryAttempt` exists as a struct but is not persisted.
3. **Inbound bounces / NDRs** — no RFC 3464 parser; async bounces are invisible.
4. **Open/click tracking** — no pixels, no link rewriter.
5. **Unsubscribe / suppression list** — only a `recipient.status` field in web.
6. **Recipient engagement history** — no per-address event table.
7. **Reputation metrics** — no bounce-rate / complaint-rate aggregates per from-domain/IP.
8. **Cohort / campaign analytics** — `send_job_items` in SQLite will not scale to per-event rows.
9. **Long-term retention** — Prometheus retention is short, Bolt cleaner deletes `delivered`.
10. **Funnel** `sent → delivered → opened → clicked → converted` — technically impossible without an event log.

Conclusion: the current stack answers "what is happening now" (monitoring), not "what happened to message X two weeks ago" or "what is the CTR of campaign Y" (analytics).

## Three Architectural Options

### Option A. External services

Plug in an existing storage + visualization:

- **A1. ClickHouse** — de-facto standard for email/ad analytics. Table `events(ts, msg_id, campaign_id, recipient, type, domain, mx, smtp_code, ip, ua, ...)`. Columnar, millions rps, fast aggregates, cheap retention. Grafana on top.
- **A2. PostgreSQL + TimescaleDB** — simpler if Postgres is already in use. Fine up to millions of writes per day, painful beyond.
- **A3. Loki + Grafana** — event search, not analytics.
- **A4. SaaS** (Segment / Mixpanel / PostHog) — not email-specific.

Fits > 1M messages/month and willingness to operate external storage.

### Option B. In-house `sendry-analytics`

A separate binary in the same repo.

Responsibilities:

1. Ingest events from `sendry` (HTTP webhook or broker).
2. Event storage (SQLite for MVP, ClickHouse for scale).
3. Aggregates (materialized views: per-campaign / per-domain / per-day).
4. API for `sendry-web` (dashboards, timelines).
5. Tracking endpoints:
   - `/t/o/{msg_id}` — open pixel,
   - `/t/c/{msg_id}/{link_id}` — click redirect,
   - `/t/u/{token}` — unsubscribe.
6. Inbound bounce handler (VERP) — SMTP receiver on the bounce domain, DSN parser.
7. Outbound webhooks (for clients).

Upsides: full control, tied to Sendry's domain model, unified UI. Downsides: one more service and DB; risk of reinventing what ClickHouse gives for free.

### Option C. Hybrid (recommended)

`sendry-analytics` as our own service, with **ClickHouse as the storage engine underneath**.

- `sendry-analytics` is thin: ingestion, tracking, bounce handler, UI API.
- Storage is ClickHouse (or SQLite in MVP with an explicit migration path).
- Dashboards inside `sendry-web` or in Grafana over ClickHouse.

Keeps domain-model control while getting scalable storage for free.

## Event Ingestion Patterns

Both are needed:

### Push: `sendry` → `sendry-analytics`

- `sendry` publishes events as it works.
- Transport:
  - HTTP webhook (KISS, retry + DLQ required);
  - NATS JetStream / Redis Streams (reliable, ack/retry built-in) at > 1k events/s;
  - local append-only log + tail (fluent-bit → ClickHouse) as a cheap fallback.

### Pull: tracking endpoints → `sendry-analytics`

- Open pixel `GET /t/o/{msg_id}.gif` → event `opened`.
- Click redirect `GET /t/c/{msg_id}/{link_id}` → event `clicked` + 302 to the original URL.
- Unsubscribe `GET /t/u/{token}` → event `unsubscribed`.
- Endpoints are public, CDN-friendly, write directly to storage.

## Event Schema

```
event {
  id            uuid        // idempotency
  occurred_at   timestamp
  received_at   timestamp
  type          enum(
                  accepted, queued, attempted, deferred, delivered,
                  soft_bounce, hard_bounce, failed,
                  opened, clicked, unsubscribed, complained,
                  webhook_delivered, webhook_failed
                )
  message_id    string      // sendry msg id, end-to-end
  campaign_id   string      // optional
  variant_id    string      // A/B, optional
  recipient     string
  from_domain   string
  to_domain     string
  mx_host       string
  smtp_code     int
  smtp_response text
  user_agent    string
  ip            string
  link_url      string
  meta          json
}
```

## Invariants to Lock In

1. **End-to-end `message_id`** — from web → sendry → delivery → bounce → open/click. The id exists in sendry today but is not propagated down to the web job item in every path.
2. **VERP** (`bounce+{msg_id}@bounce.example.com`) — to tie async bounces back.
3. **HMAC-signed tokens** on tracking links — prevent external rewrite.
4. **Idempotency** — webhook retries may deliver an event twice; dedup on `event.id`.
5. **Privacy / GDPR** — open pixel records IP; need opt-out per domain/campaign.
6. **Separation of concerns** — tracking endpoints on a separate domain from the MTA; a blocklisted pixel host must not take down the API.

## Implementation Steps

### Step 1. Event log inside `sendry` (MVP, no new service)

- Bolt bucket `events`, append-only, indexed by `msg_id`.
- API `GET /api/v1/messages/{id}/events`.
- Publish from `processor.go` and `smtp/client.go` at key points.
- Bounded by Bolt size but unblocks per-message debugging.

### Step 2. Extract `sendry-analytics`

- `cmd/sendry-analytics/` + `internal/analytics/`.
- Webhook ingestion on `/events/ingest`.
- MVP storage: SQLite with rollup tables (`events_raw`, `events_daily_by_campaign`, `events_daily_by_domain`).
- API for `sendry-web`:
  - `GET /campaigns/{id}/stats`,
  - `GET /domains/{d}/reputation`,
  - `GET /messages/{id}/timeline`.

### Step 3. Open/click tracking

- Endpoints `/t/o`, `/t/c`, `/t/u` in `sendry-analytics`.
- Link rewriter in the `sendry-web` template renderer or the `sendry` processor.
- Campaign flags: `track_opens`, `track_clicks`.
- HMAC token protection.

### Step 4. VERP + inbound bounce handler

- Config `bounce.domain`, `bounce.listen_addr` in `sendry-analytics`.
- SMTP receiver, RFC 3464 parser, hard/soft classification.
- Automatic suppression-list update in `sendry-web`.
- Event `bounce_received` in the log.

### Step 5. Replace SQLite with ClickHouse

- When events > 10M/month.
- Event schema stays; storage driver changes.
- ClickHouse as an optional dependency in `docker-compose.override.yml`.

### Step 6. Outbound webhooks

- Clients subscribe to events from their campaigns.
- HMAC, retry/backoff, DLQ.
- Subscription management in the `sendry-web` UI.

## Recommendation

Start with **steps 1 and 2** following option B/C:

- Step 1 gives per-message visibility without adding a new service (small, isolated change).
- Step 2 extracts `sendry-analytics` with SQLite on day one; the correct event schema is set from the start, storage is swapped for ClickHouse later.

Option A (plain ClickHouse + Grafana) is rejected: open/click/unsubscribe/bounce must be tied to the campaign domain model and require dynamic tracking-link generation — that is our own application.

## Open Questions

Needed to finalize the plan:

1. Target events/day (100k messages ≈ 300k–1M events after attempts / opens / clicks).
2. Are open/click needed now, or only delivery analytics?
3. Existing infrastructure (Prometheus/Grafana/Postgres/ClickHouse).
4. Acceptability of external dependencies (ClickHouse, NATS, Redis) vs a strict "only SQLite/Bolt" constraint.
5. Do we need outbound webhooks for clients (multi-tenant), or is `sendry-web` the sole consumer?
6. Required retention: 30 / 90 / 365 days / forever.
7. Can a dedicated subdomain + MX record be provisioned for VERP?
