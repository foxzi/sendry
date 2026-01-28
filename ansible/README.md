# Sendry Ansible Role

Ansible role for deploying Sendry MTA on one or multiple servers.

## Requirements

- Ansible 2.9+
- Target: Ubuntu 20.04+, Debian 10+, RHEL/CentOS 8+

## Quick Start

1. Copy inventory example:
```bash
cp inventory/hosts.yml.example inventory/hosts.yml
```

2. Edit inventory with your servers and settings:
```bash
vim inventory/hosts.yml
```

3. Run playbook:
```bash
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml
```

## Installation Methods

### From GitHub Release (default)

```yaml
sendry_install_method: package  # or binary
sendry_version: latest          # or specific version like "0.3.3"
```

### From Local Package

```yaml
sendry_install_method: package
sendry_package_path: /path/to/sendry_0.3.3_amd64.deb
```

## Required Variables

```yaml
sendry_api_key: "your-secure-api-key"
sendry_smtp_auth_users:
  admin: "secure-password"
  api: "api-password"
```

## Domain Configuration

```yaml
sendry_domains:
  example.com:
    dkim_enabled: true
    dkim_selector: mail
    mode: production
    rate_limit:
      messages_per_hour: 1000
      messages_per_day: 10000

  staging.example.com:
    dkim_enabled: true
    dkim_selector: mail
    mode: redirect
    redirect_to:
      - qa@example.com
```

## DKIM Keys

DKIM keys are automatically generated if `sendry_dkim_generate: true` (default).

After deployment, DNS records will be displayed:
```
DKIM DNS Record for example.com:
Name: mail._domainkey.example.com
Type: TXT
Value: v=DKIM1; k=rsa; p=MIIBIjANBg...
```

## TLS Configuration

### Manual Certificates
```yaml
sendry_tls_cert_file: /etc/sendry/certs/cert.pem
sendry_tls_key_file: /etc/sendry/certs/key.pem
```

### Let's Encrypt (ACME)
```yaml
sendry_acme_enabled: true
sendry_acme_email: admin@example.com
sendry_acme_domains:
  - mail.example.com
# On-demand mode: port 80 is not opened permanently
# Use 'sendry tls renew' to obtain/renew certificates via cron
sendry_acme_on_demand: true  # default: true
```

## Recipient Domain Rate Limiting

Limit how many emails can be sent TO specific mail providers (gmail.com, mail.ru, etc.):

```yaml
# Default limits for all recipient domains
sendry_rate_limit_default_recipient_domain:
  messages_per_hour: 5000
  messages_per_day: 50000

# Override for specific domains
sendry_rate_limit_recipient_domains:
  gmail.com:
    messages_per_hour: 1000
    messages_per_day: 10000
  mail.ru:
    messages_per_hour: 1000
    messages_per_day: 10000
```

## IP Filtering

Restrict access to SMTP and API ports by IP address or CIDR:

```yaml
# SMTP: allow only specific IPs to connect
sendry_smtp_allowed_ips:
  - "10.0.0.0/8"
  - "192.168.1.0/24"
  - "203.0.113.50"

# API: allow only specific IPs (excludes /health endpoint)
sendry_api_allowed_ips:
  - "10.0.0.0/8"
  - "192.168.1.0/24"
```

## Sendry Web Panel

Install web panel with Caddy reverse proxy (automatic HTTPS):

```yaml
sendry_web_enabled: true
sendry_web_session_secret: "your-secret-at-least-32-characters-long"

# Caddy for automatic HTTPS
sendry_caddy_enabled: true
sendry_caddy_domain: panel.example.com
sendry_caddy_email: admin@example.com

# Optional: connect to multiple MTA servers
sendry_web_servers:
  - name: "mta-1"
    base_url: "http://localhost:8080"
    api_key: "{{ sendry_api_key }}"
    env: "prod"
  - name: "mta-2"
    base_url: "http://192.168.1.11:8080"
    api_key: "other-api-key"
    env: "prod"

# Optional: OIDC authentication
sendry_web_oidc_enabled: true
sendry_web_oidc_provider: authentik
sendry_web_oidc_client_id: "sendry-web"
sendry_web_oidc_client_secret: "{{ vault_oidc_secret }}"
sendry_web_oidc_issuer_url: "https://auth.example.com/application/o/sendry-web/"
sendry_web_oidc_redirect_url: "https://panel.example.com/auth/callback"
sendry_web_oidc_allowed_groups:
  - "sendry-admins"
```

### Web Panel Only (separate server)

Install only the web panel without MTA:

