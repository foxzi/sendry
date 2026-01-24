# Sendry

MTA (Mail Transfer Agent) сервер для отправки электронной почты.

[English version](../README.md)

## Возможности

- SMTP сервер (порты 25, 587) с поддержкой AUTH
- SMTPS сервер (порт 465) с неявным TLS
- STARTTLS для безопасных соединений
- Let's Encrypt (ACME) автоматическое управление сертификатами
- DKIM подпись исходящих писем
- HTTP API для отправки писем
- Персистентная очередь на BoltDB
- Retry логика с exponential backoff
- Поддержка нескольких доменов с разными режимами:
  - `production` - обычная доставка
  - `sandbox` - перехват писем локально (для тестирования)
  - `redirect` - перенаправление всех писем на указанные адреса
  - `bcc` - обычная доставка + копия в архив
- Rate limiting (по домену, отправителю, IP, API ключу)
- Prometheus метрики с персистентностью
- Обработка bounce-сообщений
- Graceful shutdown
- Структурированное логирование (JSON)

## Требования

- Go 1.23+

## Быстрый старт

Подробное руководство: [quickstart.ru.md](quickstart.ru.md).

### Сборка

```bash
go build -o sendry ./cmd/sendry
```

### Конфигурация

Скопируйте и отредактируйте пример конфигурации:

```bash
cp configs/sendry.example.yaml config.yaml
```

Минимальная конфигурация:

```yaml
server:
  hostname: "mail.example.com"

smtp:
  domain: "example.com"
  auth:
    required: true
    users:
      myuser: "mypassword"

api:
  api_key: "your-secret-api-key"

storage:
  path: "./data/queue.db"
```

### Запуск

```bash
./sendry serve -c config.yaml
```

### Проверка конфигурации

```bash
./sendry config validate -c config.yaml
```

## API

### Health Check

```bash
curl http://localhost:8080/health
```

Ответ:
```json
{
  "status": "ok",
  "version": "0.2.0",
  "uptime": "1h30m",
  "queue": {
    "pending": 5,
    "sending": 1,
    "delivered": 100,
    "failed": 2,
    "deferred": 3,
    "total": 111
  }
}
```

### Отправка письма

```bash
curl -X POST http://localhost:8080/api/v1/send \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "from": "sender@example.com",
    "to": ["recipient@example.com"],
    "subject": "Тест",
    "body": "Привет, мир!",
    "html": "<p>Привет, <b>мир</b>!</p>"
  }'
```

Ответ:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending"
}
```

### Проверка статуса

```bash
curl http://localhost:8080/api/v1/status/{message_id} \
  -H "Authorization: Bearer YOUR_API_KEY"
```

Ответ:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "delivered",
  "from": "sender@example.com",
  "to": ["recipient@example.com"],
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:30:05Z",
  "retry_count": 0
}
```

### Статистика очереди

```bash
curl http://localhost:8080/api/v1/queue \
  -H "Authorization: Bearer YOUR_API_KEY"
```

### Удаление сообщения

```bash
curl -X DELETE http://localhost:8080/api/v1/queue/{message_id} \
  -H "Authorization: Bearer YOUR_API_KEY"
```

## Справочник по конфигурации

| Параметр | По умолчанию | Описание |
|----------|--------------|----------|
| `server.hostname` | hostname ОС | FQDN сервера |
| `smtp.listen_addr` | `:25` | Порт SMTP relay |
| `smtp.submission_addr` | `:587` | Порт SMTP submission |
| `smtp.smtps_addr` | `:465` | Порт SMTPS (неявный TLS) |
| `smtp.domain` | *обязательный* | Почтовый домен |
| `smtp.max_message_bytes` | `10485760` | Макс. размер сообщения (10MB) |
| `smtp.max_recipients` | `100` | Макс. получателей на сообщение |
| `smtp.auth.required` | `false` | Требовать аутентификацию |
| `smtp.auth.users` | `{}` | Словарь username -> password |
| `smtp.tls.cert_file` | `""` | Путь к TLS сертификату |
| `smtp.tls.key_file` | `""` | Путь к приватному ключу TLS |
| `smtp.tls.acme.enabled` | `false` | Включить Let's Encrypt |
| `smtp.tls.acme.email` | `""` | Email для ACME аккаунта |
| `smtp.tls.acme.domains` | `[]` | Домены для сертификата |
| `smtp.tls.acme.cache_dir` | `/var/lib/sendry/certs` | Кэш сертификатов |
| `dkim.enabled` | `false` | Включить DKIM подпись |
| `dkim.selector` | `""` | DKIM селектор |
| `dkim.domain` | `""` | DKIM домен |
| `dkim.key_file` | `""` | Путь к приватному ключу DKIM |
| `api.listen_addr` | `:8080` | Порт HTTP API |
| `api.api_key` | `""` | API ключ (пусто = без авторизации) |
| `queue.workers` | `4` | Количество воркеров доставки |
| `queue.retry_interval` | `5m` | Базовый интервал retry |
| `queue.max_retries` | `5` | Макс. попыток доставки |
| `storage.path` | `/var/lib/sendry/queue.db` | Путь к файлу BoltDB |
| `storage.retention.delivered_max_age` | `0` | Удалять доставленные сообщения старше |
| `storage.retention.cleanup_interval` | `1h` | Интервал очистки |
| `dlq.enabled` | `true` | Включить очередь недоставленных |
| `dlq.max_age` | `0` | Удалять DLQ сообщения старше |
| `dlq.max_count` | `0` | Макс. сообщений в DLQ (0 = без лимита) |
| `dlq.cleanup_interval` | `1h` | Интервал очистки DLQ |
| `logging.level` | `info` | Уровень логов (debug/info/warn/error) |
| `logging.format` | `json` | Формат логов (json/text) |
| `metrics.enabled` | `false` | Включить Prometheus метрики |
| `metrics.listen_addr` | `:9090` | Порт сервера метрик |
| `metrics.path` | `/metrics` | Путь эндпоинта метрик |
| `metrics.flush_interval` | `10s` | Интервал сохранения счетчиков |
| `metrics.allowed_ips` | `[]` | IP/CIDR с доступом к метрикам |

Подробные инструкции смотрите в [справочнике по HTTP API](api.ru.md), [документации по TLS и DKIM](tls-dkim.ru.md) и [Prometheus метрикам](metrics.ru.md).

## Структура проекта

```
sendry/
├── cmd/sendry/          # CLI точка входа
├── internal/
│   ├── api/             # HTTP API сервер
│   ├── app/             # Оркестрация приложения
│   ├── config/          # Конфигурация
│   ├── dkim/            # DKIM подпись
│   ├── dns/             # MX резолвер
│   ├── metrics/         # Prometheus метрики
│   ├── queue/           # Очередь сообщений и хранилище
│   ├── smtp/            # SMTP сервер и клиент
│   └── tls/             # TLS/ACME поддержка
├── configs/             # Примеры конфигураций
└── docs/                # Документация
```

## Статусы сообщений

| Статус | Описание |
|--------|----------|
| `pending` | Ожидает отправки |
| `sending` | В процессе отправки |
| `delivered` | Успешно доставлено |
| `deferred` | Отложено (временная ошибка, будет retry) |
| `failed` | Ошибка доставки (превышены retry или постоянная ошибка) |

## Лицензия

GPL-3.0
