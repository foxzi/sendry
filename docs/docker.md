# Docker Deployment

Running Sendry with Docker and Docker Compose.

## Quick Start

```bash
# Clone repository
git clone https://github.com/foxzi/sendry.git
cd sendry

# Copy and edit configs
cp configs/sendry.example.yaml configs/sendry.yaml
cp configs/web.example.yaml configs/web.yaml

# Start services
docker compose up -d
```

## Services

| Service | Port | Description |
|---------|------|-------------|
| sendry | 25, 465, 587 | SMTP/SMTPS server |
| sendry | 8080 | HTTP API |
| sendry | 9090 | Prometheus metrics |
| sendry-web | 8088 | Web management interface |

## Configuration

### Sendry MTA (configs/sendry.yaml)

```yaml
server:
  hostname: mail.example.com

smtp:
  port: 25
  submission_port: 587
  smtps_port: 465

api:
  port: 8080
  auth:
    api_keys:
      - key: "your-api-key"
        name: "default"

domains:
  example.com:
    mode: production

logging:
  level: info
  format: json
```

### Sendry Web (configs/web.yaml)

```yaml
server:
  listen_addr: ":8088"

database:
  path: "/var/lib/sendry-web/app.db"

auth:
  local_enabled: true
  session_secret: "change-me-to-random-32-chars-minimum"
  session_ttl: 24h

sendry:
  servers:
    - name: "sendry"
      base_url: "http://sendry:8080"  # Docker internal network
      api_key: "your-api-key"         # Same as in sendry.yaml
      env: "prod"

logging:
  level: info
  format: json
```

## Running Services

### Start All Services

```bash
docker compose up -d
```

### Start Only MTA

```bash
docker compose up sendry -d
```

### Start Only Web Panel

```bash
docker compose up sendry-web -d
```

### View Logs

```bash
# All services
docker compose logs -f

# Specific service
docker compose logs -f sendry
docker compose logs -f sendry-web
```

### Stop Services

```bash
docker compose down
```

### Rebuild After Changes

```bash
docker compose up -d --build
```

## Creating First User (Sendry Web)

After starting sendry-web, create an admin user:

```bash
docker compose exec sendry-web /usr/bin/sendry-web user create \
  --email admin@example.com \
  --password your-password \
  --config /etc/sendry/web.yaml
```

Then open http://localhost:8088 and login.

## Volumes

| Volume | Path | Description |
|--------|------|-------------|
| sendry-data | /var/lib/sendry | Queue, DKIM keys, metrics |
| sendry-web-data | /var/lib/sendry-web | SQLite database |

### Backup

```bash
# Stop services
docker compose stop

# Backup volumes
docker run --rm -v sendry_sendry-data:/data -v $(pwd):/backup alpine \
  tar czf /backup/sendry-data-backup.tar.gz -C /data .

docker run --rm -v sendry_sendry-web-data:/data -v $(pwd):/backup alpine \
  tar czf /backup/sendry-web-data-backup.tar.gz -C /data .

# Start services
docker compose start
```

## Environment Variables

You can set these in `.env` file or pass directly:

```bash
# .env
VERSION=0.4.0
TZ=Europe/Moscow
```

```bash
# Or pass directly
TZ=Europe/Moscow docker compose up -d
```

## Network

Both services share `sendry-net` network. Sendry Web connects to Sendry MTA via internal hostname `sendry:8080`.

## Production Recommendations

1. **Use external configs** - mount configs from host instead of copying
2. **Use secrets** - don't commit API keys and passwords
3. **Set up TLS** - use reverse proxy (nginx, traefik) for HTTPS
4. **Monitor** - connect Prometheus to :9090/metrics
5. **Backup** - regularly backup volumes

### Example with Traefik

```yaml
services:
  sendry-web:
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.sendry-web.rule=Host(`panel.example.com`)"
      - "traefik.http.routers.sendry-web.tls.certresolver=letsencrypt"
      - "traefik.http.services.sendry-web.loadbalancer.server.port=8088"
```

## Troubleshooting

### Container won't start

```bash
# Check logs
docker compose logs sendry
docker compose logs sendry-web

# Check config syntax
docker compose exec sendry /usr/bin/sendry config validate
```

### Web panel can't connect to MTA

1. Check API key matches in both configs
2. Check `base_url` uses Docker hostname: `http://sendry:8080`
3. Check both containers are on same network

### Permission denied on volumes

```bash
# Fix ownership
docker compose exec sendry chown -R sendry:sendry /var/lib/sendry
docker compose exec sendry-web chown -R sendry:sendry /var/lib/sendry-web
```
