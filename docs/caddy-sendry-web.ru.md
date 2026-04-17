# Пример конфигурации Caddy для sendry-web

Этот пример показывает, как:
- проксировать запросы к sendry-web,
- раздавать загруженные медиа-файлы из `/uploads/`,
- включить базовые заголовки кэширования для изображений,
- (опционально) получить автоматический HTTPS от Let's Encrypt.

## Пример Caddyfile (один домен)

```caddy
mail.example.com {
    # Загруженные медиа-файлы, используемые в шаблонах писем
    handle_path /uploads/* {
        root * /var/lib/sendry/uploads
        header Cache-Control "public, max-age=2592000"
        header -Server
        file_server
    }

    # Приложение sendry-web
    reverse_proxy 127.0.0.1:8080 {
        header_up Host {host}
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {remote_host}
        header_up X-Forwarded-Proto {scheme}
    }
}
```

## Вариант только по HTTP (без автоматического TLS)

Если Caddy стоит за другим TLS-терминатором или нужен просто HTTP:

```caddy
http://mail.example.com {
    handle_path /uploads/* {
        root * /var/lib/sendry/uploads
        header Cache-Control "public, max-age=2592000"
        file_server
    }

    reverse_proxy 127.0.0.1:8080
}
```

## Пример Caddyfile (отдельный домен для uploads)

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

```caddy
# Основное приложение sendry-web
mail.example.com {
    reverse_proxy 127.0.0.1:8080 {
        header_up Host {host}
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {remote_host}
        header_up X-Forwarded-Proto {scheme}
    }
}

# Загруженные медиа-файлы на выделенном домене
cdn.example.com {
    handle_path /uploads/* {
        root * /var/lib/sendry/uploads
        encode zstd gzip

        header Cache-Control "public, max-age=2592000"
        header Access-Control-Allow-Origin "*"
        header -Server

        file_server
    }

    # По желанию: 404 на корне, чтобы не светить листинг
    respond / 404
}
```

## Примечания

- Убедитесь, что `server.upload_path` в конфигурации sendry-web указывает на `/var/lib/sendry/uploads` (или измените `root` соответственно).
- Оба домена должны иметь доступ к одной и той же директории `uploads` (та же машина, NFS или дополнительный proxy на основной хост).
- Директория должна быть доступна на чтение пользователю Caddy и на запись пользователю службы sendry-web.
- Если блок сайта начинается с голого имени хоста (без префикса `http://`), Caddy автоматически получит и обновит сертификат Let's Encrypt — это работает и для `mail.example.com`, и для `cdn.example.com`.
- `handle_path` удаляет префикс `/uploads` перед поиском файла, поэтому `/uploads/foo.png` отображается в `/var/lib/sendry/uploads/foo.png`.
- При использовании отдельного домена для uploads добавьте `Access-Control-Allow-Origin`, чтобы просмотрщики HTML-писем / веб-превью могли загружать ресурсы с другого origin.
- `public_upload_url` влияет только на пути `/uploads/`; `/static/` по-прежнему переписывается из `public_url`.
- Перезагрузить конфигурацию после изменений: `sudo systemctl reload caddy` или `caddy reload --config /etc/caddy/Caddyfile`.
