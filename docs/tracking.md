# Tracking: opens, clicks, unsubscribes

All tracking lives in sendry-web (the panel). The MTA (`sendry`) is unchanged and knows nothing about tracking. Only the panel talks to the MTA, so it sees every message it sent itself: campaign jobs and mail sent through the panel API (port 8088).

If an external system calls the MTA API (port 8080) directly, the panel does not see those messages and cannot track them.

## What is tracked

| Event | How it works |
|---|---|
| Open | A transparent 1x1 pixel is inserted into the HTML: `GET /t/o/{token}.gif` |
| Click | Links are replaced with `GET /t/c/{token}`; the panel answers 302 to the original URL |
| Unsubscribe | Page `GET /u/{token}` with a button, `POST /u/{token}` for the button and for one-click (RFC 8058) |

Public endpoints need no authentication and bypass `server.allowed_ips`. All other panel paths are protected as before.

Tokens are HMAC-signed. The destination URL is inside the token, so it cannot be tampered with and the redirect cannot be pointed elsewhere. The domain is not part of the token: links work through any domain that proxies to the panel.

### Token lifetime

Every token carries its issue time. Old tokens stop working:

| Token | Lifetime |
|---|---|
| Open pixel, click | 180 days |
| Unsubscribe | 2 years |

A click token is a signed redirect to an external address, so it must not live forever. Unsubscribe links must keep working in old mail for much longer.

Behaviour on an invalid or expired token:

- the pixel still returns a 1x1 GIF (no event recorded), with `Cache-Control: no-store`;
- a click returns 404 for an invalid token and 410 for an expired one, without a redirect;
- the unsubscribe page says the link is invalid.

### Secret rotation

Tokens are signed with a secret stored in the database (generated on first use). **Settings → Tracking → Rotate secret** creates a new one. The current secret becomes the previous one and is still accepted, so links in mail already sent keep working. The secret before the previous one is dropped: links signed with it stop working. Rotate if the secret may have leaked (for example, a database backup got out), and do not rotate twice in a row without a reason.

## How to enable

### Settings

**Settings → Tracking** (admin only). Values are stored in the database, no restart needed.

| Field | Description |
|---|---|
| Proxying domain | Domain the pixel, clicks and unsubscribe go through, e.g. `lnk.example.com`. `https://` is optional. Empty: `server.public_url` is used |
| Opens enabled by default | Whether new campaigns track opens |
| Clicks enabled by default | Same for clicks |

