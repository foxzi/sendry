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

## Managing Many Domains

For deployments with many domains (10+), use the **web panel** instead of Ansible for domain management. Ansible is only needed for initial infrastructure setup.

### Why Panel is Better for Many Domains

| Feature | Ansible | Web Panel |
|---------|---------|-----------|
| Add domain | Edit YAML, run playbook | Click "New Domain" |
| Deploy to servers | Run playbook | Select checkboxes |
| DKIM generation | Manual or playbook | One click |
| DNS record | Shown in output | Copy button in UI |
| Sync changes | Re-run playbook | "Sync All" button |
| Track deployment status | No | Yes, per server |

### Recommended Workflow

#### Step 1: Deploy Infrastructure with Ansible (once)

```yaml
# inventory/hosts.yml
all:
  children:
    sendry_mta_cluster:
      hosts:
        mta-1.example.com:
          ansible_host: 192.168.1.30
        mta-2.example.com:
          ansible_host: 192.168.1.31
      vars:
        sendry_mta_enabled: true
        sendry_web_enabled: false
        sendry_api_key: "your-api-key"
        sendry_smtp_auth_users:
          admin: "smtp-password"
        # No domains here! Managed via panel
        sendry_domains: {}

    sendry_panel:
      hosts:
        panel.example.com:
          ansible_host: 192.168.1.40
      vars:
        sendry_mta_enabled: false
        sendry_web_enabled: true
        sendry_web_session_secret: "your-secret-32-chars"
        sendry_web_servers:
          - name: "mta-1"
            base_url: "http://192.168.1.30:8080"
            api_key: "your-api-key"
          - name: "mta-2"
            base_url: "http://192.168.1.31:8080"
            api_key: "your-api-key"
        sendry_web_multi_send_failover_enabled: true
        sendry_caddy_enabled: true
        sendry_caddy_domain: panel.example.com
        sendry_caddy_email: admin@example.com
```

```bash
# Deploy MTA servers
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml -l sendry_mta_cluster

# Deploy panel
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml -l sendry_panel
```

#### Step 2: Add Domains via Web Panel (ongoing)

1. **Open panel**: `https://panel.example.com`

2. **Create DKIM key** (`/dkim` → New):
   - Enter domain name (e.g., `example.com`)
   - Enter selector (default: `mail`)
   - Select servers to deploy (mta-1, mta-2)
   - Click Create
   - **Copy DNS record** and add to your DNS

3. **Create domain** (`/domains` → New):
   - Enter domain name
   - Select mode (production/sandbox/redirect/bcc)
   - Enable DKIM, select the key created above
   - Set rate limits if needed
   - Select servers to deploy
   - Click Create

4. **Repeat for each domain**

#### Step 3: Manage Domains (ongoing)

- **Edit domain**: `/domains/{id}` → Edit → Save → Sync All (redeploys to all servers)
- **Add new server**: Deploy with Ansible, then redeploy domains from panel
- **View status**: Panel shows deployment status per server (deployed/failed/outdated)

### Panel Features for Domain Management

```
/dkim                    - List all DKIM keys
/dkim/new               - Create new DKIM key
/dkim/{id}              - View key, DNS record, deploy to more servers

/domains                 - List all domains
/domains/new            - Create new domain
/domains/{id}           - View domain, deployment status
/domains/{id}/edit      - Edit domain config
/domains/{id}/deploy    - Deploy to additional servers
/domains/{id}/sync      - Sync changes to all servers
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

## Управление большим количеством доменов

Для деплоя с большим количеством доменов (10+) используйте **web панель** вместо Ansible. Ansible нужен только для начальной настройки инфраструктуры.

### Почему панель лучше для многих доменов

| Функция | Ansible | Web панель |
|---------|---------|-----------|
| Добавить домен | Редактировать YAML, запустить плейбук | Кнопка "New Domain" |
| Деплой на серверы | Запустить плейбук | Выбрать чекбоксы |
| Генерация DKIM | Вручную или плейбук | Один клик |
| DNS запись | В выводе команды | Кнопка копирования |
| Синхронизация | Перезапуск плейбука | Кнопка "Sync All" |
| Статус деплоя | Нет | Да, по каждому серверу |

### Рекомендуемый workflow

#### Шаг 1: Деплой инфраструктуры через Ansible (один раз)

```yaml
# inventory/hosts.yml
all:
  children:
    sendry_mta_cluster:
      hosts:
        mta-1.example.com:
          ansible_host: 192.168.1.30
        mta-2.example.com:
          ansible_host: 192.168.1.31
      vars:
        sendry_mta_enabled: true
        sendry_web_enabled: false
        sendry_api_key: "ваш-api-ключ"
        sendry_smtp_auth_users:
          admin: "smtp-пароль"
        # Домены не указываем! Управление через панель
        sendry_domains: {}

    sendry_panel:
      hosts:
        panel.example.com:
          ansible_host: 192.168.1.40
      vars:
        sendry_mta_enabled: false
        sendry_web_enabled: true
        sendry_web_session_secret: "секрет-минимум-32-символа"
        sendry_web_servers:
          - name: "mta-1"
            base_url: "http://192.168.1.30:8080"
            api_key: "ваш-api-ключ"
          - name: "mta-2"
            base_url: "http://192.168.1.31:8080"
            api_key: "ваш-api-ключ"
        sendry_web_multi_send_failover_enabled: true
        sendry_caddy_enabled: true
        sendry_caddy_domain: panel.example.com
        sendry_caddy_email: admin@example.com
```

```bash
# Деплой MTA серверов
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml -l sendry_mta_cluster

# Деплой панели
ansible-playbook -i inventory/hosts.yml playbooks/sendry.yml -l sendry_panel
```

#### Шаг 2: Добавление доменов через панель (постоянно)

1. **Открыть панель**: `https://panel.example.com`

2. **Создать DKIM ключ** (`/dkim` → New):
   - Ввести имя домена (например, `example.com`)
   - Ввести селектор (по умолчанию: `mail`)
   - Выбрать серверы для деплоя (mta-1, mta-2)
   - Нажать Create
   - **Скопировать DNS запись** и добавить в DNS

3. **Создать домен** (`/domains` → New):
   - Ввести имя домена
   - Выбрать режим (production/sandbox/redirect/bcc)
   - Включить DKIM, выбрать созданный ключ
   - Установить лимиты при необходимости
   - Выбрать серверы для деплоя
   - Нажать Create

4. **Повторить для каждого домена**

#### Шаг 3: Управление доменами (постоянно)

- **Редактировать домен**: `/domains/{id}` → Edit → Save → Sync All (передеплоит на все серверы)
- **Добавить новый сервер**: Деплой через Ansible, затем передеплоить домены из панели
- **Просмотр статуса**: Панель показывает статус деплоя по каждому серверу

### Возможности панели для управления доменами

```
/dkim                    - Список всех DKIM ключей
/dkim/new               - Создать новый DKIM ключ
/dkim/{id}              - Просмотр ключа, DNS запись, деплой на другие серверы

/domains                 - Список всех доменов
/domains/new            - Создать новый домен
/domains/{id}           - Просмотр домена, статус деплоя
/domains/{id}/edit      - Редактирование конфига
/domains/{id}/deploy    - Деплой на дополнительные серверы
/domains/{id}/sync      - Синхронизация изменений на все серверы
```
