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

### Через CLI (Рекомендуется)

```bash
# Отправить тестовое письмо через локальный сервер
sendry test send -c config.yaml --to user@example.com

# С указанием темы и текста
sendry test send -c config.yaml --to user@example.com --subject "Тест" --body "Привет!"

# Без TLS для локального тестирования без сертификатов
sendry test send -c config.yaml --to user@example.com --no-tls

# Использовать определённый порт
sendry test send -c config.yaml --to user@example.com --port 2525
```

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

## Шаблоны писем

### Создание шаблона

```bash
# Через CLI
sendry template create -c config.yaml --name welcome --subject "Привет {{.Name}}" --text welcome.txt

# Через API
curl -X POST http://localhost:8080/api/v1/templates \
  -H "X-API-Key: test-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "welcome",
    "subject": "Привет {{.Name}}",
    "text": "Добро пожаловать {{.Name}}!\nВаш заказ #{{.OrderID}} подтверждён.",
    "html": "<p>Добро пожаловать <b>{{.Name}}</b>!</p>"
  }'
```

### Предпросмотр шаблона

```bash
# Через CLI
sendry template preview -c config.yaml welcome --data '{"Name":"Иван","OrderID":"12345"}'

# Через API
curl -X POST http://localhost:8080/api/v1/templates/{id}/preview \
  -H "X-API-Key: test-api-key" \
  -H "Content-Type: application/json" \
  -d '{"data":{"Name":"Иван","OrderID":"12345"}}'
```

### Отправка письма по шаблону

```bash
curl -X POST http://localhost:8080/api/v1/send/template \
  -H "X-API-Key: test-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "template_id": "{id}",
    "from": "noreply@example.com",
    "to": ["user@example.com"],
    "data": {"Name": "Иван", "OrderID": "12345"}
  }'
```

Подробнее см. [Руководство по шаблонам](templates.ru.md).

## Веб-панель Sendry Web

Sendry Web - веб-интерфейс для управления email-рассылками, шаблонами и списками получателей.

### Запуск Sendry Web

```bash
# Из бинарника
sendry-web serve --config /etc/sendry/web.yaml

# Через Docker Compose
docker compose up -d
```

Панель доступна по адресу `http://localhost:8088`.

### Создание рассылки (пошагово)

#### 1. Настройка подключения к серверу

Отредактируйте `web.yaml`, добавив ваш Sendry MTA сервер:

```yaml
sendry:
  servers:
    - name: "mta-1"
      base_url: "http://localhost:8080"
      api_key: "your-api-key"
      env: "prod"
```

#### 2. Создание шаблона

1. Перейдите в **Templates** → **New Template**
2. Заполните:
   - **Name**: например, "Приветственное письмо"
   - **Subject**: например, "Добро пожаловать, {{name}}!"
   - **Content**: HTML-содержимое с переменными `{{name}}`, `{{email}}`
3. Выберите режим редактора:
   - **Visual** (Quill) - визуальный редактор
   - **Blocks** (Editor.js) - блочный редактор
   - **Code** (CodeMirror) - редактор кода
4. Нажмите **Save**
5. Нажмите **Deploy** для отправки шаблона на MTA сервер

#### 3. Создание списка получателей

1. Перейдите в **Recipients** → **New List**
2. Введите название списка (например, "Подписчики рассылки")
3. Нажмите **Create**
4. Добавьте получателей:
   - **Вручную**: нажмите **Add Recipient**, введите email и переменные
   - **Импорт CSV**: нажмите **Import**, загрузите CSV файл

Пример формата CSV:
```csv
email,name,company
ivan@example.com,Иван Петров,ООО Рога и Копыта
anna@example.com,Анна Сидорова,ИП Сидорова
```

#### 4. Создание кампании

1. Перейдите в **Campaigns** → **New Campaign**
2. Заполните:
   - **Name**: например, "Январская рассылка"
   - **From**: email отправителя (например, `newsletter@example.com`)
   - **Template**: выберите шаблон
   - **Recipient List**: выберите список получателей
3. Нажмите **Create**
4. (Опционально) Добавьте **Variables** для кампании (доступны в шаблонах как `{{var_name}}`)
5. (Опционально) Создайте **Variants** для A/B тестирования

#### 5. Запуск рассылки

1. Откройте кампанию → нажмите **Send**
2. Настройте параметры отправки:
   - **Server**: выберите MTA сервер
   - **Schedule**: отправить сейчас или запланировать
   - **Dry Run**: тестовый запуск без реальной отправки
   - **Batch Size**: писем за раз
   - **Delay**: пауза между пачками
3. Нажмите **Start Job**

#### 6. Мониторинг прогресса

1. Перейдите в **Jobs** для просмотра всех задач
2. Кликните на задачу для просмотра:
   - Прогресс (отправлено/ошибки/в очереди)
   - Статус каждого письма
   - Сообщения об ошибках
3. Доступные действия:
   - **Pause** / **Resume** - приостановить/возобновить
   - **Cancel** - отменить задачу
   - **Retry Failed** - повторить неудачные

### Настройка DKIM (для сервера)

1. Перейдите в **Servers** → выберите сервер → **DKIM Keys**
2. Нажмите **New DKIM Key**
3. Введите:
   - **Domain**: например, `example.com`
   - **Selector**: например, `sendry`
4. Отметьте **Deploy to server after creation**
5. Нажмите **Generate Key**
6. Добавьте отображённую DNS TXT запись в ваш домен

### Настройка домена

1. Перейдите в **Servers** → выберите сервер → **Domains**
2. Нажмите **New Domain**
3. Настройте:
   - **Domain**: например, `example.com`
   - **Mode**: production/sandbox/redirect/bcc
   - **DKIM**: включите и выберите ключ
   - **Rate Limits**: лимиты писем в час/день
4. Нажмите **Create**

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
- [Руководство по шаблонам](templates.ru.md)
- [Правила заголовков](header-rules.ru.md)
- [Справочник API](api.ru.md)
- [Справочник конфигурации](configuration.ru.md)