If the proxying domain is empty, tracking links and redirects go through the panel address (`server.public_url`). The settings page warns about it: a separate domain is recommended, see [Separate tracking domain](#separate-tracking-domain-proxy).

If neither a proxying domain nor `server.public_url` is set, a campaign cannot be sent: every campaign message must carry an unsubscribe link, and there is no public address for it. Jobs already running or scheduled send nothing then: their messages wait in the queue and the job page shows "Sending is on hold". Once the address is set, the job continues by itself. API mail goes out without pixel and tracked links.

### Campaign

The campaign form has three options:

- **Track opens**;
- **Track clicks**;
- **Add UTM tags**.

Defaults for a new campaign come from Settings. Unsubscribe is always on and does not depend on these options: a campaign with opens and clicks off still gets the unsubscribe link and `List-Unsubscribe` headers.

UTM tags are added to the URL on redirect:

| Parameter | Value |
|---|---|
| `utm_source` | `email` |
| `utm_medium` | `email` |
| `utm_campaign` | campaign name |
| `sc` | campaign ID |
| `sr` | job item ID |
| `sl` | link number |

Parameters already present in the link are not overridden.

### API key

The API key form has `track_opens` and `track_clicks` flags. Both are off by default: transactional mail looks cleaner to spam filters without a pixel.

### API request fields

`POST /api/v1/send` and `POST /api/v1/send/template` of the panel API accept optional boolean fields `track_opens` and `track_clicks`. They override the key flags.

```json
{
  "from": "shop@example.com",
  "to": ["client@example.com"],
  "subject": "Your order",
  "html": "<p>Order placed. <a href=\"https://shop.example.com/orders/1\">View order</a></p>",
  "track_opens": true,
  "track_clicks": false
}
```

If a field is omitted, the key flag applies.

There is no unsubscribe for API mail at this stage: no unsubscribe link, no `List-Unsubscribe` headers, and unsubscribed addresses are not checked.

## Unsubscribe

Every campaign message gets these headers:

```
List-Unsubscribe: <https://lnk.example.com/u/{token}>
List-Unsubscribe-Post: List-Unsubscribe=One-Click
```

Gmail and Yandex show their own "Unsubscribe" button based on them.

Templates get the variable `{{unsubscribe_url}}`. Put it in the footer:

```html
<a href="{{unsubscribe_url}}">Unsubscribe</a>
```

If the template lacks it, the job start page shows a warning.

### Flow

1. The reader opens `/u/{token}` and sees a page with the address and an "Unsubscribe" button.
2. The unsubscribe happens only after the button is pressed (POST). Antivirus software opens every link in a message, so unsubscribing on a plain visit would unsubscribe people without their knowledge.
3. The page says that <email> is being unsubscribed from this mailing list of <sender>, and that other mailings the reader is subscribed to keep coming. The internal address book name is not shown. After the click it says "You are unsubscribed" and offers "Resubscribe".
4. A one-click request from a mail client (via `List-Unsubscribe-Post`) unsubscribes immediately, without a page. Both body formats are accepted: `multipart/form-data` (recommended by RFC 8058) and `application/x-www-form-urlencoded`.
5. The unsubscribe link keeps working after the campaign is deleted or old jobs are cleaned up (`sendry-web cleanup`): the token names the recipient, and the panel finds the address book through it. The sender name is not shown on the page then.

### Unsubscribe applies to one address book

An unsubscribe applies to one address book: the book the job was sent to. The reader gets the recipient status `unsubscribed` in that book. The source of truth is the recipient status (`recipients.status`).

- If the same address is in another book, a campaign sent to that book still reaches it.
- Re-importing a CSV into the book does not resubscribe the address.
- Unsubscribed recipients of the book are not added to a job at start. The job page shows "Skipped (unsubscribed in this book)": the number of unsubscribed recipients of the book at job start.

There is no separate Unsubscribes section. Unsubscribed addresses are visible in the address book (`/recipients/{id}`): the status filter "Unsubscribed" lists them. The status can be changed back in the recipient editor.

The `unsubscribes` table is a journal per (book, email): when, from which campaign and how (link or one-click) the address unsubscribed. The panel only writes it; sending decisions use the recipient status.

## Click map

The click map is the **Click map** tab on the job page (campaign jobs only).

Links are numbered in the template source: every `<a href>` gets a `data-sl="N"` attribute before variables are rendered. The link number is therefore the same for all recipients, even with different data and conditional blocks.

Not tracked:

- `mailto:`, `tel:`, `sms:`;
- `#anchor` links;
- links with the `data-notrack` attribute;
- the `{{unsubscribe_url}}` link;
- addresses that do not parse as a URL (e.g. `https://a.com/50%off`): such a link goes out as is, otherwise the redirect to it would fail.

Outlook buttons inside conditional comments (`<!--[if mso]> ... <![endif]-->`, including VML `<v:roundrect href=...>`) are kept in the message and tracked with the same number as the regular link to the same address. Plain HTML comments are removed from the message.

When a job starts, the panel saves a snapshot of each message variant. If the template is edited later, the map stays correct.

The tab shows:

- the message snapshot in an `iframe` with badges on links: unique and total clicks;
- a table: element label, URL, unique clicks, total, share;
- a variant selector if there are several (A/B).

Links hidden by a condition and not visible in the preview appear only in the table. API mail has no map; the message card shows a list of clicks.

## Metrics

| Metric | Formula |
|---|---|
| Delivered | Messages accepted by the MTA: status "queued" or "sent". Failed, skipped and not yet sent messages are not counted |
| Unique opens | Delivered messages opened at least once |
| Total opens | Sum of open counters of delivered messages |
| Open Rate | unique opens / delivered |
| Unique clicks | Delivered messages clicked at least once |
| Total clicks | Sum of click counters of delivered messages |
| CTR | unique clicks / delivered |
| CTOR | unique clicks / unique opens |
| Unsubscribes | Delivered messages that led to an unsubscribe. If the reader resubscribed, the message no longer counts |
| Unsubscribe % | unsubscribes / delivered |

All counts use the same base (delivered messages), so no rate exceeds 100%.

A click implies an open: a message clicked with images blocked counts as opened. If the campaign did not track opens when the job was started, the job's open metrics are 0 (otherwise Open Rate would consist of clicks only) and CTOR is not computed. Changing the campaign setting later does not change the statistics of jobs already started.

Campaign metrics sum all jobs of the campaign except dry runs. "Skipped as unsubscribed" counts distinct recipients: an address skipped by two jobs counts once.

The engagement cards are hidden when the campaign tracks neither opens nor clicks and there are no unsubscribes.

Metrics appear on the job and campaign pages. The job item list has "Opened" and "Clicks" columns. Send History has an "Opened" column and an event feed in the message card.

### Machine events

Events from scanners, bots, clicks with an empty User-Agent, and clicks within 30 seconds after the message was handed to the MTA (link scanners of the receiving side) are stored with the `is_machine` flag and excluded from statistics. Raw data is not discarded.

### Repeats and limits

The public endpoints accept requests from anyone, so the panel limits what gets stored:

- a repeat of the same event (message, type, link, IP) within 30 seconds is not recorded and does not change the counters: reloading the pixel or double-clicking counts once;
- User-Agent is cut to 512 bytes and the URL to 2048 bytes;
- request headers are limited to 16 KB, the unsubscribe form body to 64 KB.

Gmail and Yandex image proxies count as real opens.

## Limitations

- Apple Mail Privacy Protection preloads images itself, so opens for Apple Mail recipients are inflated. Read Open Rate with that in mind. Clicks are not affected.
- Some clients block images. Those opens are not seen.
- An API message with several recipients (`to`, `cc`, `bcc`) has one pixel. An open counts for the message, not for a specific recipient.
- No unsubscribe for API mail yet.
- Raw events are kept until the job or campaign is deleted (table `email_events`). Retention will be decided later.
- User-Agent parsing, GeoIP, hourly charts, spam complaints and bounces are out of scope. User-Agent and IP are stored.

## Privacy

Tracking records the IP address, User-Agent, time and recipient address. This is personal data. Disclose tracking in your privacy policy and follow local rules (GDPR and others). Every campaign must have an unsubscribe link.

## Separate tracking domain (proxy)

Links in emails can be served from a separate domain (for example `lnk.example.com`) instead of the panel domain. If that domain lands on a blocklist, you can replace it in minutes, and the panel itself stays closed.

An nginx proxy passes only `/t/` and `/u/` to the panel. Everything else answers 404.

### nginx example

```nginx
server {
    listen 80;
    server_name lnk.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name lnk.example.com;

    ssl_certificate     /etc/letsencrypt/live/lnk.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/lnk.example.com/privkey.pem;

    # Tracking only: opens, clicks, unsubscribe
    location ~ ^/(t|u)/ {
        proxy_pass https://panel.example.com;
        proxy_ssl_server_name on;

        proxy_set_header Host panel.example.com;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header User-Agent $http_user_agent;
    }

    # Everything else is closed
    location / {
        return 404;
    }
}
```

`$remote_addr` here is the reader IP.

### Reader IP and trusted proxies

The panel takes the reader IP from `X-Real-IP` and `X-Forwarded-For` only if the request came from a trusted proxy listed in `server.trusted_proxies` of `web.yaml`. Otherwise anyone could put any IP into these headers, and the TCP peer address is used instead.

```yaml
server:
  trusted_proxies:
    - "127.0.0.0/8"     # nginx on the panel host
    - "::1/128"
    - "203.0.113.10"    # the lnk.example.com proxy server
```

If the option is omitted, loopback and private networks are trusted: `127.0.0.0/8`, `::1/128`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `fc00::/7` (a reverse proxy on the same host or a Docker bridge). An empty list `[]` trusts nobody. If hosts of your private network can reach the panel directly, list only the real proxies.

With a chain of proxies (lnk proxy, then nginx on the panel host) list all of them. The reader IP is the right-most `X-Forwarded-For` address that is not a trusted proxy: proxies append the address that connected to them, while the left part may come from the client. `X-Real-IP` is used only when there is no `X-Forwarded-For`: some proxies (Caddy, HAProxy) pass a client-supplied `X-Real-IP` through unchanged.

The same rule applies to the client IP of panel API sends, the `server.allowed_ips` check and the audit log.

### Setup steps

1. Create an A/AAAA record for `lnk.example.com` pointing to the proxy server and get a TLS certificate (for example with certbot).
2. Put the config above into nginx and reload it.
3. In the panel open **Settings → Tracking** and set "Proxying domain" to `lnk.example.com`.
4. Check: `curl -I https://lnk.example.com/t/o/test.gif` returns a GIF, and `curl -I https://lnk.example.com/` returns 404.

New messages use the new domain immediately, no restart needed.

### Switching domains quickly

If a domain gets blocked:

1. Register a new domain and set up the proxy as above (the same server works: add another `server_name`).
2. Change "Proxying domain" in Settings.
3. Do not drop the old domain right away. Tokens are not bound to a domain, so links in already sent messages work again as soon as the old domain points to the same panel.

To serve several domains at once, add several `server_name` entries with the same `location`.
