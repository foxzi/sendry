# Справочник по HTTP API

Sendry предоставляет полнофункциональный HTTP API для отправки писем, управления очередью, шаблонами, доменами и многим другим.

## Аутентификация

Все эндпоинты API (кроме `/health`) требуют аутентификации через API-ключ:

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" http://localhost:8080/api/v1/...
```

## Базовый URL

По умолчанию: `http://localhost:8080`

---

## Основные эндпоинты

### Health Check

Проверка состояния сервера. Аутентификация не требуется.

```
GET /health
```

**Ответ:**
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

Добавить письмо в очередь на отправку.

```
POST /api/v1/send
```

**Запрос:**
```json
{
  "from": "sender@example.com",
  "to": ["recipient@example.com"],
  "subject": "Привет",
  "body": "Текстовое содержимое",
  "html": "<p>HTML содержимое</p>",
  "headers": {
    "X-Custom-Header": "значение"
  }
}
```

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `from` | string | Да | Email отправителя |
| `to` | array | Да | Email адреса получателей |
| `subject` | string | Да* | Тема письма |
| `body` | string | Да* | Текстовое тело |
| `html` | string | Нет | HTML тело |
| `headers` | object | Нет | Дополнительные заголовки |

*Требуется хотя бы одно из: `subject`, `body` или `html`.

**Ответ (202 Accepted):**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending"
}
```

### Получить статус сообщения

Получить статус доставки сообщения.

```
GET /api/v1/status/{id}
```

**Ответ:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "delivered",
  "from": "sender@example.com",
  "to": ["recipient@example.com"],
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:30:05Z",
  "retry_count": 0,
  "last_error": ""
}
```

**Значения статусов:**
| Статус | Описание |
|--------|----------|
| `pending` | Ожидает отправки |
| `sending` | Отправляется |
| `delivered` | Успешно доставлено |
| `deferred` | Временная ошибка, повторная попытка |
| `failed` | Постоянная ошибка |

### Статистика очереди

Получить статистику очереди и список сообщений.

```
GET /api/v1/queue
```

