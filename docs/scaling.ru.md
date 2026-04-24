# Масштабирование: большие рассылки

Документ описывает узкие места текущей архитектуры при рассылках порядка 100 000 писем и план их устранения.

## Узкие места в текущем коде

### 1. `sendry-web/worker` — ограниченная параллельность

Файл: `internal/web/worker/worker.go`.

- Один экземпляр воркера на процесс.
- `batchSize = 10`, `pollInterval = 5s`, `concurrency = 5` по умолчанию.
- `processJob` берёт `GetPendingItems(batchSize)` и отправляет каждый item отдельным HTTP POST в `sendry`.
- Внутри batch'а параллельность ограничена `concurrency = 5`.

При 100k писем и latency ~50ms на вызов это десятки минут, при ~500ms — часы.

### 2. `trackQueuedItems` — N+1 pull

`Worker.trackQueuedItems` каждую итерацию опрашивает `GetStatus` по всем queued-элементам. При очереди 10k — 10k HTTP-запросов каждые 5 секунд.

### 3. SQLite writer lock

`send_job_items` на 100k строк; каждый `UpdateItemStatus` — отдельный UPDATE. При высокой параллельности воркера + UI + фоновых задач возникает contention.

### 4. `sendry` API — per-message HTTP

`POST /api/v1/send` принимает только одно сообщение. Нет batch-endpoint'а.

### 5. BoltDB — single writer

`internal/queue/BoltStorage` основан на BoltDB. У Bolt один writer на всю базу, что ограничивает throughput очереди.

### 6. Processor — нет группировки по recipient-домену

`internal/queue/processor.go` обрабатывает сообщения по одному. Для 10k писем на один домен будет 10k отдельных SMTP-сессий вместо повторного использования соединения с несколькими `RCPT TO`.

### 7. Логирование

`slog.Info("message delivered", ...)` на каждое письмо → 100k строк JSON-лога на рассылку.

## План устранения, от простого к сложному

### Этап 1. Устранить узкие места без нового сервиса

Ожидаемый эффект: x10–x50 throughput.

1. **Batch send API в `sendry`**
   - `POST /api/v1/send/batch` принимает массив сообщений.
   - Валидация и enqueue в одной транзакции Bolt.
2. **Batch enqueue в `sendry-web` worker**
   - Воркер шлёт пачками по 500–1000 писем.
3. **Webhook delivery events вместо pull'а**
   - `sendry` шлёт события (`delivered`, `failed`, `bounced`, `deferred`) в `sendry-web` по HTTP.
   - HMAC-подпись.
   - Убирает `trackQueuedItems` N+1.
4. **Batch update item status в SQLite**
   - `UpdateItemStatusBatch` вместо одиночных UPDATE.
   - Включить WAL, `PRAGMA synchronous = NORMAL`.
5. **SMTP connection reuse по recipient-домену**
   - В processor'е группировать сообщения с одинаковым MX в одну SMTP-сессию.
6. **Пул воркеров в `sendry-web`**
   - N воркеров с неблокирующей выборкой items (`UPDATE ... WHERE id IN (SELECT ... LIMIT N)` в транзакции).

### Этап 2. Выделить `sendry-dispatcher`

Необходим, когда воркер веба не справляется даже с batch'ами.

Ответственность:
- читает задания и items из web DB;
- рендерит шаблоны с подстановкой переменных;
- шлёт batch'ами в `sendry`;
- пишет статусы обратно.

Характеристики:
- stateless, горизонтально масштабируется (N реплик);
- отдельный бинарь `cmd/sendry-dispatcher`, переиспользует код из `internal/web/worker`, `internal/web/sendry`;
- `sendry-web` остаётся тонким UI + API;
- разворачивается близко к `sendry` (LAN).

Стейт остаётся в `sendry-web` DB либо переезжает в брокер (см. этап 3).

### Этап 3. Брокер заданий

Целесообразен при > 1 dispatcher-реплики и/или > 1M писем/сутки.

Варианты:
- NATS JetStream;
- Redis Streams.

Схема:
- `sendry-web` enqueue'ит `send_item` в стрим;
- dispatcher'ы конкурентно consume'ят с ack/nack;
- статусы обратно через тот же брокер;
- retry/DLQ/наблюдаемость из коробки.

Для 100k писем/сутки брокер избыточен. Оправдан при 10M+/сутки или строгих SLA.

### Этап 4. Масштабирование самого `sendry`

- Несколько инстансов `sendry` за routing'ом `sendry-web` (уже поддерживается через `sendry.Servers`).
- Для реально больших объёмов — замена BoltDB очереди на SQLite/PostgreSQL или шардирование Bolt.
- Отдельные outbound-пулы на разные IP для warming'а и защиты репутации.

## Целевые цифры и выбор этапа

| Объём рассылки | Рекомендуемый этап |
|----------------|--------------------|
| 100k / сутки | Этап 1, пункты 1–4 |
| 100k / час | Этап 1 целиком |
| 100k / 10 мин | Этап 1 + этап 2 |
| 1M / час | Этап 1 + 2 + 3 |
| 10M+ / сутки | Все этапы, включая 4 |

## Что сделать первым

1. Batch send API (`POST /api/v1/send/batch`) в `sendry`.
2. Batch enqueue в воркере `sendry-web`.
3. Webhook delivery events (пересекается с задачей по отслеживанию доставки).
4. Batch update item status + WAL в SQLite.

Далее замер на реальной нагрузке и решение: остаёмся на `sendry-web` worker или выносим в `sendry-dispatcher`.

## Открытые вопросы

Требуют уточнения для окончательного плана:

1. Целевой объём рассылки и таргет по времени (100k за сколько?).
2. 100k — один campaign или суммарно параллельных?
3. Допустимая latency между `send` и попаданием в MX получателя.
4. Количество инстансов `sendry` в плане.
5. Допустимость внешних зависимостей (Redis/NATS).
