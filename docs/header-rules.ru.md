# Руководство по правилам заголовков

Sendry поддерживает правила манипуляции заголовками для изменения заголовков email перед отправкой. Это полезно для:

- Удаления заголовков, раскрывающих внутреннюю инфраструктуру
- Добавления пользовательских заголовков для отслеживания или compliance
- Стандартизации заголовков для всей исходящей почты

## Конфигурация

Правила заголовков настраиваются в секции `header_rules` конфигурационного файла. Дополнительная настройка не требуется.

```yaml
header_rules:
  # Глобальные правила (применяются ко всем доменам)
  global:
    - action: remove
      headers:
        - "X-Originating-IP"
        - "X-Mailer"
        - "User-Agent"
    - action: add
      header: "X-Processed-By"
      value: "Sendry"

  # Правила для конкретных доменов
  domains:
    example.com:
      - action: replace
        header: "X-Mailer"
        value: "Example Mail Server"
```

## Действия

### Remove (Удаление)

Удаляет указанные заголовки из письма.

```yaml
- action: remove
  headers:
    - "X-Originating-IP"
    - "X-Mailer"
    - "User-Agent"
```

Имена заголовков регистронезависимы. Удаляются все вхождения указанных заголовков.

### Replace (Замена)

Заменяет значение заголовка. Если заголовок не существует, он добавляется.

```yaml
- action: replace
  header: "X-Mailer"
  value: "MyMailServer/1.0"
```

Заменяется только первое вхождение, если есть несколько заголовков с одинаковым именем.

### Add (Добавление)

Добавляет новый заголовок. Всегда добавляет в конец, даже если заголовок уже существует.

```yaml
- action: add
  header: "X-Processed-By"
  value: "Sendry"
```

## Порядок применения правил

1. Сначала применяются глобальные правила
2. Затем применяются правила для конкретного домена
3. Правила применяются в порядке их появления в конфиге

## Типичные сценарии

### Защита приватности

Удаление заголовков, раскрывающих внутреннюю инфраструктуру:

```yaml
header_rules:
  global:
    - action: remove
      headers:
        - "X-Originating-IP"
        - "X-Mailer"
        - "User-Agent"
        - "X-MimeOLE"
```

### Заголовки для compliance

Добавление обязательных заголовков для compliance или отслеживания:

```yaml
header_rules:
  global:
    - action: add
      header: "X-Organization"
      value: "Example Corp"
    - action: add
      header: "X-Compliance-ID"
      value: "GDPR-2024"
```

### Брендирование по доменам

Разные заголовки для разных отправляющих доменов:

```yaml
header_rules:
  domains:
    marketing.example.com:
      - action: add
        header: "X-Campaign-Source"
        value: "marketing"
    support.example.com:
      - action: add
        header: "X-Department"
        value: "support"
```

### Замена стандартных заголовков

Замена автоматически генерируемых заголовков на пользовательские:

```yaml
header_rules:
  global:
    - action: replace
      header: "X-Mailer"
      value: "Sendry MTA"
```

## Примечания

- Сопоставление заголовков регистронезависимо (`X-Mailer` соответствует `x-mailer`, `X-MAILER`)
- Многострочные заголовки (с продолжением) обрабатываются корректно
- Правила не влияют на тело сообщения
- DKIM подпись происходит после применения правил заголовков
