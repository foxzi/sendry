# Хранение сообщений и DLQ

Sendry предоставляет настраиваемые политики хранения для управления дисковым пространством и жизненным циклом сообщений.

## Очередь недоставленных сообщений (DLQ)

Когда сообщение не удаётся доставить после всех попыток, оно перемещается в DLQ (Dead Letter Queue) вместо удаления. Это позволяет:

- **Ручной просмотр** неудачных сообщений
- **Повторную отправку** через API: `POST /api/v1/dlq/{id}/retry`
- **Анализ** причин недоставки

### Конфигурация

```yaml
dlq:
  enabled: true              # Включить DLQ (по умолчанию: true)
  max_age: 30d               # Удалять сообщения старше (0 = хранить вечно)
  max_count: 10000           # Макс. сообщений в DLQ (0 = без лимита)
  cleanup_interval: 1h       # Интервал очистки (по умолчанию: 1ч)
```

### Поведение

| Настройка | Эффект |
|-----------|--------|
| `enabled: true` | Неудачные сообщения перемещаются в DLQ |
| `enabled: false` | Неудачные сообщения удаляются сразу |
| `max_age: 30d` | Сообщения старше 30 дней удаляются автоматически |
| `max_count: 10000` | При превышении лимита удаляются самые старые (FIFO) |

### API эндпоинты

| Эндпоинт | Описание |
|----------|----------|
| `GET /api/v1/dlq` | Список DLQ со статистикой |
| `GET /api/v1/dlq/{id}` | Детали сообщения |
| `POST /api/v1/dlq/{id}/retry` | Вернуть в очередь на отправку |
| `DELETE /api/v1/dlq/{id}` | Удалить навсегда |

## Хранение доставленных сообщений

По умолчанию доставленные сообщения хранятся вечно. Настройте retention для автоматической очистки.

### Конфигурация

```yaml
storage:
  path: "/var/lib/sendry/queue.db"
  retention:
    delivered_max_age: 7d    # Удалять доставленные старше (0 = хранить вечно)
    cleanup_interval: 1h     # Интервал очистки (по умолчанию: 1ч)
```

### Примеры конфигураций

#### Production (хранить историю)

```yaml
storage:
  retention:
    delivered_max_age: 30d   # Хранить 30 дней истории
    cleanup_interval: 6h

dlq:
  enabled: true
  max_age: 90d               # Хранить неудачные 90 дней
  max_count: 50000
  cleanup_interval: 6h
```

#### Высокая нагрузка (агрессивная очистка)

```yaml
storage:
  retention:
    delivered_max_age: 24h   # Хранить только 24 часа
    cleanup_interval: 1h

dlq:
  enabled: true
  max_age: 7d
  max_count: 1000
  cleanup_interval: 1h
```

#### Разработка (без DLQ)

```yaml
storage:
  retention:
    delivered_max_age: 1h
    cleanup_interval: 10m

dlq:
  enabled: false             # Удалять неудачные сразу
```

## Жизненный цикл сообщения

```
Сообщение получено
       │
       ▼
   [pending]
       │
       ▼
   [sending] ──успех──► [delivered] ──(max_age)──► удалено
       │
       ▼
   [deferred] (retry с backoff)
       │
       ▼ (превышены попытки)
       │
   dlq.enabled?
       │
  ┌────┴────┐
  │         │
 да        нет
  │         │
  ▼         ▼
[DLQ]    удалено
  │
  ├──(max_age)──► удалено
  ├──(max_count)──► старые удалены
  └──(retry API)──► [pending]
```

## Мониторинг

Отслеживайте очистку в логах:

```json
{"level":"info","component":"cleaner","msg":"cleaned up delivered messages","deleted":150}
{"level":"info","component":"cleaner","msg":"cleaned up DLQ messages","deleted":25}
```

Prometheus метрики для мониторинга очереди:

```promql
# Текущий размер DLQ
sendry_queue_size{status="failed"}

# Сообщения в DLQ за время
rate(sendry_messages_failed_total[1h])
```
