# Настройка TLS и DKIM

## TLS (Transport Layer Security)

Sendry поддерживает TLS для безопасной передачи почты:

- **STARTTLS** на портах 25 и 587 - обновление соединения до TLS
- **SMTPS** на порту 465 - неявный TLS с начала соединения

### Ручные сертификаты

```yaml
smtp:
  tls:
    cert_file: "/etc/sendry/certs/server.crt"
    key_file: "/etc/sendry/certs/server.key"
```

### Let's Encrypt (ACME)

Автоматическое управление сертификатами через Let's Encrypt:

```yaml
smtp:
  tls:
    acme:
      enabled: true
      email: "admin@example.com"
      domains:
        - "mail.example.com"
      cache_dir: "/var/lib/sendry/certs"
```

Требования для ACME:
- Порт 80 должен быть доступен для HTTP-01 проверки
- DNS должен указывать на сервер

### Проверка TLS

```bash
# Проверка STARTTLS на порту 25
openssl s_client -starttls smtp -connect localhost:25

# Проверка STARTTLS на порту 587
openssl s_client -starttls smtp -connect localhost:587

# Проверка SMTPS на порту 465
openssl s_client -connect localhost:465
```

## DKIM (DomainKeys Identified Mail)

DKIM подписывает исходящие письма для аутентификации.

### Генерация ключа DKIM

```bash
sendry dkim generate --domain example.com --selector sendry --out /etc/sendry/dkim/
```

Вывод:
```
DKIM key generated successfully

Private key saved to: /etc/sendry/dkim/example.com.key

DNS Record:
  Name: sendry._domainkey.example.com
  Type: TXT
  Value: v=DKIM1; k=rsa; p=MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A...
```

### Показать DNS-запись DKIM

```bash
sendry dkim show --key /etc/sendry/dkim/example.com.key --domain example.com --selector sendry
```

### Конфигурация

```yaml
dkim:
  enabled: true
  selector: "sendry"
  domain: "example.com"
  key_file: "/etc/sendry/dkim/example.com.key"
```

### Настройка DNS

Добавьте TXT-запись в ваш DNS:

```
sendry._domainkey.example.com. IN TXT "v=DKIM1; k=rsa; p=MIIBIjAN..."
```

### Проверка настройки DKIM

Отправьте тестовое письмо и проверьте через:
- [mail-tester.com](https://www.mail-tester.com/)
- Gmail (проверьте заголовки письма)
- [dkimvalidator.com](https://dkimvalidator.com/)

## Полный пример конфигурации

```yaml
server:
  hostname: "mail.example.com"

smtp:
  listen_addr: ":25"
  submission_addr: ":587"
  smtps_addr: ":465"
  domain: "example.com"
  max_message_bytes: 10485760
  max_recipients: 100
  read_timeout: 60s
  write_timeout: 60s
  tls:
    acme:
      enabled: true
      email: "admin@example.com"
      domains:
        - "mail.example.com"
      cache_dir: "/var/lib/sendry/certs"
  auth:
    required: false

dkim:
  enabled: true
  selector: "sendry"
  domain: "example.com"
  key_file: "/etc/sendry/dkim/example.com.key"

api:
  listen_addr: ":8080"
  api_key: "your-api-key"

queue:
  workers: 4
  retry_interval: 5m
  max_retries: 5
  process_interval: 10s

storage:
  path: "/var/lib/sendry/queue.db"

logging:
  level: "info"
  format: "json"
```
