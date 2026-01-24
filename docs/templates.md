# Email Templates Guide

Sendry supports email templates with Go template syntax for dynamic content.

## Configuration

Templates work out of the box with no additional configuration. They use the same BoltDB storage as the message queue:

```yaml
storage:
  path: "/var/lib/sendry/queue.db"

api:
  listen_addr: ":8080"
  api_key: "your-api-key"
```

Templates are stored in a separate bucket within the database file.

## Features

- Go templates (`text/template` for text, `html/template` for HTML)
- Automatic XSS protection in HTML templates
- Template versioning
- Preview with test data
- CLI and API management

## Template Structure

```json
{
  "name": "welcome",
  "description": "Welcome email for new users",
  "subject": "Welcome {{.Name}}!",
  "text": "Hello {{.Name}},\n\nWelcome to our service!",
  "html": "<p>Hello <b>{{.Name}}</b>,</p><p>Welcome to our service!</p>",
  "variables": [
    {
      "name": "Name",
      "type": "string",
      "required": true,
      "description": "User's name"
    }
  ]
}
```

## Template Syntax

Sendry uses Go template syntax. Common patterns:

### Variables

```
{{.Name}}           - Simple variable
{{.User.Email}}     - Nested field
```

### Conditionals

```
{{if .Premium}}
  Premium content
{{else}}
  Standard content
{{end}}
```

### Loops

```
{{range .Items}}
  - {{.Name}}: {{.Price}}
{{end}}
```

### Default Values

```
{{if .Name}}{{.Name}}{{else}}Customer{{end}}
```

## CLI Commands

### List Templates

```bash
sendry template list -c config.yaml
```

### Create Template

```bash
# From command line
sendry template create -c config.yaml \
  --name welcome \
  --subject "Hello {{.Name}}" \
  --text welcome.txt \
  --html welcome.html

# Minimal (text only)
sendry template create -c config.yaml \
  --name simple \
  --subject "Notification" \
  --text message.txt
```

### Show Template

```bash
sendry template show -c config.yaml welcome
sendry template show -c config.yaml {template-id}
```

### Preview Template

```bash
sendry template preview -c config.yaml welcome \
  --data '{"Name":"John","OrderID":"12345"}'
```

### Delete Template

```bash
sendry template delete -c config.yaml {template-id}
```

### Export Template

```bash
sendry template export -c config.yaml welcome --output ./templates/
```

This creates:
- `welcome.json` - metadata and subject
- `welcome.html` - HTML template (if exists)
- `welcome.txt` - text template (if exists)

### Import Template

```bash
sendry template import -c config.yaml \
  --name welcome \
  --subject "Hello {{.Name}}" \
  --html ./templates/welcome.html \
  --text ./templates/welcome.txt
```

## API Endpoints

### List Templates

```bash
GET /api/v1/templates
```

Response:
```json
{
  "templates": [
    {
      "id": "abc123",
      "name": "welcome",
      "subject": "Hello {{.Name}}",
      "version": 1,
      "created_at": "2024-01-15T10:00:00Z",
      "updated_at": "2024-01-15T10:00:00Z"
    }
  ],
  "total": 1
}
```

### Create Template

```bash
POST /api/v1/templates
```

Request:
```json
{
  "name": "welcome",
  "description": "Welcome email",
  "subject": "Hello {{.Name}}",
  "text": "Welcome {{.Name}}!",
  "html": "<p>Welcome <b>{{.Name}}</b>!</p>",
  "variables": [
    {"name": "Name", "required": true}
  ]
}
```

### Get Template

```bash
GET /api/v1/templates/{id}
```

### Update Template

```bash
PUT /api/v1/templates/{id}
```

Request:
```json
{
  "subject": "Updated: Hello {{.Name}}",
  "text": "Updated welcome message"
}
```

Version is incremented automatically on each update.

### Delete Template

```bash
DELETE /api/v1/templates/{id}
```

### Preview Template

```bash
POST /api/v1/templates/{id}/preview
```

Request:
```json
{
  "data": {
    "Name": "John",
    "OrderID": "12345"
  }
}
```

Response:
```json
{
  "subject": "Hello John",
  "text": "Welcome John!",
  "html": "<p>Welcome <b>John</b>!</p>"
}
```

### Send Email by Template

```bash
POST /api/v1/send/template
```

Request:
```json
{
  "template_id": "abc123",
  "from": "noreply@example.com",
  "to": ["user@example.com"],
  "cc": [],
  "bcc": [],
  "data": {
    "Name": "John",
    "OrderID": "12345"
  },
  "headers": {
    "X-Campaign": "welcome"
  }
}
```

You can also use template name instead of ID:
```json
{
  "template_name": "welcome",
  ...
}
```

## Examples

### Order Confirmation

```json
{
  "name": "order-confirmation",
  "subject": "Order #{{.OrderID}} Confirmed",
  "text": "Hi {{.Name}},\n\nYour order #{{.OrderID}} has been confirmed.\n\nTotal: {{.Amount}} {{.Currency}}\n\nItems:\n{{range .Items}}- {{.Name}} x{{.Qty}}: {{.Price}}\n{{end}}\n\nThank you!",
  "html": "<h1>Order Confirmed</h1><p>Hi {{.Name}},</p><p>Your order #{{.OrderID}} has been confirmed.</p><p><strong>Total: {{.Amount}} {{.Currency}}</strong></p><h2>Items:</h2><ul>{{range .Items}}<li>{{.Name}} x{{.Qty}}: {{.Price}}</li>{{end}}</ul>"
}
```

### Password Reset

```json
{
  "name": "password-reset",
  "subject": "Reset Your Password",
  "text": "Hi {{.Name}},\n\nClick to reset your password:\n{{.ResetURL}}\n\nThis link expires in {{.ExpiresIn}}.\n\nIf you didn't request this, ignore this email.",
  "html": "<p>Hi {{.Name}},</p><p><a href=\"{{.ResetURL}}\">Click here to reset your password</a></p><p>This link expires in {{.ExpiresIn}}.</p>"
}
```

## Security

- HTML templates automatically escape variables to prevent XSS
- Use `{{.Variable}}` for auto-escaped output
- Never use `{{. | safeHTML}}` with user input
