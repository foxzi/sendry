# Руководство по шаблонам

Sendry поддерживает шаблоны писем с синтаксисом Go templates для динамического контента.

## Конфигурация

Шаблоны работают из коробки без дополнительной настройки. Используется тот же BoltDB что и для очереди сообщений:

```yaml
storage:
  path: "/var/lib/sendry/queue.db"

api:
  listen_addr: ":8080"
  api_key: "your-api-key"
```

Шаблоны хранятся в отдельном bucket внутри файла базы данных.

## Возможности

- Go templates (`text/template` для текста, `html/template` для HTML)
- Автоматическая защита от XSS в HTML шаблонах
- Версионирование шаблонов
- Предпросмотр с тестовыми данными
- Управление через CLI и API

## Структура шаблона

```json
{
  "name": "welcome",
  "description": "Приветственное письмо для новых пользователей",
  "subject": "Добро пожаловать {{.Name}}!",
  "text": "Привет {{.Name}},\n\nДобро пожаловать в наш сервис!",
  "html": "<p>Привет <b>{{.Name}}</b>,</p><p>Добро пожаловать в наш сервис!</p>",
  "variables": [
    {
      "name": "Name",
      "type": "string",
      "required": true,
      "description": "Имя пользователя"
    }
  ]
}
```

## Синтаксис шаблонов

Sendry использует синтаксис Go templates. Основные паттерны:

### Переменные

```
{{.Name}}           - Простая переменная
{{.User.Email}}     - Вложенное поле
```

### Условия

```
{{if .Premium}}
  Премиум контент
{{else}}
  Стандартный контент
{{end}}
```

### Циклы

```
{{range .Items}}
  - {{.Name}}: {{.Price}}
{{end}}
```

### Значения по умолчанию

```
{{if .Name}}{{.Name}}{{else}}Клиент{{end}}
```

## CLI команды

### Список шаблонов

```bash
sendry template list -c config.yaml
```

### Создание шаблона

```bash
# Из командной строки
sendry template create -c config.yaml \
  --name welcome \
  --subject "Привет {{.Name}}" \
  --text welcome.txt \
  --html welcome.html

# Минимальный (только текст)
sendry template create -c config.yaml \
  --name simple \
  --subject "Уведомление" \
  --text message.txt
```

### Просмотр шаблона

```bash
sendry template show -c config.yaml welcome
sendry template show -c config.yaml {template-id}
```

### Предпросмотр шаблона

```bash
sendry template preview -c config.yaml welcome \
  --data '{"Name":"Иван","OrderID":"12345"}'
```

### Удаление шаблона

```bash
sendry template delete -c config.yaml {template-id}
```

### Экспорт шаблона

```bash
sendry template export -c config.yaml welcome --output ./templates/
```

Создаёт:
- `welcome.json` - метаданные и тема
- `welcome.html` - HTML шаблон (если есть)
- `welcome.txt` - текстовый шаблон (если есть)

### Импорт шаблона

```bash
sendry template import -c config.yaml \
  --name welcome \
  --subject "Привет {{.Name}}" \
  --html ./templates/welcome.html \
  --text ./templates/welcome.txt
```

## API эндпоинты

### Список шаблонов

```bash
GET /api/v1/templates
```

Ответ:
```json
{
  "templates": [
    {
      "id": "abc123",
      "name": "welcome",
      "subject": "Привет {{.Name}}",
      "version": 1,
      "created_at": "2024-01-15T10:00:00Z",
      "updated_at": "2024-01-15T10:00:00Z"
    }
  ],
  "total": 1
}
```

### Создание шаблона

```bash
POST /api/v1/templates
```

Запрос:
```json
{
  "name": "welcome",
  "description": "Приветственное письмо",
  "subject": "Привет {{.Name}}",
  "text": "Добро пожаловать {{.Name}}!",
  "html": "<p>Добро пожаловать <b>{{.Name}}</b>!</p>",
  "variables": [
    {"name": "Name", "required": true}
  ]
}
```

### Получение шаблона

```bash
GET /api/v1/templates/{id}
```

### Обновление шаблона

```bash
PUT /api/v1/templates/{id}
```

Запрос:
```json
{
  "subject": "Обновлено: Привет {{.Name}}",
  "text": "Обновлённое приветствие"
}
```

Версия увеличивается автоматически при каждом обновлении.

### Удаление шаблона

```bash
DELETE /api/v1/templates/{id}
```

### Предпросмотр шаблона

```bash
POST /api/v1/templates/{id}/preview
```

Запрос:
```json
{
  "data": {
    "Name": "Иван",
    "OrderID": "12345"
  }
}
```

Ответ:
```json
{
  "subject": "Привет Иван",
  "text": "Добро пожаловать Иван!",
  "html": "<p>Добро пожаловать <b>Иван</b>!</p>"
}
```

### Отправка письма по шаблону

```bash
POST /api/v1/send/template
```

Запрос:
```json
{
  "template_id": "abc123",
  "from": "noreply@example.com",
  "to": ["user@example.com"],
  "cc": [],
  "bcc": [],
  "data": {
    "Name": "Иван",
    "OrderID": "12345"
  },
  "headers": {
    "X-Campaign": "welcome"
  }
}
```

Можно использовать имя шаблона вместо ID:
```json
{
  "template_name": "welcome",
  ...
}
```

## Примеры

### Подтверждение заказа

```json
{
  "name": "order-confirmation",
  "subject": "Заказ #{{.OrderID}} подтверждён",
  "text": "Привет {{.Name}},\n\nВаш заказ #{{.OrderID}} подтверждён.\n\nИтого: {{.Amount}} {{.Currency}}\n\nТовары:\n{{range .Items}}- {{.Name}} x{{.Qty}}: {{.Price}}\n{{end}}\n\nСпасибо!",
  "html": "<h1>Заказ подтверждён</h1><p>Привет {{.Name}},</p><p>Ваш заказ #{{.OrderID}} подтверждён.</p><p><strong>Итого: {{.Amount}} {{.Currency}}</strong></p><h2>Товары:</h2><ul>{{range .Items}}<li>{{.Name}} x{{.Qty}}: {{.Price}}</li>{{end}}</ul>"
}
```

### Сброс пароля

```json
{
  "name": "password-reset",
  "subject": "Сброс пароля",
  "text": "Привет {{.Name}},\n\nНажмите для сброса пароля:\n{{.ResetURL}}\n\nСсылка действительна {{.ExpiresIn}}.\n\nЕсли вы не запрашивали сброс, проигнорируйте это письмо.",
  "html": "<p>Привет {{.Name}},</p><p><a href=\"{{.ResetURL}}\">Нажмите здесь для сброса пароля</a></p><p>Ссылка действительна {{.ExpiresIn}}.</p>"
}
```

## Безопасность

- HTML шаблоны автоматически экранируют переменные для защиты от XSS
- Используйте `{{.Variable}}` для автоматического экранирования
- Никогда не используйте `{{. | safeHTML}}` с пользовательским вводом