```yaml
# Disable MTA installation
sendry_mta_enabled: false

# Enable web panel
sendry_web_enabled: true
sendry_web_session_secret: "your-secret-at-least-32-characters-long"

# Required: specify MTA servers to connect to
sendry_web_servers:
  - name: "mta-1"
    base_url: "http://192.168.1.10:8080"
    api_key: "mta-1-api-key"
    env: "prod"
  - name: "mta-2"
    base_url: "http://192.168.1.11:8080"
    api_key: "mta-2-api-key"
    env: "prod"

# Caddy for HTTPS
sendry_caddy_enabled: true
sendry_caddy_domain: panel.example.com
sendry_caddy_email: admin@example.com
```

## API Load Balancer

Load balance API requests across multiple MTA servers with IP whitelist:

```yaml
sendry_caddy_api_enabled: true
sendry_caddy_api_domain: api.example.com
sendry_caddy_api_backends:
  - "192.168.1.10:8080"
  - "192.168.1.11:8080"
  - "192.168.1.12:8080"
sendry_caddy_api_lb_policy: round_robin  # round_robin, least_conn, ip_hash, random

# Health checks
sendry_caddy_api_health_check: true
sendry_caddy_api_health_uri: /health
sendry_caddy_api_health_interval: 10s

# IP whitelist (empty = allow all)
sendry_caddy_api_allowed_ips:
  - "10.0.0.0/8"
  - "192.168.0.0/16"
  - "203.0.113.50"

# Optional: restrict to specific paths only
sendry_caddy_api_allowed_paths:
  - "/api/v1/send"
  - "/api/v1/templates/*"
```

## Tags

Run specific parts of the playbook:

```bash
# Only install
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags install

# Only configure
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags configure

# Only DKIM
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags dkim

# Only web panel
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags web

# Only Caddy
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags caddy
```

## Example Inventory

```yaml
all:
  children:
    sendry_servers:
      hosts:
        mail1.example.com:
          ansible_host: 192.168.1.10
          sendry_hostname: mail1.example.com
          sendry_api_key: "change-me-api-key"
          sendry_smtp_auth_users:
            admin: "change-me-password"
          sendry_domains:
            example.com:
              dkim_enabled: true
              dkim_selector: mail

      vars:
        ansible_user: root
        sendry_install_method: package
        sendry_version: latest
```

## Multi-Server with Shared DKIM Keys

For multiple servers sending mail for the same domain, you must use shared DKIM keys.

### Step 1: Generate DKIM key once

```bash
sendry dkim generate --domain example.com --selector mail --out ./dkim-keys/
```

### Step 2: Store keys in Ansible Vault

```bash
# Create vault file
ansible-vault create group_vars/sendry_cluster/vault.yml
```

Add keys to vault:
```yaml
vault_dkim_keys:
  example.com:
    private: |
      -----BEGIN PRIVATE KEY-----
      ... content of example.com.key ...
      -----END PRIVATE KEY-----
    public: |
      -----BEGIN PUBLIC KEY-----
      ... content of example.com.pub ...
      -----END PUBLIC KEY-----
```

### Step 3: Reference vault in inventory

```yaml
sendry_cluster:
  hosts:
    mail1.example.com:
    mail2.example.com:
    mail3.example.com:
  vars:
    sendry_dkim_keys: "{{ vault_dkim_keys }}"
```

### Step 4: Run playbook with vault

```bash
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --ask-vault-pass
```

## Testing with Molecule

```bash
cd roles/sendry
molecule test
```

See [Ansible docs](../docs/ansible.md) for details.

## All Variables

See `roles/sendry/defaults/main.yml` for all available variables with defaults.

---

# Sendry Ansible Role (RU)

Ansible роль для развертывания Sendry MTA на одном или нескольких серверах.

## Требования

- Ansible 2.9+
- Целевая ОС: Ubuntu 20.04+, Debian 10+, RHEL/CentOS 8+

## Быстрый старт

1. Скопируйте пример inventory:
```bash
cp inventory/hosts.yml.example inventory/hosts.yml
```

2. Отредактируйте inventory с вашими серверами:
```bash
vim inventory/hosts.yml
```

3. Запустите playbook:
```bash
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml
```

## Обязательные переменные

```yaml
sendry_api_key: "ваш-секретный-api-ключ"
sendry_smtp_auth_users:
  admin: "надежный-пароль"
```

## TLS/ACME

```yaml
sendry_acme_enabled: true
sendry_acme_email: admin@example.com
sendry_acme_domains:
  - mail.example.com
# On-demand режим: порт 80 не открывается постоянно
# Используйте 'sendry tls renew' для получения/обновления сертификатов через cron
sendry_acme_on_demand: true  # по умолчанию: true
```

## Лимиты по доменам получателей

Ограничение количества писем, которые можно отправить НА определённые почтовые провайдеры:

```yaml
# Лимиты по умолчанию для всех доменов получателей
sendry_rate_limit_default_recipient_domain:
  messages_per_hour: 5000
  messages_per_day: 50000

# Переопределение для конкретных доменов
sendry_rate_limit_recipient_domains:
  gmail.com:
    messages_per_hour: 1000
    messages_per_day: 10000
  mail.ru:
    messages_per_hour: 1000
    messages_per_day: 10000
```

