# Аналитика доставки и engagement

Документ описывает, почему текущая телеметрия Sendry непригодна как
аналитика, и предлагает архитектуру отдельного сервиса аналитики.

## Что уже есть

- **Prometheus метрики** (`internal/metrics/`): счётчики `sent`/`failed`/`bounced`/`deferred` по доменам, SMTP-соединения, API-запросы, очередь. Память + snapshot в Bolt.
- **Per-message статусы** в `internal/queue/storage.go`: `pending`/`sending`/`delivered`/`failed`/`deferred` в Bolt.
- **`sendry-web`**: таблицы `send_jobs`, `send_job_items`, `sends` в SQLite — агрегаты по кампаниям.

## Чего фундаментально нет

1. **Event log** — поток событий (`accepted`, `attempted`, `deferred`, `delivered`, `bounced`, `opened`, `clicked`, `unsubscribed`) с timestamp и контекстом. Сохраняется только финальный статус.
2. **Per-message attempt timeline** — попытки, MX, SMTP-ответ. `queue.DeliveryAttempt` есть как struct, но не персистится.
3. **Входящие bounce/NDR** — парсер RFC 3464 не запущен, async-bounce невидим.
4. **Open/click tracking** — нет пикселей и link rewriter'а.
5. **Unsubscribe / suppression list** — только на уровне `recipient.status` в web.
6. **Recipient engagement history** — нет таблицы событий по адресу.
7. **Reputation metrics** — нет агрегатов bounce rate / complaint rate по from-домену / IP.
8. **Cohort / campaign analytics** — `send_job_items` в SQLite умрёт на росте per-event записей.
9. **Долгосрочное хранение** — Prometheus = короткий retention, Bolt cleaner удаляет `delivered`.
10. **Воронка** `sent → delivered → opened → clicked → converted` — технически невозможна без event log.

Вывод: текущий стек решает «что сейчас происходит» (monitoring), но не «что случилось с письмом X две недели назад» и «какой CTR у кампании Y» (analytics).

## Три варианта архитектуры

### Вариант A. Внешние сервисы

Подключить готовое хранилище и визуализацию:

- **A1. ClickHouse** — де-факто стандарт для email/ad аналитики. Таблица `events(ts, msg_id, campaign_id, recipient, type, domain, mx, smtp_code, ip, ua, ...)`. Колоночное хранение, миллионы rps, быстрые агрегаты, дешёвый retention. Grafana сверху.
- **A2. PostgreSQL + TimescaleDB** — проще, если Postgres уже есть. На миллионы writes/сутки ок, выше — тяжело.
- **A3. Loki + Grafana** — поиск по событиям, не аналитика.
- **A4. SaaS** (Segment / Mixpanel / PostHog) — не про email specifically.

Подходит при > 1M писем/месяц и готовности обслуживать внешнее хранилище.

### Вариант B. Свой `sendry-analytics`

Отдельный бинарь в этом же репо.

Ответственность:

1. Приём событий от `sendry` (HTTP webhook или брокер).
2. Хранилище событий (SQLite для MVP, ClickHouse для масштаба).
3. Агрегаты (материализованные view: per-campaign / per-domain / per-day).
4. API для `sendry-web` (dashboards, timelines).
5. Трекинг-endpoints:
   - `/t/o/{msg_id}` — open pixel,
   - `/t/c/{msg_id}/{link_id}` — click redirect,
   - `/t/u/{token}` — unsubscribe.
6. Inbound bounce handler (VERP) — SMTP-приёмник на bounce-домен, парсер DSN.
7. Webhooks наружу (для клиентов).

Плюсы: полный контроль, привязка к доменной модели Sendry, единый UI. Минусы: ещё один сервис и БД; риск переизобрести то, что в ClickHouse из коробки.

### Вариант C. Гибрид (рекомендация)

`sendry-analytics` как свой сервис, но **ClickHouse под капотом** как storage.

- `sendry-analytics` — тонкий слой: ingestion, трекинг, bounce handler, API для UI.
- Storage — ClickHouse (или SQLite в MVP с явным путём миграции).
- Dashboards — внутренние в `sendry-web` или Grafana поверх ClickHouse.

Даёт и контроль доменной модели, и масштабируемость storage.

## Паттерны ингестирования событий

Оба нужны:

### Push: `sendry` → `sendry-analytics`

- `sendry` публикует события по мере работы.
- Транспорт:
  - HTTP webhook (KISS, retry + DLQ обязательны);
  - NATS JetStream / Redis Streams (надёжно, ack/retry встроены) при > 1k событий/с;
  - локальный append-only log + tail (fluent-bit → ClickHouse) как дешёвый вариант.

### Pull: трекинг-endpoints → `sendry-analytics`

- Open pixel `GET /t/o/{msg_id}.gif` → event `opened`.
- Click redirect `GET /t/c/{msg_id}/{link_id}` → event `clicked` + 302 на оригинальный URL.
- Unsubscribe `GET /t/u/{token}` → event `unsubscribed`.
- Endpoints публичные, CDN-friendly, пишут напрямую в storage.

