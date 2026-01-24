# Sendry: Быстрый старт

## Установка

### Из бинарника

```bash
# Скачать последний релиз
curl -LO https://github.com/foxzi/sendry/releases/latest/download/sendry-linux-amd64
chmod +x sendry-linux-amd64
sudo mv sendry-linux-amd64 /usr/local/bin/sendry
```

### Из исходников

```bash
git clone https://github.com/foxzi/sendry.git
cd sendry
make build
sudo cp build/sendry /usr/local/bin/
```

### Docker

```bash
docker pull ghcr.io/foxzi/sendry:latest
```

## Мастер настройки

Самый простой способ создать конфигурацию - использовать команду init:

```bash
# Интерактивный режим - запрашивает значения
sendry init

# Неинтерактивный режим с флагами
sendry init --domain example.com --hostname mail.example.com --dkim

# Быстрая настройка sandbox для тестирования
sendry init --domain test.local --mode sandbox -o test.yaml
```

Мастер выполнит:
- Генерацию полного файла конфигурации
- Создание DKIM ключей (опционально)
- Покажет все DNS записи (SPF, DKIM, DMARC)
- Сгенерирует безопасный API ключ и SMTP пароль

## Быстрый тест (Sandbox режим)

Создайте тестовый конфиг `test.yaml`:

```yaml
server:
  hostname: "localhost"

smtp:
  listen_addr: ":2525"
  submission_addr: ":2587"
  domain: "test.local"
  auth:
    required: false

domains:
  test.local:
    mode: sandbox

api:
  listen_addr: ":8080"
  api_key: "test-api-key"

queue:
  workers: 2

storage:
  path: "./data/queue.db"

logging:
  level: "debug"
  format: "text"
```

Запустите сервер:

```bash
mkdir -p data
sendry serve -c test.yaml
```

## Отправка тестовых писем

### Через SMTP

```bash
# Используя netcat
echo -e "EHLO test\nMAIL FROM:<test@test.local>\nRCPT TO:<user@example.com>\nDATA\nSubject: Test\n\nHello World\n.\nQUIT" | nc localhost 2525

# Используя swaks (если установлен)
swaks --to user@example.com --from test@test.local --server localhost:2525
```

### Через HTTP API

```bash
curl -X POST http://localhost:8080/api/v1/send \
  -H "X-API-Key: test-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "from": "api@test.local",
    "to": ["user@example.com"],
    "subject": "Тестовое письмо",
    "body": "Привет от Sendry!"
  }'
```

## Просмотр перехваченных писем (Sandbox)

```bash
# Список всех сообщений
curl -H "X-API-Key: test-api-key" http://localhost:8080/api/v1/sandbox/messages

# Получить конкретное сообщение
curl -H "X-API-Key: test-api-key" http://localhost:8080/api/v1/sandbox/messages/{id}

# Статистика
curl -H "X-API-Key: test-api-key" http://localhost:8080/api/v1/sandbox/stats
```

## Проверка здоровья

```bash
curl http://localhost:8080/health
```

## Конфигурация для продакшена

### Используя мастер Init (Рекомендуется)

```bash
# Полная настройка с DKIM и Let's Encrypt
sendry init --domain example.com --dkim --acme --acme-email admin@example.com

# Или интерактивный режим
sendry init
```

### Ручная настройка

Для ручной настройки смотрите [полный пример конфигурации](../configs/sendry.example.yaml).

Основные шаги:
1. Настройте TLS сертификаты или включите ACME
2. Настройте DKIM подпись
3. Включите аутентификацию
4. Настройте лимиты
5. Установите режим домена `production`

### Генерация DKIM ключа (Вручную)

```bash
sendry dkim generate --domain example.com --selector sendry --out /var/lib/sendry/dkim/
```

Добавьте показанную DNS TXT запись.

### Проверка репутации IP

Перед запуском в продакшен проверьте, не в blacklist ли ваш IP:

```bash
sendry ip check <ip-вашего-сервера>
```

## Порты

| Порт | Сервис | Описание |
|------|--------|----------|
| 25 | SMTP | Приём почты от других серверов |
| 587 | Submission | Отправка почты от клиентов (STARTTLS) |
| 465 | SMTPS | Отправка почты от клиентов (implicit TLS) |
| 8080 | HTTP API | REST API для отправки и управления |

## Режимы доменов

| Режим | Описание |
|-------|----------|
| `production` | Обычная доставка получателям |
| `sandbox` | Перехват писем локально (для тестирования) |
| `redirect` | Перенаправление всех писем на указанные адреса |
| `bcc` | Обычная доставка + копия в архив |

## Дальнейшие шаги

- [Настройка TLS и DKIM](tls-dkim.ru.md)
- [Справочник API](api.ru.md)
- [Справочник конфигурации](configuration.ru.md)
