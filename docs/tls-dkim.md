# TLS and DKIM Configuration

## TLS (Transport Layer Security)

Sendry supports TLS for secure email communication:

- **STARTTLS** on ports 25 and 587 - upgrade connection to TLS
- **SMTPS** on port 465 - implicit TLS from connection start

### Manual Certificates

```yaml
smtp:
  tls:
    cert_file: "/etc/sendry/certs/server.crt"
    key_file: "/etc/sendry/certs/server.key"
```

### Let's Encrypt (ACME)

Automatic certificate management with Let's Encrypt:

```yaml
smtp:
  tls:
    acme:
      enabled: true
      email: "admin@example.com"
      domains:
        - "mail.example.com"
      cache_dir: "/var/lib/sendry/certs"
      on_demand: true  # Recommended: port 80 not always open
```

Requirements for ACME:
- Port 80 must be accessible for HTTP-01 challenge (during renewal)
- DNS must resolve to the server

### ACME Modes

#### On-Demand Mode (Recommended)

With `on_demand: true`, port 80 is **not** opened automatically. Use `sendry tls renew` to obtain/renew certificates:

```bash
# Obtain or renew certificates
sendry tls renew -c /etc/sendry/config.yaml

# Force renewal even if certificates are valid
sendry tls renew -c /etc/sendry/config.yaml --force

# Check certificate status
sendry tls status -c /etc/sendry/config.yaml
```

Setup automatic renewal with cron or systemd timer:

```bash
# Cron (weekly renewal check)
0 3 * * 0 /usr/bin/sendry tls renew -c /etc/sendry/config.yaml
```

Or create systemd timer `/etc/systemd/system/sendry-tls-renew.timer`:
```ini
[Unit]
Description=Sendry TLS certificate renewal

[Timer]
OnCalendar=weekly
Persistent=true

[Install]
WantedBy=timers.target
```

And `/etc/systemd/system/sendry-tls-renew.service`:
```ini
[Unit]
Description=Sendry TLS certificate renewal

[Service]
Type=oneshot
ExecStart=/usr/bin/sendry tls renew -c /etc/sendry/config.yaml
```

Enable:
```bash
systemctl enable --now sendry-tls-renew.timer
```

#### Always-On Mode

With `on_demand: false` (default), port 80 is always open for automatic certificate renewal:

1. On startup, Sendry starts HTTP server on port 80 for ACME challenges
2. Certificates are fetched/validated for all configured domains
3. Certificates are cached in `cache_dir`
4. Automatic renewal 30 days before expiration

Startup logs will show certificate status:
```
level=INFO msg="obtained new certificate" domain=mail.example.com expires=2025-04-24 days_left=90
level=INFO msg="certificate valid" domain=mail.example.com expires=2025-04-24 days_left=85
```

**Certificate files:**
```bash
ls /var/lib/sendry/certs/
# acme_account+key        - ACME account key
# mail.example.com+rsa    - certificate and private key
```

### HTTPS for API

When TLS is configured (ACME or manual), the API server automatically uses HTTPS:

```
level=INFO msg="starting HTTPS API server" addr=:8080
```

Access API via HTTPS:
```bash
curl -k https://mail.example.com:8080/health
```

### Testing TLS

```bash
# Test STARTTLS on port 25
openssl s_client -starttls smtp -connect localhost:25

# Test STARTTLS on port 587
openssl s_client -starttls smtp -connect localhost:587

# Test SMTPS on port 465
openssl s_client -connect localhost:465

# Test HTTPS API
openssl s_client -connect localhost:8080
```

## DKIM (DomainKeys Identified Mail)

DKIM signs outgoing emails for authentication.

### Generate DKIM Key

```bash
sendry dkim generate --domain example.com --selector sendry --out /etc/sendry/dkim/
```

Output:
```
DKIM key generated successfully

Private key saved to: /etc/sendry/dkim/example.com.key

DNS Record:
  Name: sendry._domainkey.example.com
  Type: TXT
  Value: v=DKIM1; k=rsa; p=MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A...
```

### Show DKIM DNS Record

```bash
sendry dkim show --key /etc/sendry/dkim/example.com.key --domain example.com --selector sendry
```

### Configuration

```yaml
dkim:
  enabled: true
  selector: "sendry"
  domain: "example.com"
  key_file: "/etc/sendry/dkim/example.com.key"
```

### DNS Setup

Add the TXT record to your DNS:

```
sendry._domainkey.example.com. IN TXT "v=DKIM1; k=rsa; p=MIIBIjAN..."
```

### Verify DKIM Setup

Send a test email and check with:
- [mail-tester.com](https://www.mail-tester.com/)
- Gmail (check email headers)
- [dkimvalidator.com](https://dkimvalidator.com/)

## Full Configuration Example

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