## Схема события

```
event {
  id            uuid        // для idempotency
  occurred_at   timestamp   // когда произошло
  received_at   timestamp   // когда принято
  type          enum(
                  accepted, queued, attempted, deferred, delivered,
                  soft_bounce, hard_bounce, failed,
                  opened, clicked, unsubscribed, complained,
                  webhook_delivered, webhook_failed
                )
  message_id    string      // sendry msg id, сквозной
  campaign_id   string      // optional
  variant_id    string      // A/B, optional
  recipient     string
  from_domain   string
  to_domain     string
  mx_host       string      // для delivery
  smtp_code     int
  smtp_response text
  user_agent    string      // для opened/clicked
  ip            string
  link_url      string      // для clicked
  meta          json
}
```

## Инварианты, которые критично зафиксировать

1. **Сквозной `message_id`** — от web → sendry → delivery → bounce → open/click. Сейчас id есть в sendry, но не протянут в web до item'а во всех сценариях.
2. **VERP** (`bounce+{msg_id}@bounce.example.com`) — для привязки async-bounce'ов.
3. **HMAC-подписанные токены** в трекинг-линках — запрет переписывания внешними.
4. **Idempotency** — webhook retry может доставить событие дважды, дедупликация по `event.id`.
5. **Privacy / GDPR** — open pixel трекает IP, нужен opt-out флаг на уровне домена/кампании.
6. **Separation of concerns** — трекинг-endpoints на отдельном домене от MTA, blacklist пикселя не должен ронять API.

## Пошаговый план внедрения

### Шаг 1. Event log в `sendry` (MVP, без нового сервиса)

- Bolt bucket `events`, append-only, индекс по `msg_id`.
- API `GET /api/v1/messages/{id}/events`.
- Публикация из `processor.go` и `smtp/client.go` на ключевых точках.
- Ограничен объёмом Bolt, но даёт timeline и разблокирует отладку.

### Шаг 2. Выделить `sendry-analytics`

- `cmd/sendry-analytics/` + `internal/analytics/`.
- Приём webhook от `sendry` на `/events/ingest`.
- MVP storage: SQLite с rollup-таблицами (`events_raw`, `events_daily_by_campaign`, `events_daily_by_domain`).
- API для `sendry-web`:
  - `GET /campaigns/{id}/stats`,
  - `GET /domains/{d}/reputation`,
  - `GET /messages/{id}/timeline`.

### Шаг 3. Open/click tracking

- Endpoints `/t/o`, `/t/c`, `/t/u` в `sendry-analytics`.
- Link rewriter в шаблонизаторе `sendry-web` или в `sendry` processor'е.
- Флаги кампании: `track_opens`, `track_clicks`.
- HMAC-токены для защиты.

### Шаг 4. VERP + inbound bounce handler

- Конфиг `bounce.domain`, `bounce.listen_addr` в `sendry-analytics`.
- SMTP-приёмник, парсер RFC 3464, классификация hard/soft.
- Автоматическое обновление suppression-list в `sendry-web`.
- Эвент `bounce_received` в log.

### Шаг 5. Замена SQLite → ClickHouse

- Когда события > 10M/мес.
- Схема events остаётся, меняется storage-драйвер.
- ClickHouse как опциональная зависимость в `docker-compose.override.yml`.

### Шаг 6. Webhooks наружу

- Клиенты подписываются на события своих кампаний.
- HMAC, retry/backoff, DLQ.
- Управление подписками в UI `sendry-web`.

## Рекомендация

Начать с **шагов 1 и 2** по варианту B/C:

- Шаг 1 даёт видимость по каждому письму без нового сервиса (небольшой изолированный change).
- Шаг 2 выделяет `sendry-analytics` с SQLite на старте; со дня первого кладётся правильная схема events, storage потом легко меняется на ClickHouse.

Вариант A (просто ClickHouse + Grafana) не подходит, потому что open/click/unsubscribe/bounce требуют привязки к доменной модели кампаний и динамического генерирования трекинг-линков — это нужно своё приложение.

## Открытые вопросы

Требуют ответа перед окончательным планом:

1. Целевой объём событий/сутки (100k писем ≈ 300k–1M событий с учётом attempts / opens / clicks).
2. Нужны ли сейчас open/click или только delivery analytics.
3. Существующая инфраструктура (Prometheus/Grafana/Postgres/ClickHouse).
4. Допустимость внешних зависимостей (ClickHouse, NATS, Redis) или жёсткое «только SQLite/Bolt».
5. Нужны ли webhook'и клиентам (multi-tenant) или `sendry-web` — единственный потребитель.
6. Требуемый retention: 30 / 90 / 365 дней / навсегда.
7. Возможность выделить поддомен + MX-запись под VERP.
