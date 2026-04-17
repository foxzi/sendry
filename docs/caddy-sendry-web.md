# Caddy configuration example for sendry-web

This example shows how to:
- proxy requests to sendry-web,
- serve uploaded media files from `/uploads/`,
- enable basic cache headers for images,
- (optionally) get automatic HTTPS from Let's Encrypt.

## Example Caddyfile (single domain)

```caddy
mail.example.com {
    # Uploaded media files used in email templates
    handle_path /uploads/* {
        root * /var/lib/sendry/uploads
        header Cache-Control "public, max-age=2592000"
        header -Server
        file_server
    }

    # sendry-web application
    reverse_proxy 127.0.0.1:8080 {
        header_up Host {host}
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {remote_host}
        header_up X-Forwarded-Proto {scheme}
    }
}
```

## HTTP-only variant (no automatic TLS)

If Caddy is behind another TLS terminator or you just need plain HTTP:

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

## Example Caddyfile (separate domain for uploads)

Use this when uploaded media should be served from a dedicated hostname
(e.g. `cdn.example.com`) different from the main sendry-web domain
(`mail.example.com`). Useful for cookie-less asset delivery, separate
caching, or putting a CDN in front.

Set `server.public_upload_url` in `web.yaml` so sendry-web rewrites
`src="/uploads/…"` in rendered email HTML to the CDN hostname:

```yaml
server:
  public_url: "https://mail.example.com"
  public_upload_url: "https://cdn.example.com"
  upload_path: "/var/lib/sendry/uploads"
```

With that configured, templates can keep using relative `/uploads/…`
paths — on send they become absolute URLs on `cdn.example.com`
automatically.

```caddy
# Main sendry-web application
mail.example.com {
    reverse_proxy 127.0.0.1:8080 {
        header_up Host {host}
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {remote_host}
        header_up X-Forwarded-Proto {scheme}
    }
}

# Uploaded media files on a dedicated domain
cdn.example.com {
    handle_path /uploads/* {
        root * /var/lib/sendry/uploads
        encode zstd gzip

        header Cache-Control "public, max-age=2592000"
        header Access-Control-Allow-Origin "*"
        header -Server

        file_server
    }

    # Optional: root returns 404 to avoid exposing directory listing
    respond / 404
}
```

## Notes

- Ensure `server.upload_path` in sendry-web config points to `/var/lib/sendry/uploads` (or change `root` accordingly).
- Both hostnames must have access to the same physical `uploads` directory (same host, NFS share, or an extra proxy to the main host).
- Directory must be readable by the Caddy service user and writable by the sendry-web service user.
- When a site block uses a bare hostname (no `http://` prefix), Caddy will obtain and renew a Let's Encrypt certificate automatically — this applies to both `mail.example.com` and `cdn.example.com`.
- `handle_path` strips the `/uploads` prefix before looking up the file, so `/uploads/foo.png` maps to `/var/lib/sendry/uploads/foo.png`.
- When using a separate uploads domain, add `Access-Control-Allow-Origin` so HTML email viewers / web previews can load assets cross-origin.
- `public_upload_url` only affects `/uploads/` paths; `/static/` assets continue to be rewritten using `public_url`.
- Reload config after changes: `sudo systemctl reload caddy` or `caddy reload --config /etc/caddy/Caddyfile`.
