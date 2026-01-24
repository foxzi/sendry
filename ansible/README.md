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

## DKIM ключи

DKIM ключи генерируются автоматически. После развертывания будут показаны DNS-записи для добавления.

## Теги

```bash
# Только установка
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags install

# Только настройка
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags configure
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
