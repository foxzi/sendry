# Nginx configuration example for sendry-web

This example shows how to:
- proxy requests to sendry-web,
- serve uploaded media files from `/uploads/`,
- enable basic cache headers for images.

## Example config (single domain)

```nginx
server {
    listen 80;
    server_name mail.example.com;

    # Uploaded media files used in email templates
    location /uploads/ {
        alias /var/lib/sendry/uploads/;
        access_log off;
        expires 30d;
        add_header Cache-Control "public, max-age=2592000";
        try_files $uri =404;
    }

    # sendry-web application
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

## Example config (separate domain for uploads)

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

```nginx
# Main sendry-web application
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

# Uploaded media files on a dedicated domain
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

    # Optional: 404 on root so the directory is not exposed
    location = / {
        return 404;
    }
}
```

## Notes

- Ensure `server.upload_path` in sendry-web config points to `/var/lib/sendry/uploads` (or change `alias` accordingly).
- Both hostnames must have access to the same physical `uploads` directory (same host, NFS share, or an extra proxy to the main host).
- Directory must be readable by nginx and writable by sendry-web service user.
- For production email clients, use HTTPS and a public domain.
- When using a separate uploads domain, add `Access-Control-Allow-Origin` so HTML email viewers / web previews can load assets cross-origin.
- `public_upload_url` only affects `/uploads/` paths; `/static/` assets continue to be rewritten using `public_url`.
