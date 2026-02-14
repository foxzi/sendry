# HTTP API Reference

Sendry provides a comprehensive HTTP API for sending emails, managing queues, templates, domains, and more.

## Authentication

All API endpoints (except `/health`) require authentication via API key:

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" http://localhost:8080/api/v1/...
```

### API Key Management

API keys can be created and managed through the web interface at `/settings/api-keys`.

**Features:**
- **Domain Restrictions**: Limit API keys to send only from specific domains. If no domains are specified, the key can send from any configured domain.
- **Rate Limits**: Set per-minute and per-hour rate limits for each key.
- **Expiration**: Optionally set an expiration date for keys.
- **Activity Tracking**: View last used timestamp and total send count.

**Error Responses:**
| Code | Error | Description |
|------|-------|-------------|
| `DOMAIN_NOT_ALLOWED` | 403 | API key is not allowed to send from the specified domain |
| `UNAUTHORIZED` | 401 | Invalid or missing API key |
| `RATE_LIMITED` | 429 | Rate limit exceeded for this API key |

## Base URL

Default: `http://localhost:8080`

---

## Core Endpoints

### Health Check

Check server status. No authentication required.

```
GET /health
```

**Response:**
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

### Send Email

Queue an email for delivery.

```
POST /api/v1/send
```

**Request:**
```json
{
  "from": "sender@example.com",
  "to": ["recipient@example.com"],
  "subject": "Hello",
  "body": "Plain text content",
  "html": "<p>HTML content</p>",
  "headers": {
    "X-Custom-Header": "value"
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from` | string | Yes | Sender email address |
| `to` | array | Yes | Recipient email addresses |
| `subject` | string | Yes* | Email subject |
| `body` | string | Yes* | Plain text body |
| `html` | string | No | HTML body |
| `headers` | object | No | Custom email headers |

*At least one of `subject`, `body`, or `html` is required.

