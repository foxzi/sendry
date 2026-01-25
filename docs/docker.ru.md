# Развёртывание в Docker

Запуск Sendry с Docker и Docker Compose.

## Быстрый старт

```bash
# Клонировать репозиторий
git clone https://github.com/foxzi/sendry.git
cd sendry

# Скопировать и отредактировать конфиги
cp configs/sendry.example.yaml configs/sendry.yaml
cp configs/web.example.yaml configs/web.yaml

# Запустить сервисы
docker compose up -d
```

## Сервисы

| Сервис | Порт | Описание |
|--------|------|----------|
| sendry | 25, 465, 587 | SMTP/SMTPS сервер |
| sendry | 8080 | HTTP API |
| sendry | 9090 | Prometheus метрики |
| sendry-web | 8088 | Веб-интерфейс управления |

## Конфигурация

### Sendry MTA (configs/sendry.yaml)

```yaml
server:
  hostname: mail.example.com

smtp:
  port: 25
  submission_port: 587
  smtps_port: 465

api:
  port: 8080
  auth:
    api_keys:
      - key: "your-api-key"
        name: "default"

domains:
  example.com:
    mode: production

logging:
  level: info
  format: json
```

### Sendry Web (configs/web.yaml)

```yaml
server:
  listen_addr: ":8088"

database:
  path: "/var/lib/sendry-web/app.db"

auth:
  local_enabled: true
  session_secret: "измените-на-случайную-строку-минимум-32-символа"
  session_ttl: 24h

sendry:
  servers:
    - name: "sendry"
      base_url: "http://sendry:8080"  # Внутренняя сеть Docker
      api_key: "your-api-key"         # Такой же как в sendry.yaml
      env: "prod"

logging:
  level: info
  format: json
```

## Запуск сервисов

### Запустить все сервисы

```bash
docker compose up -d
```

### Запустить только MTA

```bash
docker compose up sendry -d
```

### Запустить только веб-панель

```bash
docker compose up sendry-web -d
```

### Просмотр логов

```bash
# Все сервисы
docker compose logs -f

# Конкретный сервис
docker compose logs -f sendry
docker compose logs -f sendry-web
```

### Остановка сервисов

```bash
docker compose down
```

### Пересборка после изменений

```bash
docker compose up -d --build
```

## Создание первого пользователя (Sendry Web)

После запуска sendry-web создайте администратора:

```bash
docker compose exec sendry-web /usr/bin/sendry-web user create \
  --email admin@example.com \
  --password your-password \
  --config /etc/sendry/web.yaml
```

Затем откройте http://localhost:8088 и войдите.

## Тома (Volumes)

| Том | Путь | Описание |
|-----|------|----------|
| sendry-data | /var/lib/sendry | Очередь, DKIM ключи, метрики |
| sendry-web-data | /var/lib/sendry-web | SQLite база данных |

### Резервное копирование

```bash
# Остановить сервисы
docker compose stop

# Бэкап томов
docker run --rm -v sendry_sendry-data:/data -v $(pwd):/backup alpine \
  tar czf /backup/sendry-data-backup.tar.gz -C /data .

docker run --rm -v sendry_sendry-web-data:/data -v $(pwd):/backup alpine \
  tar czf /backup/sendry-web-data-backup.tar.gz -C /data .

# Запустить сервисы
docker compose start
```

## Переменные окружения

Можно задать в файле `.env` или передать напрямую:

```bash
# .env
VERSION=0.4.0
TZ=Europe/Moscow
```

```bash
# Или передать напрямую
TZ=Europe/Moscow docker compose up -d
```

## Сеть

Оба сервиса используют общую сеть `sendry-net`. Sendry Web подключается к Sendry MTA по внутреннему имени `sendry:8080`.

## Рекомендации для продакшена

1. **Используйте внешние конфиги** - монтируйте конфиги с хоста
2. **Используйте секреты** - не коммитьте API ключи и пароли
3. **Настройте TLS** - используйте reverse proxy (nginx, traefik) для HTTPS
4. **Мониторинг** - подключите Prometheus к :9090/metrics
5. **Бэкапы** - регулярно делайте резервные копии томов

### Пример с Traefik

```yaml
services:
  sendry-web:
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.sendry-web.rule=Host(`panel.example.com`)"
      - "traefik.http.routers.sendry-web.tls.certresolver=letsencrypt"
      - "traefik.http.services.sendry-web.loadbalancer.server.port=8088"
```

## Устранение неполадок

### Контейнер не запускается

```bash
# Проверить логи
docker compose logs sendry
docker compose logs sendry-web

# Проверить синтаксис конфига
docker compose exec sendry /usr/bin/sendry config validate
```

### Веб-панель не может подключиться к MTA

1. Проверьте, что API ключ совпадает в обоих конфигах
2. Проверьте `base_url` - должен быть Docker hostname: `http://sendry:8080`
3. Проверьте, что оба контейнера в одной сети

### Permission denied на томах

```bash
# Исправить права
docker compose exec sendry chown -R sendry:sendry /var/lib/sendry
docker compose exec sendry-web chown -R sendry:sendry /var/lib/sendry-web
```
