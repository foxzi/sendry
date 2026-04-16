# DNS Sync

`sendry-web dns-sync` compares your domain's current DNS records with
Sendry's recommended values (SPF, DKIM, DMARC) and, optionally, creates or
updates them through a DNS provider API.

Supported providers:

- Cloudflare (`--provider cloudflare`)

## Recommended records

For a domain `example.com` with DKIM selector `mail`, the command checks:

| Kind  | Name                         | Type | Expected value                                                   |
|-------|------------------------------|------|------------------------------------------------------------------|
| SPF   | `example.com`                | TXT  | `v=spf1 a mx ~all` (or with `include:<spf_include>` when set)    |
| DMARC | `_dmarc.example.com`         | TXT  | `v=DMARC1; p=quarantine; rua=mailto:dmarc@example.com`           |
| DKIM  | `mail._domainkey.example.com`| TXT  | Value of the linked DKIM key's DNS record                        |

The SPF `include:` part is driven by the `spf_include` global variable
(Settings → Global Variables). Example values:

- `_spf.mailgun.org`
- `spf.sendgrid.net`

DKIM is checked only when DKIM is enabled on the domain **and** a DKIM key
is linked.

## Cloudflare authentication

Two modes are supported:

### API Token (recommended)

Create a token in Cloudflare dashboard with:

- Zone → Zone → Read
- Zone → DNS → Edit
- Zone Resources: *Include → All zones from an account → your account* (or specific zones)

Pass it via `--token` or `CLOUDFLARE_API_TOKEN`. This is the preferred option
and covers multiple domains under one account.

### Legacy Global API Key

Supported for backward compatibility. Global API Key grants full access to the
entire account, so an API Token with scoped permissions is safer.

Set:

- `--email <account-email>` or `CLOUDFLARE_API_EMAIL`
- `--token <global-key>` or `CLOUDFLARE_API_KEY`

You can force the mode with `--auth global` (auto mode selects it automatically
when `--email` is provided).

## Usage

Check a single domain (plan only, no changes):

```bash
sendry-web dns-sync --config /etc/sendry/web.yaml \
  --domain example.com \
  --token "$CLOUDFLARE_API_TOKEN"
```

Apply changes (API Token):

```bash
sendry-web dns-sync --config /etc/sendry/web.yaml \
  --domain example.com \
  --apply \
  --token "$CLOUDFLARE_API_TOKEN"
```

Apply changes with Global API Key:

```bash
sendry-web dns-sync --config /etc/sendry/web.yaml \
  --domain example.com --apply \
  --email "$CLOUDFLARE_API_EMAIL" \
  --token "$CLOUDFLARE_API_KEY"
```

Check all domains:

```bash
sendry-web dns-sync --config /etc/sendry/web.yaml --all \
  --token "$CLOUDFLARE_API_TOKEN"
```

## Output

```
DNS sync [plan] provider=cloudflare domains=1

=== example.com ===
KIND   NAME                         ACTION  STATUS   DETAILS
SPF    example.com                  noop    planned  matches expected value
DMARC  _dmarc.example.com           update  planned  value differs from expected
DKIM   mail._domainkey.example.com  create  planned  no current record found
```

Exit code is non-zero when any error occurs (for example: zone not found,
API error, or DKIM key is not linked and `--apply` cannot proceed safely).

## Notes

- The command never deletes records; it only creates or updates them.
- For TXT comparisons the value is normalized (surrounding quotes stripped,
  whitespace collapsed), so quoting differences don't trigger updates.
- When multiple TXT records share the same name, Sendry picks the one that
  matches the record family (`v=spf1`, `v=DMARC1`, `v=DKIM1`) to update.