## Фильтрация по IP

Ограничение доступа к SMTP и API портам по IP адресу или CIDR:

```yaml
# SMTP: разрешить подключение только с определённых IP
sendry_smtp_allowed_ips:
  - "10.0.0.0/8"
  - "192.168.1.0/24"
  - "203.0.113.50"

# API: разрешить доступ только с определённых IP (кроме /health)
sendry_api_allowed_ips:
  - "10.0.0.0/8"
  - "192.168.1.0/24"
```

## DKIM ключи

DKIM ключи генерируются автоматически. После развертывания будут показаны DNS-записи для добавления.

## Sendry Web панель

Установка web панели с Caddy reverse proxy (автоматический HTTPS):

```yaml
sendry_web_enabled: true
sendry_web_session_secret: "ваш-секрет-минимум-32-символа"

# Caddy для автоматического HTTPS
sendry_caddy_enabled: true
sendry_caddy_domain: panel.example.com
sendry_caddy_email: admin@example.com

# Опционально: подключение к нескольким MTA серверам
sendry_web_servers:
  - name: "mta-1"
    base_url: "http://localhost:8080"
    api_key: "{{ sendry_api_key }}"
    env: "prod"

# Опционально: OIDC аутентификация
sendry_web_oidc_enabled: true
sendry_web_oidc_provider: authentik
sendry_web_oidc_client_id: "sendry-web"
sendry_web_oidc_client_secret: "{{ vault_oidc_secret }}"
sendry_web_oidc_issuer_url: "https://auth.example.com/application/o/sendry-web/"
sendry_web_oidc_redirect_url: "https://panel.example.com/auth/callback"
```

### Только Web панель (отдельный сервер)

Установка только web панели без MTA:

```yaml
# Отключить установку MTA
sendry_mta_enabled: false

# Включить web панель
sendry_web_enabled: true
sendry_web_session_secret: "ваш-секрет-минимум-32-символа"

# Обязательно: указать MTA серверы для подключения
sendry_web_servers:
  - name: "mta-1"
    base_url: "http://192.168.1.10:8080"
    api_key: "mta-1-api-key"
    env: "prod"
  - name: "mta-2"
    base_url: "http://192.168.1.11:8080"
    api_key: "mta-2-api-key"
    env: "prod"

# Caddy для HTTPS
sendry_caddy_enabled: true
sendry_caddy_domain: panel.example.com
sendry_caddy_email: admin@example.com
```

## API балансировщик

Балансировка API запросов между несколькими MTA серверами с ограничением по IP:

```yaml
sendry_caddy_api_enabled: true
sendry_caddy_api_domain: api.example.com
sendry_caddy_api_backends:
  - "192.168.1.10:8080"
  - "192.168.1.11:8080"
  - "192.168.1.12:8080"
sendry_caddy_api_lb_policy: round_robin  # round_robin, least_conn, ip_hash, random

# Проверка здоровья
sendry_caddy_api_health_check: true
sendry_caddy_api_health_uri: /health
sendry_caddy_api_health_interval: 10s

# Белый список IP (пусто = разрешить всем)
sendry_caddy_api_allowed_ips:
  - "10.0.0.0/8"
  - "192.168.0.0/16"
  - "203.0.113.50"

# Опционально: ограничить только определённые пути
sendry_caddy_api_allowed_paths:
  - "/api/v1/send"
  - "/api/v1/templates/*"
```

## Теги

```bash
# Только установка
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags install

# Только настройка
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags configure

# Только web панель
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags web

# Только Caddy
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags caddy
```

## Несколько серверов с общими DKIM ключами

При деплое нескольких серверов для одного домена необходимо использовать общие DKIM ключи.

### Шаг 1: Сгенерировать DKIM ключ один раз

```bash
sendry dkim generate --domain example.com --selector mail --out ./dkim-keys/
```

### Шаг 2: Сохранить ключи в Ansible Vault

```bash
ansible-vault create group_vars/sendry_cluster/vault.yml
```

Добавить ключи:
```yaml
vault_dkim_keys:
  example.com:
    private: |
      -----BEGIN PRIVATE KEY-----
      ... содержимое example.com.key ...
      -----END PRIVATE KEY-----
    public: |
      -----BEGIN PUBLIC KEY-----
      ... содержимое example.com.pub ...
      -----END PUBLIC KEY-----
```

### Шаг 3: Указать ключи в inventory

```yaml
sendry_cluster:
  hosts:
    mail1.example.com:
    mail2.example.com:
  vars:
    sendry_dkim_keys: "{{ vault_dkim_keys }}"
```

### Шаг 4: Запустить с vault

```bash
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --ask-vault-pass
```

## Тестирование с Molecule

```bash
cd roles/sendry
molecule test
```

См. [документацию Ansible](../docs/ansible.ru.md) для деталей.
