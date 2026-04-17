# Пример конфигурации Nginx для sendry-web

Этот пример показывает, как:
- проксировать запросы к sendry-web,
- раздавать загруженные медиа-файлы из `/uploads/`,
- включить базовые заголовки кэширования для изображений.

## Пример конфигурации (один домен)

```nginx
server {
    listen 80;
    server_name mail.example.com;

    # Загруженные медиа-файлы, используемые в шаблонах писем
    location /uploads/ {
        alias /var/lib/sendry/uploads/;
        access_log off;
        expires 30d;
        add_header Cache-Control "public, max-age=2592000";
        try_files $uri =404;
    }

    # Приложение sendry-web
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Пример конфигурации (отдельный домен для uploads)

Используйте этот вариант, когда загруженные медиа-файлы нужно отдавать
с выделенного хоста (например, `cdn.example.com`), отличного от
основного домена sendry-web (`mail.example.com`). Полезно для раздачи
статики без cookies, отдельного кэширования или если перед статикой
стоит CDN.

Пропишите `server.public_upload_url` в `web.yaml`, чтобы sendry-web
переписывал `src="/uploads/…"` в исходящем HTML писем на CDN-хост:

```yaml
server:
  public_url: "https://mail.example.com"
  public_upload_url: "https://cdn.example.com"
  upload_path: "/var/lib/sendry/uploads"
```

После этого в шаблонах можно использовать относительные пути
`/uploads/…` — при отправке они автоматически станут абсолютными
URL на `cdn.example.com`.

```nginx
# Основное приложение sendry-web
server {
    listen 80;
    server_name mail.example.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}

# Загруженные медиа-файлы на выделенном домене
server {
    listen 80;
    server_name cdn.example.com;

    location /uploads/ {
        alias /var/lib/sendry/uploads/;
        access_log off;
        expires 30d;
        add_header Cache-Control "public, max-age=2592000";
        add_header Access-Control-Allow-Origin "*";
        try_files $uri =404;
    }

    # По желанию: 404 на корне, чтобы не светить листинг директории
    location = / {
        return 404;
    }
}
```

## Примечания

- Убедитесь, что `server.upload_path` в конфигурации sendry-web указывает на `/var/lib/sendry/uploads` (или измените `alias` соответственно).
- Оба домена должны иметь доступ к одной и той же директории `uploads` (та же машина, NFS или дополнительный proxy на основной хост).
- Директория должна быть доступна на чтение пользователю nginx и на запись пользователю службы sendry-web.
- Для продакшена (почтовые клиенты получателей) используйте HTTPS и публичный домен.
- При использовании отдельного домена для uploads добавьте `Access-Control-Allow-Origin`, чтобы просмотрщики HTML-писем / веб-превью могли загружать ресурсы с другого origin.
- `public_upload_url` влияет только на пути `/uploads/`; `/static/` по-прежнему переписывается из `public_url`.
