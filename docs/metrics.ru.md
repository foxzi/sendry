# Prometheus Метрики

Sendry MTA предоставляет встроенные метрики Prometheus для мониторинга и алертинга.

## Конфигурация

Добавьте в config.yaml:

```yaml
metrics:
  enabled: true
  listen_addr: ":9090"    # По умолчанию: :9090
  path: "/metrics"        # По умолчанию: /metrics
  flush_interval: 10s     # По умолчанию: 10s
  allowed_ips:            # IP адреса/CIDR с доступом к метрикам
    - "127.0.0.1"
    - "::1"
    - "10.0.0.0/8"
    - "192.168.0.0/16"
```

### Контроль доступа по IP

Поле `allowed_ips` ограничивает доступ к эндпоинту метрик. Поддерживает:
- Одиночные IP адреса: `192.168.1.100`
- CIDR нотацию: `10.0.0.0/8`, `192.168.0.0/16`
- IPv6: `::1`, `fe80::/10`

Если `allowed_ips` пуст или не указан, доступ разрешен всем.

Эндпоинт `/health` всегда доступен (полезно для балансировщиков).

Порядок определения IP клиента:
1. Заголовок `X-Forwarded-For` (первый IP)
2. Заголовок `X-Real-IP`
3. `RemoteAddr`

## Доступные метрики

### Счетчики сообщений

| Метрика | Labels | Описание |
|---------|--------|----------|
| `sendry_messages_sent_total` | domain | Успешно доставленные сообщения |
| `sendry_messages_failed_total` | domain, error_type | Не доставленные сообщения |
| `sendry_messages_bounced_total` | domain | Отправленные bounce-сообщения |
| `sendry_messages_deferred_total` | domain | Отложенные сообщения |

**Типы ошибок:**
- `connection_refused` - Сервер отклонил соединение
- `timeout` - Таймаут соединения или доставки
- `dns_error` - Ошибка DNS
- `recipient_rejected` - Получатель отклонен (550)
- `spam_rejected` - Сообщение отклонено как спам (554)
- `relay_denied` - Relay запрещен
- `auth_failed` - Ошибка аутентификации
- `tls_error` - Ошибка TLS/сертификата
- `other` - Другие ошибки

### Gauge очереди

| Метрика | Описание |
|---------|----------|
| `sendry_queue_size` | Ожидающие + отложенные сообщения |
| `sendry_queue_oldest_seconds` | Возраст самого старого сообщения |
| `sendry_queue_active` | Сейчас обрабатываются |
| `sendry_queue_deferred` | Ожидают повторной отправки |

### SMTP метрики

| Метрика | Labels | Тип | Описание |
|---------|--------|-----|----------|
| `sendry_smtp_connections_total` | server_type | counter | Всего SMTP соединений |
| `sendry_smtp_connections_active` | - | gauge | Активные соединения |
| `sendry_smtp_auth_success_total` | - | counter | Успешные аутентификации |
| `sendry_smtp_auth_failed_total` | - | counter | Неудачные аутентификации |
| `sendry_smtp_tls_connections_total` | - | counter | TLS соединения |

**Типы серверов:**
- `smtp` - Порт 25
- `submission` - Порт 587
- `smtps` - Порт 465

### API метрики

| Метрика | Labels | Тип | Описание |
|---------|--------|-----|----------|
| `sendry_api_requests_total` | method, path, status | counter | Всего API запросов |
| `sendry_api_request_duration_seconds` | method, path | histogram | Время запроса |
| `sendry_api_errors_total` | error_type | counter | Ошибки API |

**Типы ошибок:**
- `server_error` - Ошибки 5xx
- `rate_limited` - Ошибки 429
- `auth_error` - Ошибки 401/403
- `not_found` - Ошибки 404
- `bad_request` - Ошибки 400
- `client_error` - Другие 4xx

### Rate Limiting

| Метрика | Labels | Тип | Описание |
|---------|--------|-----|----------|
| `sendry_ratelimit_exceeded_total` | level | counter | Превышения лимитов |

**Уровни:**
- `global` - Серверный лимит
- `domain` - Лимит домена
- `sender` - Лимит отправителя
- `ip` - Лимит IP
- `api_key` - Лимит API ключа

### Системные метрики

| Метрика | Описание |
|---------|----------|
| `sendry_uptime_seconds` | Время работы сервера |
| `sendry_goroutines` | Активные горутины |
| `sendry_storage_used_bytes` | Размер BoltDB |

## Персистентность

Значения счетчиков сохраняются в BoltDB каждые `flush_interval` (по умолчанию 10 секунд) и восстанавливаются при запуске. Это гарантирует сохранение метрик при перезапуске.

## Конфигурация Prometheus

Добавьте в `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'sendry'
    static_configs:
      - targets: ['localhost:9090']
    scrape_interval: 15s
```

## Примеры запросов

### Скорость доставки (за час)
```promql
rate(sendry_messages_sent_total[1h])
```

### Процент ошибок
```promql
sum(rate(sendry_messages_failed_total[1h])) /
sum(rate(sendry_messages_sent_total[1h]) + rate(sendry_messages_failed_total[1h])) * 100
```

### Размер очереди
```promql
sendry_queue_size
```

### Активные SMTP соединения
```promql
sendry_smtp_connections_active
```

### Latency API (p99)
```promql
histogram_quantile(0.99, rate(sendry_api_request_duration_seconds_bucket[5m]))
```

### Rate limit по уровням
```promql
sum by (level) (rate(sendry_ratelimit_exceeded_total[1h]))
```

## Примеры алертов

### Большая очередь
```yaml
- alert: SendryHighQueueSize
  expr: sendry_queue_size > 1000
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Очередь Sendry слишком большая"
```

### Высокий процент ошибок
```yaml
- alert: SendryHighFailureRate
  expr: |
    sum(rate(sendry_messages_failed_total[5m])) /
    (sum(rate(sendry_messages_sent_total[5m])) + sum(rate(sendry_messages_failed_total[5m]))) > 0.1
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "Процент ошибок Sendry > 10%"
```

### Сервис недоступен
```yaml
- alert: SendryDown
  expr: up{job="sendry"} == 0
  for: 1m
  labels:
    severity: critical
  annotations:
    summary: "Sendry недоступен"
```
