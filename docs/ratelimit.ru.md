# Rate Limiting

Sendry предоставляет многоуровневое ограничение частоты запросов для защиты от злоупотреблений и справедливого распределения ресурсов.

## Обзор

Rate limiting работает на нескольких уровнях, каждый с независимыми счётчиками:

| Уровень | Описание | Пример ключа | Время проверки |
|---------|----------|--------------|----------------|
| `global` | Общий лимит сервера | `global` | При приёме |
| `domain` | По домену отправителя | `example.com` | При приёме |
| `sender` | По email отправителя | `user@example.com` | При приёме |
| `ip` | По IP клиента | `192.168.1.1` | При приёме |
| `api_key` | По API ключу | `key-abc123` | При приёме |
| `recipient_domain` | По домену получателя | `gmail.com` | При отправке |

При получении сообщения (через SMTP или API) проверяются первые 5 уровней. Если любой лимит превышен, сообщение отклоняется.

Лимиты доменов получателей проверяются **во время отправки** (в обработчике очереди). Если превышен, сообщение **откладывается** (не отклоняется) и будет повторно отправлено после сброса окна лимита.

## Конфигурация

```yaml
rate_limit:
  enabled: true

  # Глобальные лимиты сервера
  global:
    messages_per_hour: 50000
    messages_per_day: 500000

  # Лимиты по умолчанию для доменов
  default_domain:
    messages_per_hour: 1000
    messages_per_day: 10000

  # Лимиты по умолчанию для отправителей
  default_sender:
    messages_per_hour: 100
    messages_per_day: 1000

  # Лимиты по умолчанию для IP
  default_ip:
    messages_per_hour: 500
    messages_per_day: 5000

  # Лимиты по умолчанию для API ключей
  default_api_key:
    messages_per_hour: 1000
    messages_per_day: 10000

  # Лимиты по умолчанию для доменов получателей (gmail.com, mail.ru и т.д.)
  # Проверяется при отправке - если превышен, сообщение откладывается
  default_recipient_domain:
    messages_per_hour: 5000
    messages_per_day: 50000

  # Лимиты для конкретных доменов получателей (переопределяют default)
  recipient_domains:
    gmail.com:
      messages_per_hour: 1000
      messages_per_day: 10000
    mail.ru:
      messages_per_hour: 1000
      messages_per_day: 10000
    yandex.ru:
      messages_per_hour: 1000
      messages_per_day: 10000
```

### Переопределение для доменов

Переопределите лимиты для конкретных доменов:

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

## Как это работает

### Окна счётчиков

- **Часовой счётчик**: Сбрасывается каждый час с момента отправки первого сообщения
- **Дневной счётчик**: Сбрасывается каждые 24 часа с момента отправки первого сообщения

### Порядок проверки лимитов

При получении сообщения лимиты проверяются в таком порядке:

1. Глобальный лимит
2. Лимит домена
3. Лимит отправителя
4. Лимит IP
5. Лимит API ключа

Первый превышенный лимит вызывает отклонение. Все счётчики для разрешённых сообщений увеличиваются атомарно.

При отправке сообщения (из очереди) проверяются лимиты доменов получателей:

6. Лимит домена получателя (для каждого получателя)

Если лимит домена получателя превышен, сообщение **откладывается** (не отклоняется) и будет повторно отправлено.

### Нулевое значение

Установка лимита в `0` означает без ограничений:

```yaml
rate_limit:
  global:
    messages_per_hour: 0     # Без почасового лимита
    messages_per_day: 100000 # Но с дневным лимитом
```

### Персистентность

Счётчики rate limit сохраняются в BoltDB и переживают перезапуск сервера. Интервал сохранения настраивается:

```yaml
rate_limit:
  flush_interval: 10s  # По умолчанию: 10s
```

## API эндпоинты

### Получить конфигурацию rate limit

```bash
curl http://localhost:8080/api/v1/ratelimits \
  -H "Authorization: Bearer YOUR_API_KEY"
```

Ответ:
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

### Получить статистику rate limit

Получить текущие значения счётчиков для конкретного уровня и ключа:

```bash
# Глобальная статистика
curl http://localhost:8080/api/v1/ratelimits/global/global \
  -H "Authorization: Bearer YOUR_API_KEY"

# Статистика домена
curl http://localhost:8080/api/v1/ratelimits/domain/example.com \
  -H "Authorization: Bearer YOUR_API_KEY"

# Статистика отправителя
curl http://localhost:8080/api/v1/ratelimits/sender/user@example.com \
  -H "Authorization: Bearer YOUR_API_KEY"

# Статистика IP
curl http://localhost:8080/api/v1/ratelimits/ip/192.168.1.1 \
  -H "Authorization: Bearer YOUR_API_KEY"

# Статистика API ключа
curl http://localhost:8080/api/v1/ratelimits/api_key/key-123 \
  -H "Authorization: Bearer YOUR_API_KEY"

# Статистика домена получателя
curl http://localhost:8080/api/v1/ratelimits/recipient_domain/gmail.com \
  -H "Authorization: Bearer YOUR_API_KEY"
```

Ответ:
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

### Обновить лимиты домена

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

## Ответы об ошибках

При превышении лимита API возвращает:

```json
{
  "error": "rate limit exceeded",
  "denied_by": "sender",
  "denied_key": "user@example.com",
  "retry_after": 1800
}
```

- `denied_by`: Какой уровень вызвал отклонение
- `denied_key`: Конкретный ключ, который был ограничен
- `retry_after`: Секунды до сброса лимита

## Мониторинг

### Prometheus метрики

```promql
# Отклонения по уровням
rate(sendry_ratelimit_denied_total[5m])

# Текущий процент использования
sendry_ratelimit_usage_ratio{level="global"}
```

### Логи

События rate limit логируются:

```json
{"level":"warn","component":"ratelimit","msg":"rate limit exceeded","denied_by":"sender","key":"user@example.com","retry_after":"30m"}
```

## Примеры конфигураций

### Высоконагруженные транзакционные письма

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

### Рассылки / Маркетинг

```yaml
rate_limit:
  enabled: true
  global:
    messages_per_hour: 50000
  default_domain:
    messages_per_hour: 5000
  # Мягкие лимиты для массовой рассылки
  default_sender:
    messages_per_hour: 5000
    messages_per_day: 50000
```

### Shared хостинг

```yaml
rate_limit:
  enabled: true
  global:
    messages_per_hour: 10000
  # Строгие лимиты по доменам
  default_domain:
    messages_per_hour: 100
    messages_per_day: 500
  # Очень строгие лимиты по отправителям
  default_sender:
    messages_per_hour: 20
    messages_per_day: 100
```

### Лимиты доменов получателей

Ограничьте исходящие письма к крупным почтовым провайдерам, чтобы избежать блокировки:

```yaml
rate_limit:
  enabled: true
  # По умолчанию для всех доменов получателей
  default_recipient_domain:
    messages_per_hour: 5000
    messages_per_day: 50000
  # Конкретные лимиты для крупных провайдеров
  recipient_domains:
    gmail.com:
      messages_per_hour: 1000
      messages_per_day: 10000
    mail.ru:
      messages_per_hour: 1000
      messages_per_day: 10000
    yandex.ru:
      messages_per_hour: 1000
      messages_per_day: 10000
    yahoo.com:
      messages_per_hour: 1000
      messages_per_day: 10000
    outlook.com:
      messages_per_hour: 1000
      messages_per_day: 10000
    hotmail.com:
      messages_per_hour: 1000
      messages_per_day: 10000
```

### Разработка (отключено)

```yaml
rate_limit:
  enabled: false
```
