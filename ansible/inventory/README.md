# Inventory Examples

## Single Server

Simple deployment with MTA and web panel on one server:

```yaml
sendry_single:
  hosts:
    mail.example.com:
      ansible_host: 192.168.1.10
      sendry_hostname: mail.example.com
      sendry_api_key: "your-api-key"
      sendry_smtp_auth_users:
        admin: "password"
      sendry_domains:
        example.com:
          dkim_enabled: true
          dkim_selector: mail
```

Deploy:
```bash
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml
```

## MTA Cluster with Shared DKIM

Multiple MTA servers sending for the same domains (requires shared DKIM keys):

```yaml
sendry_cluster:
  hosts:
    mail1.example.com:
      ansible_host: 192.168.1.20
    mail2.example.com:
      ansible_host: 192.168.1.21
  vars:
    sendry_api_key: "shared-api-key"
    sendry_domains:
      example.com:
        dkim_enabled: true
        dkim_selector: mail
```

Deploy:
```bash
# 1. Deploy MTA servers
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml -l sendry_cluster

# 2. Generate shared DKIM key
ansible-playbook playbooks/dkim.yml -e dkim_action=generate -e dkim_domain=example.com

# 3. Deploy DKIM to all servers
ansible-playbook playbooks/dkim.yml -e dkim_action=deploy -l sendry_cluster

# 4. Add DNS record (shown after generate)
```

## Web Panel + MTA Cluster

Separate web panel connecting to multiple MTA servers via API. Best for:
- Centralized management of multiple MTA servers
- Load balancing between servers
- Automatic failover

### Architecture

```
                    ┌─────────────────┐
                    │   Web Panel     │
                    │ panel.example.com│
                    └────────┬────────┘
                             │ API
              ┌──────────────┼──────────────┐
              │              │              │
              ▼              ▼              ▼
       ┌──────────┐   ┌──────────┐   ┌──────────┐
       │  MTA-1   │   │  MTA-2   │   │  MTA-3   │
       │ :8080    │   │ :8080    │   │ :8080    │
       └──────────┘   └──────────┘   └──────────┘
```

### Inventory Configuration

```yaml
all:
  children:
    # MTA servers
    sendry_mta_cluster:
      hosts:
        mta-1.example.com:
          ansible_host: 192.168.1.30
          sendry_hostname: mta-1.example.com
        mta-2.example.com:
          ansible_host: 192.168.1.31
          sendry_hostname: mta-2.example.com
      vars:
        sendry_mta_enabled: true
        sendry_web_enabled: false
        sendry_api_key: "mta-cluster-api-key"
        sendry_smtp_auth_users:
          admin: "smtp-password"
        sendry_domains:
          example.com:
            dkim_enabled: true
            dkim_selector: mail

    # Web panel (separate server)
    sendry_panel:
      hosts:
        panel.example.com:
          ansible_host: 192.168.1.40
      vars:
        sendry_mta_enabled: false
        sendry_web_enabled: true
        sendry_web_session_secret: "secret-at-least-32-characters"

        # Connect to MTA servers
        sendry_web_servers:
          - name: "mta-1"
            base_url: "http://192.168.1.30:8080"
            api_key: "mta-cluster-api-key"
            env: "prod"
          - name: "mta-2"
            base_url: "http://192.168.1.31:8080"
            api_key: "mta-cluster-api-key"
            env: "prod"

        # Load balancing
        sendry_web_multi_send_strategy: round_robin
        sendry_web_multi_send_failover_enabled: true

        # Caddy for HTTPS
        sendry_caddy_enabled: true
        sendry_caddy_domain: panel.example.com
        sendry_caddy_email: admin@example.com
```

### Deployment Steps

```bash
# 1. Deploy MTA servers
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml -l sendry_mta_cluster

# 2. Generate and deploy shared DKIM keys
ansible-playbook playbooks/dkim.yml -e dkim_action=generate -e dkim_domain=example.com
ansible-playbook playbooks/dkim.yml -e dkim_action=deploy -l sendry_mta_cluster

# 3. Deploy web panel
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml -l sendry_panel
```

### Load Balancing Strategies

#### Round Robin (default)
Distributes requests evenly across all servers:
```yaml
sendry_web_multi_send_strategy: round_robin
```

#### Weighted Round Robin
Sends more traffic to specific servers:
```yaml
sendry_web_multi_send_strategy: weighted_round_robin
sendry_web_multi_send_weights:
  mta-1: 3  # 75% of traffic
  mta-2: 1  # 25% of traffic
```

#### Domain Affinity
Routes specific sender domains to specific servers:
```yaml
sendry_web_multi_send_strategy: domain_affinity
sendry_web_multi_send_domain_affinity:
  "example.com": "mta-1"
  "example.org": "mta-2"
```

### Failover

When enabled, if primary server fails, request is automatically retried on another server:
```yaml
sendry_web_multi_send_failover_enabled: true
sendry_web_multi_send_failover_max_retries: 2
```

---

# Примеры Inventory (RU)

## Один сервер

Простой деплой с MTA и web панелью на одном сервере:

```yaml
sendry_single:
  hosts:
    mail.example.com:
      ansible_host: 192.168.1.10
      sendry_hostname: mail.example.com
      sendry_api_key: "ваш-api-ключ"
      sendry_smtp_auth_users:
        admin: "пароль"
      sendry_domains:
        example.com:
          dkim_enabled: true
          dkim_selector: mail
```

Деплой:
```bash
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml
```

## Кластер MTA с общими DKIM

Несколько MTA серверов для одних доменов (требуются общие DKIM ключи):

```bash
# 1. Деплой MTA серверов
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml -l sendry_cluster

# 2. Генерация общего DKIM ключа
ansible-playbook playbooks/dkim.yml -e dkim_action=generate -e dkim_domain=example.com

# 3. Деплой DKIM на все серверы
ansible-playbook playbooks/dkim.yml -e dkim_action=deploy -l sendry_cluster
```

## Web панель + кластер MTA

Отдельная web панель, подключённая к нескольким MTA серверам через API.

### Архитектура

```
                    ┌─────────────────┐
                    │   Web Панель    │
                    │ panel.example.com│
                    └────────┬────────┘
                             │ API
              ┌──────────────┼──────────────┐
              │              │              │
              ▼              ▼              ▼
       ┌──────────┐   ┌──────────┐   ┌──────────┐
       │  MTA-1   │   │  MTA-2   │   │  MTA-3   │
       └──────────┘   └──────────┘   └──────────┘
```

### Порядок деплоя

```bash
# 1. Деплой MTA серверов
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml -l sendry_mta_cluster

# 2. Генерация и деплой общих DKIM ключей
ansible-playbook playbooks/dkim.yml -e dkim_action=generate -e dkim_domain=example.com
ansible-playbook playbooks/dkim.yml -e dkim_action=deploy -l sendry_mta_cluster

# 3. Деплой web панели
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml -l sendry_panel
```

### Стратегии балансировки

- **round_robin** - равномерное распределение
- **weighted_round_robin** - взвешенное распределение
- **domain_affinity** - привязка доменов к серверам

### Failover

При включении, если основной сервер недоступен, запрос автоматически повторяется на другом:
```yaml
sendry_web_multi_send_failover_enabled: true
sendry_web_multi_send_failover_max_retries: 2
```
