# Sendry Web

Web interface for managing Sendry email servers.

## Features

- Template management with versioning and deployment
- Recipient list management with CSV/JSON import
- Campaign management with A/B testing support
- Send job management with pause/resume/cancel
- Multi-server support with load balancing strategies
- Server monitoring (queue, DLQ, domains, sandbox)
- OIDC authentication (Authentik, Keycloak, etc.)

## Installation

Sendry Web is included in the sendry package. After installation:

```bash
# Start the service
sudo systemctl enable sendry-web
sudo systemctl start sendry-web
```

## Configuration

Configuration file: `/etc/sendry/web.yaml`

### Basic Configuration

```yaml
server:
  listen_addr: ":8088"
  tls:
    enabled: false
    cert_file: "/etc/sendry/certs/web.crt"
    key_file: "/etc/sendry/certs/web.key"

database:
  path: "/var/lib/sendry-web/app.db"

logging:
  level: info    # debug, info, warn, error
  format: json   # json, text
```

### Authentication

Sendry Web supports two authentication methods:

#### Local Authentication

```yaml
auth:
  local_enabled: true
  session_secret: "your-secret-key-at-least-32-chars"
  session_ttl: 24h
```

Create local users via CLI:

```bash
sendry-web user create --email admin@example.com --password secretpassword
```

#### OIDC Authentication

```yaml
auth:
  local_enabled: false  # can be true for fallback
  session_secret: "your-secret-key-at-least-32-chars"
  session_ttl: 24h

  oidc:
    enabled: true
    provider: "Authentik"  # display name on login page
    client_id: "sendry-web"
    client_secret: "your-client-secret"
    issuer_url: "https://auth.example.com/application/o/sendry-web/"
    redirect_url: "https://panel.example.com/auth/callback"
    scopes:
      - openid
      - profile
      - email
    allowed_groups:  # optional, empty = allow all authenticated users
      - "sendry-admins"
```

### Sendry Servers

Configure connections to Sendry MTA servers:

```yaml
sendry:
  servers:
    - name: "mta-prod-1"
      base_url: "https://mta-1.example.com"
      api_key: "your-api-key"
      env: "prod"
    - name: "mta-stage"
      base_url: "https://mta-stage.example.com"
      api_key: "your-api-key"
      env: "stage"

  multi_send:
    strategy: weighted_round_robin  # round_robin, weighted_round_robin, domain_affinity
    weights:
      mta-prod-1: 3
      mta-stage: 1
    domain_affinity:
      "example.com": "mta-prod-1"
    failover:
      enabled: true
      max_retries: 2
```

## CLI Commands

### Server Management

```bash
# Start server
sendry-web serve -c /etc/sendry/web.yaml

# Validate configuration
sendry-web config validate -c /etc/sendry/web.yaml
```

### User Management

```bash
# Create user
sendry-web user create --email admin@example.com --password secret

# List users
sendry-web user list

# Delete user
sendry-web user delete admin@example.com

# Reset password
sendry-web user reset-password admin@example.com
```

### Database Management

```bash
# Run migrations
sendry-web migrate

# Cleanup old data
sendry-web cleanup --days 30              # clean job items older than 30 days
sendry-web cleanup --days 90 --dry-run    # preview what would be deleted
```

## Web Interface

### Templates

- Create and edit email templates with HTML editor
- Version history with diff comparison
- Deploy templates to Sendry servers
- Preview with variable substitution

### Recipients

- Create recipient lists
- Import from CSV or JSON
- Per-recipient variables for personalization
- Status tracking (active, unsubscribed, bounced)

### Campaigns

- Create campaigns with sender settings
- Campaign-level variables
- A/B testing with multiple variants
- Weight-based traffic distribution

### Jobs

- Create send jobs from campaigns
- Schedule for future delivery
- Real-time progress monitoring
- Pause, resume, cancel operations
- Retry failed items

### Monitoring

- Dashboard with server status overview
- Queue and DLQ management
- Domain configuration view
- Sandbox message inspection

## Variable Substitution

Variables are merged with priority (highest to lowest):
1. Recipient variables
2. Campaign variables
3. Global variables

Use `{{variable_name}}` syntax in templates:

```html
<p>Hello, {{name}}!</p>
<p>Your order #{{order_id}} has been shipped.</p>
<p>Contact us at {{support_email}}</p>
```

## Security

- Session-based authentication with configurable TTL
- CSRF protection for state parameter in OIDC flow
- Secure cookie settings (HttpOnly, Secure, SameSite)
- Group-based access control with OIDC
- API key encryption in database (planned)

## Troubleshooting

### OIDC Login Issues

1. Verify `issuer_url` is accessible from server
2. Check `redirect_url` matches OIDC provider configuration
3. Ensure `client_id` and `client_secret` are correct
4. Check logs for detailed error messages

### Database Issues

```bash
# Check database file permissions
ls -la /var/lib/sendry-web/app.db

# Run migrations manually
sendry-web migrate -c /etc/sendry/web.yaml
```

### Connection to Sendry Servers

1. Verify `base_url` is accessible
2. Check API key is valid
3. Ensure TLS certificates are trusted
