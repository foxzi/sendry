# Развертывание через Ansible

Ansible роль для автоматического развертывания Sendry на одном или нескольких серверах.

[English version](ansible.md)

## Требования

- Ansible 2.9+
- Целевая ОС: Ubuntu 20.04+, Debian 10+, RHEL/CentOS 8+

## Структура каталогов

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

## Быстрый старт

```bash
cd ansible

# Скопировать и отредактировать inventory
cp inventory/hosts.yml.example inventory/hosts.yml
vim inventory/hosts.yml

# Запустить playbook
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml
```

## Способы установки

### Из GitHub Release (рекомендуется)

Скачивает последний релиз с GitHub:

```yaml
sendry_install_method: package  # DEB/RPM в зависимости от ОС
sendry_version: latest
```

Или конкретная версия:

```yaml
sendry_install_method: binary
sendry_version: "0.3.3"
```

### Из бинарного файла

Скачивает бинарник напрямую:

```yaml
sendry_install_method: binary
sendry_version: latest
```

## Обязательные переменные

Должны быть указаны в inventory:

```yaml
sendry_api_key: "ваш-секретный-api-ключ"
sendry_smtp_auth_users:
  admin: "надежный-пароль"
  api: "api-пароль"
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

## DKIM ключи

### Один сервер

Ключи генерируются автоматически при `sendry_dkim_generate: true` (по умолчанию).

После развертывания будут показаны DNS-записи:

```
DKIM DNS Record for example.com:
Name: mail._domainkey.example.com
Type: TXT
Value: v=DKIM1; k=rsa; p=MIIBIjANBg...
```

Добавьте эти записи в DNS вашего домена.

### Несколько серверов (общие ключи)

При развертывании нескольких серверов для одного домена **обязательно** используйте общие DKIM ключи.

#### Шаг 1: Сгенерировать ключ один раз

```bash
sendry dkim generate --domain example.com --selector mail --out ./dkim-keys/
```

#### Шаг 2: Сохранить ключи в Ansible Vault

```bash
ansible-vault create group_vars/sendry_cluster/vault.yml
```

Содержимое vault:

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

#### Шаг 3: Указать ключи в inventory

```yaml
sendry_cluster:
  hosts:
    mail1.example.com:
    mail2.example.com:
    mail3.example.com:
  vars:
    sendry_dkim_keys: "{{ vault_dkim_keys }}"
```

#### Шаг 4: Запустить с vault

```bash
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --ask-vault-pass
```

## Настройка TLS

### Ручные сертификаты

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

## Теги

Запуск отдельных частей:

```bash
# Только установка
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags install

# Только настройка
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags configure

# Только DKIM
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags dkim

# Только firewall
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml --tags firewall
```

## Пример inventory для кластера

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
        sendry_install_method: package
        sendry_version: latest

        sendry_api_key: "общий-api-ключ"
        sendry_smtp_auth_users:
          admin: "общий-пароль"

        sendry_domains:
          example.com:
            dkim_enabled: true
            dkim_selector: mail
            mode: production

        # Общие DKIM ключи из vault
        sendry_dkim_keys: "{{ vault_dkim_keys }}"
```

## Все переменные

Полный список переменных с значениями по умолчанию см. в `ansible/roles/sendry/defaults/main.yml`.

### Основные переменные

| Переменная | По умолчанию | Описание |
|------------|--------------|----------|
| `sendry_install_method` | `binary` | Способ установки: `binary` или `package` |
| `sendry_version` | `latest` | Версия для установки |
| `sendry_hostname` | `inventory_hostname` | FQDN сервера |
| `sendry_api_key` | `""` | API ключ (обязательный) |
| `sendry_smtp_auth_users` | `{}` | Пользователи SMTP (обязательный) |
| `sendry_domains` | `{}` | Конфигурация доменов |
| `sendry_dkim_keys` | `{}` | Общие DKIM ключи для кластера |
| `sendry_dkim_generate` | `true` | Генерировать ключи если не заданы |
| `sendry_configure_firewall` | `false` | Настроить UFW/firewalld |
| `sendry_service_enabled` | `true` | Включить автозапуск |
| `sendry_service_state` | `started` | Состояние сервиса |
