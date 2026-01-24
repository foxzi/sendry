# Ansible Deployment

Ansible role for automated Sendry deployment on one or multiple servers.

[Русская версия](ansible.ru.md)

## Requirements

- Ansible 2.9+
- Target OS: Ubuntu 20.04+, Debian 10+, RHEL/CentOS 8+

## Directory Structure

```
ansible/
├── inventory/
│   └── hosts.yml.example
├── playbooks/
│   └── sendry.yml
├── roles/
│   └── sendry/
│       ├── defaults/main.yml
│       ├── handlers/main.yml
│       ├── tasks/
│       │   ├── main.yml
│       │   ├── install.yml
│       │   ├── configure.yml
│       │   ├── dkim.yml
│       │   └── firewall.yml
│       └── templates/
│           ├── config.yaml.j2
│           └── sendry.service.j2
└── README.md
```

## Quick Start

```bash
cd ansible

# Copy and edit inventory
cp inventory/hosts.yml.example inventory/hosts.yml
vim inventory/hosts.yml

# Run playbook
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml
```

## Installation Methods

### From GitHub Release (recommended)

Downloads the latest release package from GitHub:

```yaml
sendry_install_method: package  # DEB/RPM based on OS
sendry_version: latest
```

Or specific version:

```yaml
sendry_install_method: binary
sendry_version: "0.3.3"
```

### From Binary

Downloads the binary directly:

```yaml
sendry_install_method: binary
sendry_version: latest
```

## Required Variables

These must be set in your inventory:

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
      recipients_per_message: 100

  staging.example.com:
    dkim_enabled: true
    dkim_selector: mail
    mode: redirect
    redirect_to:
      - qa@example.com

  test.example.com:
    mode: sandbox
```

## DKIM Keys

Keys are automatically generated when `sendry_dkim_generate: true` (default).

After deployment, DNS records are displayed:

```
DKIM DNS Record for example.com:
Name: mail._domainkey.example.com
Type: TXT
Value: v=DKIM1; k=rsa; p=MIIBIjANBg...
```

Add these records to your DNS provider.

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

Run specific parts:

```bash
# Install only
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags install

# Configure only
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags configure

# DKIM only
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags dkim

# Firewall only
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags firewall
```

## Multi-Server Deployment with Shared DKIM

When deploying multiple servers for the same domain, you must use shared DKIM keys.

### Generate DKIM key once

```bash
sendry dkim generate --domain example.com --selector mail --out ./dkim-keys/
```

### Store in Ansible Vault

```bash
ansible-vault create group_vars/sendry_cluster/vault.yml
```

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

### Inventory configuration

```yaml
all:
  children:
    sendry_servers:
      hosts:
        mail1.example.com:
          ansible_host: 192.168.1.10
          sendry_hostname: mail1.example.com
        mail2.example.com:
          ansible_host: 192.168.1.11
          sendry_hostname: mail2.example.com
        mail3.example.com:
          ansible_host: 192.168.1.12
          sendry_hostname: mail3.example.com

      vars:
        ansible_user: root
        sendry_api_key: "shared-api-key"
        sendry_smtp_auth_users:
          admin: "shared-password"
        sendry_domains:
          example.com:
            dkim_enabled: true
            dkim_selector: mail
        # Shared DKIM keys from vault
        sendry_dkim_keys: "{{ vault_dkim_keys }}"
```

### Run with vault

```bash
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --ask-vault-pass
```

## All Variables

See `ansible/roles/sendry/defaults/main.yml` for complete list.

---

# Развертывание через Ansible (RU)

Ansible роль для автоматического развертывания Sendry.

## Требования

- Ansible 2.9+
- Целевая ОС: Ubuntu 20.04+, Debian 10+, RHEL/CentOS 8+

## Быстрый старт

```bash
cd ansible

# Скопировать и отредактировать inventory
cp inventory/hosts.yml.example inventory/hosts.yml
vim inventory/hosts.yml

# Запустить playbook
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml
```

## Обязательные переменные

```yaml
sendry_api_key: "ваш-секретный-ключ"
sendry_smtp_auth_users:
  admin: "надежный-пароль"
```

## Настройка доменов

```yaml
sendry_domains:
  example.com:
    dkim_enabled: true
    dkim_selector: mail
    mode: production
    rate_limit:
      messages_per_hour: 1000
      messages_per_day: 10000
```

## DKIM ключи

Генерируются автоматически. DNS-записи выводятся после развертывания.

## Теги

```bash
# Только установка
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags install

# Только настройка
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags configure
```

## Развертывание на несколько серверов с общими DKIM

При деплое нескольких серверов для одного домена нужно использовать общие DKIM ключи.

### Сгенерировать ключ один раз

```bash
sendry dkim generate --domain example.com --selector mail --out ./dkim-keys/
```

### Сохранить в Ansible Vault

```bash
ansible-vault create group_vars/sendry_cluster/vault.yml
```

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

### Настройка inventory

```yaml
all:
  children:
    sendry_servers:
      hosts:
        mail1.example.com:
          ansible_host: 192.168.1.10
        mail2.example.com:
          ansible_host: 192.168.1.11
      vars:
        sendry_dkim_keys: "{{ vault_dkim_keys }}"
```

### Запуск с vault

```bash
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --ask-vault-pass
```