**Response (202 Accepted):**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending"
}
```

### Get Message Status

Get the delivery status of a message.

```
GET /api/v1/status/{id}
```

**Response:**
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

**Status values:**
| Status | Description |
|--------|-------------|
| `pending` | Waiting to be sent |
| `sending` | Currently being sent |
| `delivered` | Successfully delivered |
| `deferred` | Temporary failure, will retry |
| `failed` | Permanent failure |

### Get Queue Stats

Get queue statistics and list of messages.

```
GET /api/v1/queue
```

**Response:**
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

### Delete Message

Remove a message from the queue.

```
DELETE /api/v1/queue/{id}
```

**Response:** `204 No Content`

---

## Dead Letter Queue (DLQ)

Failed messages are moved to the DLQ for manual review.

### List DLQ Messages

```
GET /api/v1/dlq
```

**Response:**
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

### Get DLQ Message

```
GET /api/v1/dlq/{id}
```

**Response:** Same as message status response.

### Retry DLQ Message

Move a message back to the pending queue for retry.

```
POST /api/v1/dlq/{id}/retry
```

**Response:**
```json
{
  "status": "ok",
  "message": "Message moved to pending queue"
}
```

### Delete DLQ Message

```
DELETE /api/v1/dlq/{id}
```

**Response:** `204 No Content`

---

## Templates

Email templates with variable substitution.

### List Templates

```
GET /api/v1/templates
```

**Query Parameters:**
| Parameter | Description |
|-----------|-------------|
| `search` | Search by name |
| `limit` | Max results (default: 100) |
| `offset` | Skip N results |

**Response:**
```json
{
  "templates": [
    {
      "id": "...",
      "name": "welcome",
      "description": "Welcome email",
      "subject": "Welcome {{.Name}}!",
      "html": "<h1>Hello {{.Name}}</h1>",
      "text": "Hello {{.Name}}",
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

### Create Template

```
POST /api/v1/templates
```

**Request:**
```json
{
  "name": "welcome",
  "description": "Welcome email template",
  "subject": "Welcome {{.Name}}!",
  "html": "<h1>Hello {{.Name}}</h1>",
  "text": "Hello {{.Name}}",
  "variables": [
    {"name": "Name", "required": true, "default": "User"}
  ]
}
```

**Response (201 Created):** Template object.

### Get Template

```
GET /api/v1/templates/{id}
```

Note: `{id}` can be the template ID or name.

**Response:** Template object.

### Update Template

```
PUT /api/v1/templates/{id}
```

**Request:** Same as create (all fields optional).

**Response:** Updated template object.

### Delete Template

```
DELETE /api/v1/templates/{id}
```

**Response:** `204 No Content`

### Preview Template

Render a template with sample data.

```
POST /api/v1/templates/{id}/preview
```

**Request:**
```json
{
  "data": {
    "Name": "John"
  }
}
```

**Response:**
```json
{
  "subject": "Welcome John!",
  "html": "<h1>Hello John</h1>",
  "text": "Hello John"
}
```

### Send via Template

Send an email using a template.

```
POST /api/v1/send/template
```

**Request:**
```json
{
  "template_id": "...",
  "template_name": "welcome",
  "from": "noreply@example.com",
  "to": ["user@example.com"],
  "cc": [],
  "bcc": [],
  "data": {
    "Name": "John"
  },
  "headers": {}
}
```

Note: Provide either `template_id` or `template_name`.

**Response (202 Accepted):**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending"
}
```

---

## Sandbox

Sandbox mode captures emails locally for testing. Available when domains are configured with `mode: sandbox` or `mode: redirect`.

### List Sandbox Messages

```
GET /api/v1/sandbox/messages
```

**Query Parameters:**
| Parameter | Description |
|-----------|-------------|
| `domain` | Filter by domain |
| `mode` | Filter by mode (sandbox/redirect/bcc) |
| `from` | Filter by sender |
| `limit` | Max results (default: 100) |
| `offset` | Skip N results |

**Response:**
```json
{
  "messages": [
    {
      "id": "...",
      "from": "sender@example.com",
      "to": ["test@sandbox.example.com"],
      "original_to": ["real@example.com"],
      "subject": "Test Email",
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

### Get Sandbox Message

```
GET /api/v1/sandbox/messages/{id}
```

**Response:**
```json
{
  "id": "...",
  "from": "sender@example.com",
  "to": ["test@sandbox.example.com"],
  "subject": "Test Email",
  "domain": "sandbox.example.com",
  "mode": "sandbox",
  "captured_at": "2024-01-15T10:30:00Z",
  "headers": {
    "From": "sender@example.com",
    "To": "test@sandbox.example.com",
    "Subject": "Test Email"
  },
  "body": "Plain text content",
  "html": "<p>HTML content</p>",
  "size": 1234
}
```

### Get Raw Email

Download the raw RFC 5322 email data.

```
GET /api/v1/sandbox/messages/{id}/raw
```

**Response:** `message/rfc822` content with `.eml` filename.

### Resend Message

Re-queue a sandbox message for actual delivery.

```
POST /api/v1/sandbox/messages/{id}/resend
```

**Response:**
```json
{
  "status": "queued",
  "message_id": "...-resend-20240115103000"
}
```

### Clear Sandbox Messages

Delete multiple sandbox messages.

```
DELETE /api/v1/sandbox/messages
```

**Query Parameters:**
| Parameter | Description |
|-----------|-------------|
| `domain` | Only clear messages for this domain |
| `older_than` | Clear messages older than duration (e.g., `24h`, `7d`) |

**Response:**
```json
{
  "cleared": 50
}
```

### Delete Sandbox Message

```
DELETE /api/v1/sandbox/messages/{id}
```

**Response:** `204 No Content`

### Sandbox Stats

```
GET /api/v1/sandbox/stats
```

**Response:**
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

## Domain Management

Manage domain configurations at runtime.

### List Domains

```
GET /api/v1/domains
```

**Response:**
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

### Create Domain

```
POST /api/v1/domains
```

**Request:**
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

**Response (201 Created):** Domain object.

### Get Domain

```
GET /api/v1/domains/{domain}
```

**Response:** Domain object.

### Update Domain

```
PUT /api/v1/domains/{domain}
```

**Request:** Same as create (all fields optional).

**Response:** Updated domain object.

### Delete Domain

```
DELETE /api/v1/domains/{domain}
```

Note: Cannot delete the main SMTP domain.

**Response:** `204 No Content`

---

## DKIM Management

### Generate DKIM Key

```
POST /api/v1/dkim/generate
```

**Request:**
```json
{
  "domain": "example.com",
  "selector": "default"
}
```

**Response (201 Created):**
```json
{
  "domain": "example.com",
  "selector": "default",
  "dns_name": "default._domainkey.example.com",
  "dns_record": "v=DKIM1; k=rsa; p=MIIBIjANBgkq...",
  "key_file": "/var/lib/sendry/dkim/example.com/default.key"
}
```

### Get DKIM Info

```
GET /api/v1/dkim/{domain}
```

**Response:**
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

### Verify DKIM Configuration

```
GET /api/v1/dkim/{domain}/verify?selector=default
```

**Response:**
```json
{
  "domain": "example.com",
  "selector": "default",
  "valid": true,
  "error": "",
  "dns_name": "default._domainkey.example.com"
}
```

### Delete DKIM Key

```
DELETE /api/v1/dkim/{domain}/{selector}
```

**Response:** `204 No Content`

---

## TLS Management

### List Certificates

```
GET /api/v1/tls/certificates
```

**Response:**
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

### Upload Certificate

```
POST /api/v1/tls/certificates
```

**Request:**
```json
{
  "domain": "mail.example.com",
  "certificate": "-----BEGIN CERTIFICATE-----\n...",
  "private_key": "-----BEGIN PRIVATE KEY-----\n..."
}
```

**Response (201 Created):** Certificate info object.

### Request Let's Encrypt Certificate

```
POST /api/v1/tls/letsencrypt/{domain}
```

Note: Domain must be in the ACME allowed domains list.

**Response (202 Accepted):**
```json
{
  "status": "pending",
  "message": "Certificate will be obtained automatically on first TLS connection",
  "domain": "mail.example.com"
}
```

---

## Rate Limits

### Get Rate Limit Configuration

```
GET /api/v1/ratelimits
```

**Response:**
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

### Get Rate Limit Stats

```
GET /api/v1/ratelimits/{level}/{key}
```

**Levels:** `global`, `domain`, `sender`, `ip`, `api_key`

**Response:**
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

### Update Domain Rate Limits

```
PUT /api/v1/ratelimits/{domain}
```

**Request:**
```json
{
  "messages_per_hour": 2000,
  "messages_per_day": 20000,
  "recipients_per_message": 100
}
```

**Response:** Updated rate limit object.

---

## DNS Checking

Check DNS records for domains and IP reputation.

### Check Domain DNS Records

```
GET /api/v1/dns/check/{domain}
```

Check MX, SPF, DKIM, DMARC, and MTA-STS records for a domain.

**Query Parameters:**

| Parameter | Default | Description |
|-----------|---------|-------------|
| `mx` | false | Check only MX records |
| `spf` | false | Check only SPF record |
| `dkim` | false | Check only DKIM record |
| `dmarc` | false | Check only DMARC record |
| `mta_sts` | false | Check only MTA-STS record |
| `selector` | sendry | DKIM selector to check |

If no specific check is requested, all records are checked.

**Response:**
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

**Status values:** `ok`, `warning`, `error`, `not_found`

### Check IP Against DNSBL

```
GET /api/v1/ip/check/{ip}
```

Check an IPv4 address against DNS-based blackhole lists (DNSBL).

**Response:**
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

### List DNSBL Services

```
GET /api/v1/ip/dnsbls
```

List all DNS blacklist services that are checked.

**Response:**
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

## Error Responses

All endpoints return errors in the following format:

```json
{
  "error": "Error message description"
}
```

**Common HTTP Status Codes:**

| Code | Description |
|------|-------------|
| 200 | Success |
| 201 | Created |
| 202 | Accepted (queued for processing) |
| 204 | No Content (successful deletion) |
| 400 | Bad Request (invalid input) |
| 401 | Unauthorized (missing/invalid API key) |
| 403 | Forbidden |
| 404 | Not Found |
| 409 | Conflict (e.g., duplicate name) |
| 429 | Too Many Requests (rate limited) |
| 500 | Internal Server Error |
| 503 | Service Unavailable |