**Ответ:**
```json
{
  "stats": {
    "pending": 5,
    "sending": 1,
    "delivered": 100,
    "failed": 2,
    "deferred": 3,
    "total": 111
  },
  "messages": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "from": "sender@example.com",
      "to": ["recipient@example.com"],
      "status": "pending",
      "created_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

### Удалить сообщение

Удалить сообщение из очереди.

```
DELETE /api/v1/queue/{id}
```

**Ответ:** `204 No Content`

---

## Очередь недоставленных писем (DLQ)

Сообщения с ошибками перемещаются в DLQ для ручной проверки.

### Список DLQ

```
GET /api/v1/dlq
```

**Ответ:**
```json
{
  "stats": {
    "count": 5,
    "oldest_at": "2024-01-10T08:00:00Z",
    "newest_at": "2024-01-15T10:00:00Z"
  },
  "messages": [
    {
      "id": "...",
      "from": "sender@example.com",
      "to": ["recipient@example.com"],
      "status": "failed",
      "created_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

### Получить сообщение из DLQ

```
GET /api/v1/dlq/{id}
```

**Ответ:** Аналогичен ответу статуса сообщения.

### Повторить отправку из DLQ

Переместить сообщение обратно в очередь для повторной отправки.

```
POST /api/v1/dlq/{id}/retry
```

**Ответ:**
```json
{
  "status": "ok",
  "message": "Message moved to pending queue"
}
```

### Удалить сообщение из DLQ

```
DELETE /api/v1/dlq/{id}
```

**Ответ:** `204 No Content`

---

## Шаблоны

Email-шаблоны с подстановкой переменных.

### Список шаблонов

```
GET /api/v1/templates
```

**Параметры запроса:**
| Параметр | Описание |
|----------|----------|
| `search` | Поиск по имени |
| `limit` | Макс. результатов (по умолчанию: 100) |
| `offset` | Пропустить N результатов |

**Ответ:**
```json
{
  "templates": [
    {
      "id": "...",
      "name": "welcome",
      "description": "Приветственное письмо",
      "subject": "Добро пожаловать, {{.Name}}!",
      "html": "<h1>Привет, {{.Name}}</h1>",
      "text": "Привет, {{.Name}}",
      "variables": [
        {"name": "Name", "required": true, "default": ""}
      ],
      "version": 1,
      "created_at": "2024-01-15T10:00:00Z",
      "updated_at": "2024-01-15T10:00:00Z"
    }
  ],
  "total": 1
}
```

### Создать шаблон

```
POST /api/v1/templates
```

**Запрос:**
```json
{
  "name": "welcome",
  "description": "Шаблон приветственного письма",
  "subject": "Добро пожаловать, {{.Name}}!",
  "html": "<h1>Привет, {{.Name}}</h1>",
  "text": "Привет, {{.Name}}",
  "variables": [
    {"name": "Name", "required": true, "default": "Пользователь"}
  ]
}
```

**Ответ (201 Created):** Объект шаблона.

### Получить шаблон

```
GET /api/v1/templates/{id}
```

Примечание: `{id}` может быть ID шаблона или его имя.

**Ответ:** Объект шаблона.

### Обновить шаблон

```
PUT /api/v1/templates/{id}
```

**Запрос:** Аналогичен созданию (все поля необязательны).

**Ответ:** Обновленный объект шаблона.

### Удалить шаблон

```
DELETE /api/v1/templates/{id}
```

**Ответ:** `204 No Content`

### Предпросмотр шаблона

Отрендерить шаблон с тестовыми данными.

```
POST /api/v1/templates/{id}/preview
```

**Запрос:**
```json
{
  "data": {
    "Name": "Иван"
  }
}
```

**Ответ:**
```json
{
  "subject": "Добро пожаловать, Иван!",
  "html": "<h1>Привет, Иван</h1>",
  "text": "Привет, Иван"
}
```

### Отправка через шаблон

Отправить письмо с использованием шаблона.

```
POST /api/v1/send/template
```

**Запрос:**
```json
{
  "template_id": "...",
  "template_name": "welcome",
  "from": "noreply@example.com",
  "to": ["user@example.com"],
  "cc": [],
  "bcc": [],
  "data": {
    "Name": "Иван"
  },
  "headers": {}
}
```

Примечание: Укажите либо `template_id`, либо `template_name`.

**Ответ (202 Accepted):**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending"
}
```

---

## Песочница (Sandbox)

Режим песочницы перехватывает письма локально для тестирования. Доступен когда домены настроены с `mode: sandbox` или `mode: redirect`.

### Список сообщений песочницы

```
GET /api/v1/sandbox/messages
```

**Параметры запроса:**
| Параметр | Описание |
|----------|----------|
| `domain` | Фильтр по домену |
| `mode` | Фильтр по режиму (sandbox/redirect/bcc) |
| `from` | Фильтр по отправителю |
| `limit` | Макс. результатов (по умолчанию: 100) |
| `offset` | Пропустить N результатов |

**Ответ:**
```json
{
  "messages": [
    {
      "id": "...",
      "from": "sender@example.com",
      "to": ["test@sandbox.example.com"],
      "original_to": ["real@example.com"],
      "subject": "Тестовое письмо",
      "domain": "sandbox.example.com",
      "mode": "redirect",
      "captured_at": "2024-01-15T10:30:00Z",
      "client_ip": "192.168.1.100",
      "simulated_error": ""
    }
  ],
  "total": 1
}
```

### Получить сообщение

```
GET /api/v1/sandbox/messages/{id}
```

**Ответ:**
```json
{
  "id": "...",
  "from": "sender@example.com",
  "to": ["test@sandbox.example.com"],
  "subject": "Тестовое письмо",
  "domain": "sandbox.example.com",
  "mode": "sandbox",
  "captured_at": "2024-01-15T10:30:00Z",
  "headers": {
    "From": "sender@example.com",
    "To": "test@sandbox.example.com",
    "Subject": "Тестовое письмо"
  },
  "body": "Текстовое содержимое",
  "html": "<p>HTML содержимое</p>",
  "size": 1234
}
```

### Получить сырые данные

Скачать сырые данные письма в формате RFC 5322.

```
GET /api/v1/sandbox/messages/{id}/raw
```

**Ответ:** Контент типа `message/rfc822` с расширением `.eml`.

### Переотправить сообщение

Добавить сообщение из песочницы обратно в очередь для реальной доставки.

```
POST /api/v1/sandbox/messages/{id}/resend
```

**Ответ:**
```json
{
  "status": "queued",
  "message_id": "...-resend-20240115103000"
}
```

### Очистить песочницу

Удалить несколько сообщений из песочницы.

```
DELETE /api/v1/sandbox/messages
```

**Параметры запроса:**
| Параметр | Описание |
|----------|----------|
| `domain` | Очистить только сообщения для этого домена |
| `older_than` | Очистить сообщения старше указанного времени (напр., `24h`, `7d`) |

**Ответ:**
```json
{
  "cleared": 50
}
```

### Удалить сообщение

```
DELETE /api/v1/sandbox/messages/{id}
```

**Ответ:** `204 No Content`

### Статистика песочницы

```
GET /api/v1/sandbox/stats
```

**Ответ:**
```json
{
  "total": 100,
  "by_domain": {
    "sandbox.example.com": 50,
    "test.example.com": 50
  },
  "by_mode": {
    "sandbox": 70,
    "redirect": 30
  },
  "oldest_at": "2024-01-10T08:00:00Z",
  "newest_at": "2024-01-15T10:30:00Z",
  "total_size": 524288
}
```

---

## Управление доменами

Управление конфигурацией доменов в runtime.

### Список доменов

```
GET /api/v1/domains
```

**Ответ:**
```json
{
  "domains": [
    {
      "domain": "example.com",
      "mode": "production",
      "default_from": "noreply@example.com",
      "dkim": {
        "enabled": true,
        "selector": "default"
      },
      "rate_limit": {
        "messages_per_hour": 1000,
        "messages_per_day": 10000
      }
    }
  ]
}
```

### Создать домен

```
POST /api/v1/domains
```

**Запрос:**
```json
{
  "domain": "newdomain.com",
  "mode": "production",
  "default_from": "noreply@newdomain.com",
  "dkim": {
    "enabled": true,
    "selector": "default",
    "key_file": "/path/to/key.pem"
  },
  "tls": {
    "cert_file": "/path/to/cert.pem",
    "key_file": "/path/to/key.pem"
  },
  "rate_limit": {
    "messages_per_hour": 500,
    "messages_per_day": 5000,
    "recipients_per_message": 50
  },
  "redirect_to": [],
  "bcc_to": []
}
```

**Ответ (201 Created):** Объект домена.

### Получить домен

```
GET /api/v1/domains/{domain}
```

**Ответ:** Объект домена.

### Обновить домен

```
PUT /api/v1/domains/{domain}
```

**Запрос:** Аналогичен созданию (все поля необязательны).

**Ответ:** Обновленный объект домена.

### Удалить домен

```
DELETE /api/v1/domains/{domain}
```

Примечание: Нельзя удалить основной SMTP домен.

**Ответ:** `204 No Content`

---

## Управление DKIM

### Сгенерировать DKIM ключ

```
POST /api/v1/dkim/generate
```

**Запрос:**
```json
{
  "domain": "example.com",
  "selector": "default"
}
```

**Ответ (201 Created):**
```json
{
  "domain": "example.com",
  "selector": "default",
  "dns_name": "default._domainkey.example.com",
  "dns_record": "v=DKIM1; k=rsa; p=MIIBIjANBgkq...",
  "key_file": "/var/lib/sendry/dkim/example.com/default.key"
}
```

### Получить информацию о DKIM

```
GET /api/v1/dkim/{domain}
```

**Ответ:**
```json
{
  "domain": "example.com",
  "enabled": true,
  "selector": "default",
  "key_file": "/var/lib/sendry/dkim/example.com/default.key",
  "dns_name": "default._domainkey.example.com",
  "dns_record": "v=DKIM1; k=rsa; p=MIIBIjANBgkq...",
  "selectors": ["default", "backup"]
}
```

### Проверить конфигурацию DKIM

```
GET /api/v1/dkim/{domain}/verify?selector=default
```

**Ответ:**
```json
{
  "domain": "example.com",
  "selector": "default",
  "valid": true,
  "error": "",
  "dns_name": "default._domainkey.example.com"
}
```

### Удалить DKIM ключ

```
DELETE /api/v1/dkim/{domain}/{selector}
```

**Ответ:** `204 No Content`

---

## Управление TLS

### Список сертификатов

```
GET /api/v1/tls/certificates
```

**Ответ:**
```json
{
  "certificates": [
    {
      "domain": "mail.example.com",
      "cert_file": "/path/to/cert.pem",
      "key_file": "/path/to/key.pem",
      "acme": false
    }
  ],
  "acme_enabled": true,
  "acme_domains": ["mail.example.com"]
}
```

### Загрузить сертификат

```
POST /api/v1/tls/certificates
```

**Запрос:**
```json
{
  "domain": "mail.example.com",
  "certificate": "-----BEGIN CERTIFICATE-----\n...",
  "private_key": "-----BEGIN PRIVATE KEY-----\n..."
}
```

**Ответ (201 Created):** Объект информации о сертификате.

### Запросить сертификат Let's Encrypt

```
POST /api/v1/tls/letsencrypt/{domain}
```

Примечание: Домен должен быть в списке разрешенных ACME доменов.

**Ответ (202 Accepted):**
```json
{
  "status": "pending",
  "message": "Certificate will be obtained automatically on first TLS connection",
  "domain": "mail.example.com"
}
```

---

## Rate Limiting

### Получить конфигурацию лимитов

```
GET /api/v1/ratelimits
```

**Ответ:**
```json
{
  "enabled": true,
  "global": {
    "messages_per_hour": 10000,
    "messages_per_day": 100000
  },
  "default_domain": {
    "messages_per_hour": 1000,
    "messages_per_day": 10000
  },
  "default_sender": {
    "messages_per_hour": 100,
    "messages_per_day": 1000
  },
  "default_ip": {
    "messages_per_hour": 500,
    "messages_per_day": 5000
  },
  "default_api_key": {
    "messages_per_hour": 1000,
    "messages_per_day": 10000
  },
  "domains": {
    "example.com": {
      "messages_per_hour": 2000,
      "messages_per_day": 20000,
      "recipients_per_message": 100
    }
  }
}
```

### Получить статистику лимитов

```
GET /api/v1/ratelimits/{level}/{key}
```

**Уровни:** `global`, `domain`, `sender`, `ip`, `api_key`

**Ответ:**
```json
{
  "level": "domain",
  "key": "example.com",
  "hourly_count": 150,
  "daily_count": 1500,
  "hourly_limit": 1000,
  "daily_limit": 10000
}
```

### Обновить лимиты домена

```
PUT /api/v1/ratelimits/{domain}
```

**Запрос:**
```json
{
  "messages_per_hour": 2000,
  "messages_per_day": 20000,
  "recipients_per_message": 100
}
```

**Ответ:** Обновленный объект лимитов.

---

## Проверка DNS

Проверка DNS записей для доменов и репутации IP.

### Проверить DNS записи домена

```
GET /api/v1/dns/check/{domain}
```

Проверка MX, SPF, DKIM, DMARC и MTA-STS записей для домена.

**Query параметры:**

| Параметр | По умолчанию | Описание |
|----------|--------------|----------|
| `mx` | false | Проверить только MX записи |
| `spf` | false | Проверить только SPF запись |
| `dkim` | false | Проверить только DKIM запись |
| `dmarc` | false | Проверить только DMARC запись |
| `mta_sts` | false | Проверить только MTA-STS запись |
| `selector` | sendry | DKIM селектор для проверки |

Если конкретная проверка не указана, проверяются все записи.

**Ответ:**
```json
{
  "domain": "example.com",
  "results": [
    {
      "type": "MX Records",
      "status": "ok",
      "value": "mail.example.com (priority 10)",
      "message": "1 MX record(s) found"
    },
    {
      "type": "SPF Record",
      "status": "ok",
      "value": "v=spf1 include:_spf.example.com -all",
      "message": "SPF configured with strict policy (-all)"
    },
    {
      "type": "DKIM Record (sendry._domainkey)",
      "status": "ok",
      "value": "v=DKIM1; k=rsa; p=MIIBIjANBgkq...",
      "message": "DKIM configured with RSA key"
    },
    {
      "type": "DMARC Record",
      "status": "ok",
      "value": "v=DMARC1; p=reject; rua=mailto:dmarc@example.com",
      "message": "DMARC configured with reject policy (strict)"
    },
    {
      "type": "MTA-STS Record",
      "status": "not_found",
      "message": "No MTA-STS record found (optional)"
    }
  ],
  "summary": {
    "ok": 4,
    "warnings": 0,
    "errors": 0,
    "not_found": 1
  }
}
```

**Значения статуса:** `ok`, `warning`, `error`, `not_found`

### Проверить IP в DNSBL

```
GET /api/v1/ip/check/{ip}
```

Проверка IPv4 адреса в DNS-based blackhole списках (DNSBL).

**Ответ:**
```json
{
  "ip": "1.2.3.4",
  "results": [
    {
      "dnsbl": {
        "name": "Spamhaus ZEN",
        "zone": "zen.spamhaus.org",
        "description": "Combined Spamhaus blocklist (SBL, XBL, PBL)"
      },
      "listed": false,
      "return_codes": null,
      "error": ""
    },
    {
      "dnsbl": {
        "name": "Barracuda",
        "zone": "b.barracudacentral.org",
        "description": "Barracuda Reputation Block List"
      },
      "listed": true,
      "return_codes": ["127.0.0.2"],
      "error": ""
    }
  ],
  "summary": {
    "clean": 17,
    "listed": 1,
    "errors": 0
  }
}
```

### Список DNSBL сервисов

```
GET /api/v1/ip/dnsbls
```

Список всех DNS blacklist сервисов для проверки.

**Ответ:**
```json
{
  "dnsbls": [
    {
      "name": "Spamhaus ZEN",
      "zone": "zen.spamhaus.org",
      "description": "Combined Spamhaus blocklist (SBL, XBL, PBL)"
    },
    {
      "name": "Barracuda",
      "zone": "b.barracudacentral.org",
      "description": "Barracuda Reputation Block List"
    }
  ],
  "count": 15
}
```

---

## Ответы об ошибках

Все эндпоинты возвращают ошибки в следующем формате:

```json
{
  "error": "Описание ошибки"
}
```

**Распространенные HTTP коды:**

| Код | Описание |
|-----|----------|
| 200 | Успех |
| 201 | Создано |
| 202 | Принято (поставлено в очередь) |
| 204 | Нет содержимого (успешное удаление) |
| 400 | Неверный запрос (некорректные данные) |
| 401 | Не авторизован (отсутствует/неверный API ключ) |
| 403 | Доступ запрещен |
| 404 | Не найдено |
| 409 | Конфликт (напр., дублирующееся имя) |
| 429 | Слишком много запросов (превышен лимит) |
| 500 | Внутренняя ошибка сервера |
| 503 | Сервис недоступен |
