# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- sendry-web: open, click and unsubscribe tracking (the MTA is unchanged). Public endpoints `GET /t/o/{token}.gif`, `GET /t/c/{token}`, `GET /u/{token}`, `POST /u/{token}` (button and RFC 8058 one-click) with HMAC-signed tokens, no auth, bypassing `server.allowed_ips`
- sendry-web: Settings → Tracking (proxying domain, opens/clicks defaults), stored in the DB
- sendry-web: campaign options track opens, track clicks, add UTM tags; API key flags `track_opens`/`track_clicks` and the same request fields on `POST /api/v1/send` and `/api/v1/send/template`
- sendry-web: `List-Unsubscribe` / `List-Unsubscribe-Post` headers and `{{unsubscribe_url}}` variable for campaign mail, per-address-book unsubscribe (recipient status `unsubscribed` in the job's book only; `unsubscribes` is a per-(list, email) journal), unsubscribed addresses are visible in the address book with the status filter "Unsubscribed", unsubscribed recipients of the book skipped at job start and counted on the job
- sendry-web: job and campaign statistics (Open Rate, CTR, CTOR, unsubscribes), per-recipient open/click columns, click map with per-variant snapshots, machine event filtering (`is_machine`)
- Docs `docs/tracking.md` / `docs/tracking.ru.md` including an nginx example for a separate tracking domain
- GitHub Actions workflow `Release (button)`: computes the next semver from the latest tag and tags HEAD of main (patch/minor/major)
- GitHub Actions workflow `Deploy`: rolls out a released version to production servers over SSH with Ansible (all/mta/panel)
- Ansible playbook `playbooks/upgrade.yml`: upgrades the deb package only, backs up the panel SQLite database, restarts active services and checks `/health`
- Production inventory `ansible/inventory/prod.yml` and docs `docs/deploy.md` / `docs/deploy.ru.md`

### Changed
- `release.yml` can be started on an existing tag via `workflow_dispatch` (used by `Release (button)`, since tags pushed with `GITHUB_TOKEN` do not trigger it)
- sendry-web: `server.trusted_proxies` defaults to loopback and private networks (`127.0.0.0/8`, `::1/128`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `fc00::/7`) so a proxy in front of a Docker container keeps working; set the list explicitly if untrusted hosts of a private network can reach the panel directly
- sendry-web: `sendry.multi_send.strategy` is validated at startup: `round_robin`, `weighted`, `first_healthy`; `weighted_round_robin` (the name the docs used) is accepted as a deprecated alias of `weighted` and now really uses `weights` (before it silently fell back to `round_robin`); `domain_affinity` is accepted but not implemented yet and falls back to `round_robin` with a warning; any other value is a config error
- `make build-web-linux` / `build-web-linux-arm64` build a static binary on `golang:1.24-alpine` (musl), the same way as the release workflow
- go.mod: `go-oidc`, `oauth2`, `go-sqlite3`, `x/term`, `client_model` listed as direct dependencies (`go mod tidy`)
- **Upgrade note**, sendry-web: campaign mail needs a public address for the unsubscribe link (`server.public_url` or Settings → Tracking → proxying domain). Without it new campaigns cannot be started, and jobs already running or scheduled hold their emails until it is set (the job page says so). Check `server.public_url` before upgrading
- **Upgrade note**, sendry-web: the database migration lower-cases recipient emails (ASCII letters) and merges addresses that differ only in case within one list into one recipient (an unsubscribed duplicate wins; name and variables of the removed duplicate are lost). Back up the database before upgrading
- sendry-web: foreign keys are enforced on every database connection. A recipient list, recipient, campaign variant or template that a campaign job used cannot be deleted any more (409 with an explanation); delete the campaign first, or set a recipient to unsubscribed
- sendry-web: retrying failed job items re-reads the template of a variant that has not sent anything yet, so a fixed template is used; variants that sent mail keep the template frozen at job start
- sendry-web: the open rate of a job uses the campaign's open tracking at job start, not the current setting
- sendry-web: resubscribing removes the address from the unsubscribe statistics of past jobs
- sendry-web: only success messages hide after 5 seconds; warnings and errors (`alert-danger` too) stay, and so does the one-time display of a new API key
- **Upgrade note (breaking, API)**, sendry-web: the per-API-key `track_opens`/`track_clicks` flags and the global opens/clicks defaults of Settings → Tracking (v0.6.0) are removed, the migration drops them. `POST /api/v1/send` and `/api/v1/send/template` track opens or clicks only when the request has `"track_opens": true` / `"track_clicks": true`; clients that relied on the key defaults silently stop being tracked. Before upgrading, find such keys with `SELECT id, name FROM api_keys WHERE track_opens = 1 OR track_clicks = 1`
- sendry-web: without a public address (`server.public_url` or Settings → Tracking) campaign jobs are no longer held: mail is sent without the unsubscribe link, `List-Unsubscribe` headers and open/click tracking (replaces the upgrade note above). The campaign, send and job pages warn about it, the worker logs a warning once per job. Gmail and Yahoo require one-click unsubscribe from bulk senders, so set the address before sending to large lists
- sendry-web: campaigns are created, edited and sent on one page of rows (sender, content, variables, tracking, name); the old edit, variables and variants pages redirect to the campaign page, campaign tags are no longer shown or saved
- MTA: the `path` label of `sendry_api_requests_total` / `sendry_api_request_duration_seconds` is `unmatched` for requests that match no route (scanners added a series per probed URL, kept forever since counters are persisted)

### Fixed
- Security: sendry-web `server.allowed_ips` could be bypassed by sending a forged `X-Forwarded-For` / `X-Real-IP` header; the panel IP filter and the audit log IP now honour these headers only from `server.trusted_proxies`. The right-most untrusted `X-Forwarded-For` address wins and `X-Real-IP` is used only without `X-Forwarded-For`, since Caddy passes a client-supplied `X-Real-IP` through; the ansible Caddyfile and `docs/caddy-sendry-web.md` also overwrite `X-Real-IP`
- Security: MTA `api.allowed_ips` and `metrics.allowed_ips` could be bypassed with a forged `X-Forwarded-For` / `X-Real-IP` header from any address; these headers now count only from loopback (a reverse proxy on the same host)
- sendry-web: tracking events were lost under concurrent requests (`database is locked`): transactions now take the SQLite write lock at start (`_txlock=immediate`)
- sendry-web: RFC 8058 one-click unsubscribe with a `multipart/form-data` body answered success without unsubscribing
- sendry-web: unsubscribe links in mail already sent stopped working after the campaign was deleted or old job items were cleaned up; the token now names the recipient
- sendry-web: an invalid schedule time started the campaign immediately; it is now an error
- sendry-web: Outlook conditional comments (`<!--[if mso]>`) were removed from rendered templates, losing bulletproof buttons; they are kept now and their links tracked
- sendry-web: tracking left links that do not parse as a URL (`50%off`) untouched instead of turning them into a 404 redirect; an unquoted `href` ending in `/` lost the slash; `<a` inside attribute values, `<textarea>`, `<title>` and after `<!-->` was mistaken for a link
- sendry-web: a machine click no longer hides a human click from the same IP within 30 seconds; resume and retry keep the job's first start time
- sendry-web: missing translations on Settings → Tracking
- Metrics persistence: counters incremented by the MTA (messages, SMTP connections/auth, rate limits, API requests/errors) were never written to BoltDB, so they reset on restart despite `docs/metrics.md`; they are now persisted and restored
- `sendry_smtp_tls_connections_total` is now incremented when an SMTP session runs over TLS (STARTTLS or implicit TLS)
- sendry-web: expired sessions are purged from the database every hour; session expiry is compared in the same time zone as it is stored
- sendry-web: `weighted` strategy no longer panics when all weights are zero
- sendry-web: broken links on the API keys page (`/settings/domains` → `/domains`) and the domain form (`/settings/dkim/new` → `/dkim/new`)
- Docs: `POST /api/v1/dkim/upload` documented; `analytics.md` points to the tracking docs; `domains.<d>.tls`, `default_from`, `recipients_per_message` and `domain_affinity` marked as not implemented yet

### Removed
- sendry-web: unused htmx script (loaded from unpkg on every page), the timezone selector in Settings (it only affected elements that no page renders), unused route `POST /domains/import`, the unrouted `ServerDomains` handler with its `server_domains.html` template, dead builder JS and CSS
- Unused Go code: unreachable functions and types reported by `deadcode`/`staticcheck` (sendry-web MTA client methods for templates, sandbox, DKIM generate/verify and DNSBL list; `queue.DeliveryAttempt`; `api.NewServer`, `smtp.NewServer`, `metrics.NewServer`, etc.)

## [0.4.20] - 2026-09-09

### Fixed
- API keys page: `.btn-warning`/`.btn-success` now use dark labels and domain badges a darker primary background to meet WCAG 4.5:1 contrast
- API keys page: domain disclosure button now exposes `aria-expanded` and a descriptive `aria-label` for screen readers

## [0.4.19] - 2026-07-13

### Fixed
- Email builder: `replaceTextVars` now supports dotted placeholder paths (e.g. `{{.User.Email}}`) and optional whitespace inside braces, so nested template variables are wrapped as styled placeholders in the preview instead of leaking raw template syntax
- Email builder: `enableInlineEdit` skips nodes whose ancestor is already `contenteditable`, preventing nested edit zones (e.g. `<a>` inside `<td>`) that broke source-index mapping in `saveInlineEdit`
- Email builder: initial `render()` is deferred until `DOMContentLoaded` in template/block views so `SendryPreview` helper is loaded before first use (avoids race with layout scripts)
- Email builder: `replaceHrefVars` now requires a whitespace boundary so it no longer rewrites `data-href` or `xlink:href` attributes

### Changed
- Extract duplicated dark-mode strip regex from email builder preview views into shared `preview-utils.js` helper

### Added
- Unit test `TestStripDarkModeCSS` covering wrapper removal, custom dark-mode preservation, and no-op cases

## [0.4.18] - 2026-05-12

### Added
- Documentation: `docs/analytics.md` and `docs/analytics.ru.md` describing why current telemetry is monitoring, not analytics, and proposing a dedicated `sendry-analytics` service with event log, open/click tracking, VERP bounce handler, and a SQLite-to-ClickHouse migration path
- Documentation: `docs/scaling.md` and `docs/scaling.ru.md` describing bottlenecks for large mailings and a staged plan to scale (batch API, webhook events, SMTP connection reuse, optional `sendry-dispatcher`, optional broker)
- API: `POST /api/v1/send/batch` accepts up to 1000 messages per request, validates each independently, and enqueues valid ones in a single BoltDB transaction (reduces write-lock contention on large mailings)
- Tests: batch API handler (accepted/rejected/mixed/empty/too-large)
- Queue: `BoltStorage.EnqueueBatch` atomically enqueues a slice of messages in one transaction
- Tests: `EnqueueBatch` happy/empty paths
- Ansible: `sendry_web_public_upload_url` renders `server.public_upload_url` in generated `web.yaml`
- Ansible: `sendry_caddy_uploads_enabled` adds a `/uploads/*` file server to the main panel site
- Ansible: `sendry_caddy_uploads_hosts` adds dedicated Caddy sites that serve `/uploads/*` only (supports custom scheme/port via verbatim address)
- Ansible: `caddy` system user is added to the sendry group when the web panel is enabled so it can read the uploads directory
- Tests: additional `dnssync` plan cases — DKIM skipped on empty `DNSRecord`, default selector `mail`, and quoted/whitespace-equivalent values treated as noop
- Unit test `TestStripDarkModeCSS` covering wrapper removal, custom dark-mode preservation, and no-op cases

### Changed
- Extract duplicated dark-mode strip regex from email builder preview views into shared `preview-utils.js` helper

### Fixed
- Email builder preview: `replaceHrefVars` now requires a whitespace boundary so it no longer rewrites `data-href` or `xlink:href` attributes
- Email builder preview: defer initial render until DOMContentLoaded so `SendryPreview` helper is loaded before use (avoids race with layout scripts)

## [0.4.17] - 2026-04-17

### Added
- Sendry Web CLI: `sendry-web dns-sync` command to compare current DNS with recommended SPF/DKIM/DMARC records and create/update them via DNS provider API
- DNS provider abstraction with Cloudflare implementation (token via `--token` or `CLOUDFLARE_API_TOKEN`)
- Cloudflare legacy Global API Key support via `--email`/`--token` (or `CLOUDFLARE_API_EMAIL`/`CLOUDFLARE_API_KEY`) with `--auth auto|token|global`
- Namedot DNS provider (github.com/foxzi/namedot) via `--provider namedot`, `--namedot-url`/`NAMEDOT_API_URL`, `--token`/`NAMEDOT_API_TOKEN`
- Sendry Web: `server.public_upload_url` config option to serve `/uploads/` links in outgoing email HTML from a dedicated host (e.g. CDN) separate from `public_url`; `/static/` continues to use `public_url`
- Tests: DNS sync plan/apply logic, SPF include handling, Cloudflare auth headers, Namedot zone/rrset operations, asset URL rewriting with separate upload host
- Documentation: `docs/dns-sync.md` and `docs/dns-sync.ru.md`
- Documentation: `docs/nginx-sendry-web.md`, `docs/nginx-sendry-web.ru.md`, `docs/caddy-sendry-web.md`, `docs/caddy-sendry-web.ru.md` with examples for single and separate-upload-domain setups

### Changed
- Documentation: `sendry-web.md` and `sendry-web.ru.md` describe `spf_include` global variable, link to `dns-sync` guide, and show both Cloudflare API Token and Global API Key examples
- Documentation: `README.md` and `docs/README.ru.md` link to Sendry Web and DNS sync guides
- Documentation: updated example versions to `0.4.17` in `README.md`, `docs/README.ru.md`, `docs/quickstart.md`, `docs/quickstart.ru.md`, `docs/docker.md`, and `docs/docker.ru.md`

## [0.4.15] - 2026-04-16

### Added
- Sendry Web: Recommended DNS SPF value in domain settings now supports global variable `spf_include` and renders `include:<value>` when configured
- Tests: SPF recommended DNS records in CentralDomainsView for both base value and value with `spf_include`

### Fixed
- Release workflow: add actions:write permission for workflow_dispatch trigger

## [0.4.14] - 2026-04-16

### Added
- CC and BCC address validation in template send endpoint
- Tests for CC/BCC functionality (envelope recipients, header presence, invalid addresses)

### Fixed
- Release workflow now explicitly triggers package publishing to avoid missed events
- Header injection vulnerability in template email builder (custom headers now sanitized)

## [0.4.13] - 2026-04-03

## [0.4.12] - 2026-03-04

### Added
- Sendry Web: Audit log now records all mutating actions: login, logout, user create/update/delete/password-change, template create/update/delete/deploy/import, campaign create/update/delete/send, global variable set/delete
- Sendry Web: LogAction helper in SettingsRepository extracts client IP automatically (X-Forwarded-For, X-Real-IP, RemoteAddr)
- Sendry Web: Audit log filter extended with Deploy, Send actions and User entity type
- Sendry Web: User management at /settings/users (create, edit, delete, change password)
- Sendry Web: Two user roles: admin and user; admins manage other users
- Sendry Web: AdminOnly middleware blocks non-admin access to /settings routes (returns 403)
- Sendry Web: Settings nav link hidden for non-admin users
- Sendry Web: First registered user auto-promoted to admin via migration
- Sendry Web: User role and ID stored in session context (extends Auth middleware)
- Sendry Web: Queue size badge in navbar with auto-refresh (polls every 30s)
- Sendry Web: NavStats API endpoint for navbar queue counter
- Sendry Web: i18n translations for queue badge (EN/RU)
- Tests: Middleware unit tests (GetUserEmail, GetAPIKeyFromContext, MethodOverride)
- Tests: Sendry client unit tests (Queue, DLQ, Purge operations, auth, errors)
- Sendry Web: Combined queue overview page at /queue with Queue and DLQ tabs
- Sendry Web: Queue overview fetches data from all servers in parallel
- Sendry Web: Per-server stats cards showing queue/DLQ counts
- Sendry Web: Search and column sorting on both Queue and DLQ tables
- Sendry Web: DLQ actions (Retry/Delete) directly from overview page

### Changed
- Sendry Web: Queue badge in navbar now links to /queue overview page instead of /servers

### Fixed
- Sendry Web: Deploy sends DKIM enabled with empty key_file when DKIM key is missing or unlinked
- Sendry MTA: Validate DKIM key_file is not empty when DKIM is enabled in domain create/update API

## [0.4.8] - 2026-03-02

### Added
- Sendry Web: Working View/Delete buttons for queue messages
- Sendry Web: Working Retry/Delete buttons for DLQ messages
- Sendry Web: Queue message detail page with full message info
- Sendry Web: Search/filter for queue and DLQ tables
- Sendry Web: Column sorting for queue and DLQ tables
- Sendry Web: Clear Queue / Clear DLQ buttons to purge all messages

### Fixed
- Sendry Web: Checkbox alignment - checkboxes now left-aligned instead of centered
- Sendry Web: Display actual user email in navbar instead of hardcoded value

## [0.4.7] - 2026-02-14

### Added
- GitHub Pages package repository (APT, YUM, APK) with automatic CI deployment
- Documentation: Package repository installation instructions (EN/RU)
- Ansible: Repository installation method (sendry_install_method: repository)
- Sendry Web: API key domain restrictions (limit which domains each key can send from)
- Sendry Web: API key editing (update name, domains, rate limits)
- Sendry Web: Domain selection UI in API key create/edit forms
- Scripts: test-api.sh for testing Sendry Web API (send, template, status commands)
- Sendry Web: Monitoring page with interactive charts (Chart.js)
- Sendry Web: Time series chart showing sent/failed/pending emails over time
- Sendry Web: Domain breakdown chart (top 10 sender domains by volume)
- Sendry Web: Period filter (24h, 7d, 30d) and domain filter for monitoring
- Sendry Web: API endpoints /monitoring/api/stats and /monitoring/api/servers

### Changed
- Ansible: Default install method changed from binary to repository

### Fixed
- Sendry MTA: Nil pointer dereference in domain API handlers when domainManager not initialized
- CI: Fix APT Release file format (missing file sizes, wrong paths)
- Sendry Web: Fix transparent modal background in API keys create/edit dialogs
- Sendry Web: Fix inconsistent fonts and row heights in API keys table
- Sendry Web: Fix stats widgets alignment on Send History page (now full width)

## [0.4.6] - 2026-02-14

### Added
- Sendry Web: Public API for email sending (POST /api/v1/send, POST /api/v1/send/template)
- Sendry Web: API key authentication system for public API
- Sendry Web: Email routing by sender domain with server selection
- Sendry Web: Round-robin and weighted load balancing between MTA servers
- Sendry Web: Failover support when primary server fails
- Sendry Web: Template integration for API sends
- Sendry Web: Send history with UI at /sends
- Sendry Web: API keys management at /settings/api-keys
- Sendry Web: GET /api/v1/send/{id}/status endpoint for tracking
- Sendry Web: API key rate limiting (per minute and per hour)
- Sendry Web: API key activation/deactivation toggle
- Sendry Web: IP filtering for entire panel (allowed_ips config option)
- Sendry Web: Add DNS Check section to domain view page
- Sendry Web: Add MTA server selection to sandbox test email page
- Sendry Web: Add test email sending to Settings page (/settings/test-email)
- Ansible: Add sendry_web_allowed_ips variable for web panel IP filtering
- Ansible: Add example inventory for panel + MTA cluster deployment
- Ansible: Add inventory README with deployment examples
- Ansible: Add documentation for managing many domains via web panel

### Fixed
- Ansible: Fix package download URL for DEB and RPM packages
- Sendry Web: Fix sandbox POST handler not registered
- Sendry Web: Fix DKIM deploy not updating domain configuration with KeyFile path
- Sendry Web: Fix domain deploy overwriting DKIM KeyFile with empty value
- Sendry Web: Fix Public API not finding servers (GetByDomain missing deployments)
- Sendry MTA: Fix DKIM signer not reloading when domain config updated via API
- Sendry MTA: Fix DKIM provider not available for domains added via API after startup
- Sendry MTA: Add persistence for domain configurations (domains.yaml)

## [0.4.5] - 2026-01-29

### Added
- Sendry MTA: IP filtering for API and SMTP (allowed_ips config option)
- Sendry Web: Central DKIM management at /dkim (list, create, view, delete, deploy)
- Sendry Web: Central Domain management at /domains (list, create, edit, view, delete)
- Sendry Web: Domain deployment tracking with config hash for change detection
- Sendry Web: Domain sync functionality to update outdated deployments
- Sendry Web: Domain import from server to central configuration
- Sendry Web: Navigation updated with Domains and DKIM menu items
- Ansible: sendry-web installation and configuration support
- Ansible: Caddy reverse proxy with automatic Let's Encrypt certificates
- Ansible: web-only deployment mode (sendry_mta_enabled: false)
- Ansible: API load balancer with health checks and IP whitelist
- Ansible: allowed_ips support for SMTP and API

### Changed
- Sendry Web: DKIM keys can now be managed centrally at /dkim
- Sendry Web: Domains can now be managed centrally at /domains
- Sendry Web: Per-server DKIM and domain pages remain for server-specific views
- Sendry Web: Moved Servers, Domains, DKIM to Settings page (Infrastructure section)
- Sendry Web: Simplified main navigation menu
- Sendry Web: Added unit tests for DomainRepository (23 tests)
- Sendry Web: Added unit tests for DKIMRepository (11 tests)
- Sendry Web: Added unit tests for Domain model (6 tests)
- Ansible: added ACME on_demand option support (v0.3.4 feature)
- Ansible: added recipient domain rate limiting support (v0.4.3 feature)
- Ansible: updated Molecule test version to 0.4.5

## [0.4.4] - 2026-01-27

### Fixed
- Security: HTTP header injection via Content-Disposition filename in template export, recipient export, and sandbox message download
- Security: Unbounded memory consumption in recipient CSV export (now uses streaming)

## [0.4.3] - 2026-01-27

### Added
- Sendry MTA: Recipient domain rate limiting (limit emails to gmail.com, mail.ru, etc.)
- Sendry MTA: DNS check API endpoint (GET /api/v1/dns/check/{domain})
- Sendry MTA: IP/DNSBL check API endpoint (GET /api/v1/ip/check/{ip})
- Sendry MTA: DNSBL list API endpoint (GET /api/v1/ip/dnsbls)
- Sendry Web: DNS Check page (check MX, SPF, DKIM, DMARC, MTA-STS records)
- Sendry Web: IP Check page (check IP against 15 DNSBL services)
- Sendry Web: Domain management UI (create, edit, view, delete domains per server)
- Sendry Web: Domain configuration with mode, DKIM, rate limits, redirect/bcc settings
- Sendry API Client: CreateDomain, UpdateDomain, DeleteDomain, GetDomain methods
- Tests: Comprehensive tests for dnscheck, ratelimit, queue processor packages

### Changed
- Sendry Web: DKIM management moved from Settings to Servers section
- Sendry Web: DKIM keys now accessible via /servers/{server}/dkim
- Sendry Web: Auto-deploy option when creating DKIM keys from server context
- Documentation: Added Sendry Web campaign guide to quickstart (EN/RU)

### Fixed
- Security: Input validation for domain names and DKIM selectors in DNS check API
- Security: Sanitized error messages in IP check API (no internal details exposure)
- Security: Path traversal protection in DKIM and TLS API handlers
- Security: Max limit (1000) for pagination in sandbox and templates API (prevents DoS)
- Security: Anti-relay protection - only configured domains allowed for sending
- Optimization: Pre-compiled regex for DKIM selector validation
- Optimization: Rate limiter cleans up expired counters from BoltDB (prevents db growth)
- Optimization: SMTP backend cleans up expired auth failure records (prevents memory leak)

## [0.4.2] - 2026-01-26

### Added
- Sendry MTA: DKIM key upload endpoint (POST /api/v1/dkim/upload)
- Sendry Web: DKIM key management (generate, store, deploy to servers)
- Sendry Web: DKIM deployment tracking per server
- Sendry Web: DKIM settings page with DNS record display
- Ansible: DKIM playbook for centralized key generation and deployment
- Ansible: Molecule tests for sendry role with multi-host DKIM key sync validation

### Fixed
- Ansible: duration format in defaults (7d -> 168h, 30d -> 720h for Go compatibility)
- Ansible: version fact handling when sendry_version is not 'latest'

## [0.4.1] - 2026-01-26

### Added
- Docker: universal Dockerfile with TARGET build arg for sendry and sendry-web
- Docker: CI image now includes both sendry and sendry-web binaries

### Changed
- Docker: Alpine base image updated to 3.23
- Docker: sendry-web now built statically with musl for Alpine compatibility
- Documentation: Docker Compose guide (EN/RU)
- Documentation: expanded variable substitution docs in sendry-web guide (EN/RU)
- Sendry Web: replaced GrapesJS with Quill + CodeMirror + Editor.js for template editing
- Sendry Web: three editor modes - Visual (Quill), Blocks (Editor.js), Code (CodeMirror)
- Sendry Web: editor preference saved in localStorage

### Fixed
- Sendry Web: template deploy now converts {{var}} to {{.var}} for Go template compatibility
- Config: duration format in example config (7d -> 168h, 30d -> 720h)

## [0.4.0] - 2026-01-25

### Added
- Sendry Web: web interface foundation (project structure, config, SQLite, routing)
- Sendry Web: UI with login, dashboard, embedded templates and CSS
- Sendry Web: template management (CRUD, versioning, deployment tracking)
- Sendry Web: recipient list management (CRUD, CSV import/export, filtering)
- Sendry Web: campaign management (CRUD, variants for A/B testing, variables)
- Sendry Web: job management (send jobs, status tracking, pause/resume/cancel, dry-run)
- Sendry Web: server monitoring (server list, queue, DLQ, domains, sandbox)
- Sendry Web: settings (global variables, users list, audit log)
- Sendry Web: monitoring dashboard with server status overview
- Sendry Web: Sendry API client for server communication
- Sendry Web: background worker for processing send jobs
- Sendry Web: GrapesJS visual HTML editor for email templates
- Sendry Web: template variable substitution (global, campaign, recipient vars)
- Sendry Web: template deployment to Sendry servers via API
- Sendry Web: template version diff view, import/export JSON, test email
- Sendry Web: scheduled jobs support (worker auto-starts jobs at scheduled time)
- Sendry Web: CLI cleanup command (jobs, audit logs, template versions)
- Sendry Web: status tracking for queued items via Sendry API
- Sendry Web: OIDC authentication (Authentik, Keycloak, etc.) with group filtering
- Sendry Web: dark/light theme toggle with localStorage persistence
- Sendry Web: localization support (English and Russian)
- Sendry Web: timezone configuration in settings
- Sendry Web: unit tests for auth, repository, and worker packages
- Packaging: include sendry-web in DEB/RPM/APK packages

## [0.3.4] - 2025-01-25

### Added
- Ansible role for automated deployment
- ACME on-demand mode: port 80 not opened permanently
- CLI commands: `sendry tls renew`, `sendry tls status`

## [0.3.3] - 2025-01-24

### Added
- Configurable auth brute force protection limits (max_failures, block_duration, failure_window)
- Configurable API HTTP server limits (max_header_bytes, read_timeout, write_timeout, idle_timeout)

### Changed
- Auth brute force limits moved from hardcoded constants to configuration
- API HTTP server limits moved from hardcoded values to configuration

## [0.3.2] - 2025-01-24

### Added
- Auth brute force protection (5 failures = 15 min block per IP)
- Email size limit validation (10 MB max)
- HTTP MaxHeaderBytes limit (1 MB)

### Changed
- Bounce message IDs now use UUID instead of predictable suffix
- Queue storage logging now uses structured slog

### Fixed
- SMTP QUIT command errors now logged

## [0.3.1] - 2025-01-24

### Fixed
- Header injection vulnerability via CRLF in custom headers
- JSON encoding errors now logged instead of silently ignored
- SMTP error code parsing using regex instead of substring matching
- Silent recipient filtering now returns error if no valid recipients
- ACME certificate failures now fatal if no valid certificates exist

## [0.3.0] - 2025-01-24

### Added
- Rate limiting tests and documentation
- Dead Letter Queue (DLQ) configuration with max_age and max_count
- Delivered messages retention policy with automatic cleanup
- HTTP API documentation (EN/RU)
- Tests for bounce, domain, smtp, and email packages
- Shared email utility package for domain extraction

### Changed
- API version bumped to 0.3.0
- Refactored extractDomain functions to use shared email package

### Fixed
- Race condition in sandbox sender error simulation
- Silent error handling in rate limiter persistence
- Silent error handling for corrupted messages in queue
- Email address validation in API send handler
- Type assertions cleanup in DLQ handlers
- HTML extraction in sandbox CLI (was TODO)

## [0.2.0] - 2024-01-24

### Added
- Prometheus metrics with persistence
- Metrics server with IP filtering
- Metrics documentation

### Fixed
- Dynamic test certificate generation

## [0.1.1] - 2024-01-23

### Added
- Email templates feature with variables and layouts
- Header rules for email header manipulation
- HTTPS support for API server
- ACME HTTP challenge server on port 80
- Configuration wizard (`sendry init`)
- IP blacklist check command (`sendry dns ip-check`)
- DKIM CLI commands (`sendry dkim generate`, `sendry dkim dns-record`)
- Quickstart documentation (EN/RU)
- Templates documentation (EN/RU)
- Header rules documentation (EN/RU)

### Changed
- License changed to GPL-3.0
- Documentation included in packages
- Removed PTR check from dns check command

### Fixed
- TLS certificate validation at startup
- Release workflow artifact handling

## [0.1.0] - 2024-01-20

### Added
- Initial release
- SMTP server (ports 25, 587) with AUTH support
- SMTPS server (port 465) with implicit TLS
- STARTTLS support
- Let's Encrypt (ACME) automatic certificate management
- DKIM signing for outgoing emails
- HTTP API for sending emails
- Persistent queue with BoltDB
- Retry logic with exponential backoff
- Multi-domain support with modes: production, sandbox, redirect, bcc
- Rate limiting (per domain, sender, IP, API key)
- Bounce handling
- Graceful shutdown
- Structured JSON logging
- CI/CD workflows for testing and releases
- DEB/RPM/APK packaging
- Docker images

[0.4.15]: https://github.com/foxzi/sendry/compare/v0.4.14...v0.4.15
[0.4.14]: https://github.com/foxzi/sendry/compare/v0.4.13...v0.4.14
[0.4.4]: https://github.com/foxzi/sendry/compare/v0.4.3...v0.4.4
[0.4.3]: https://github.com/foxzi/sendry/compare/v0.4.2...v0.4.3
[0.4.2]: https://github.com/foxzi/sendry/compare/v0.4.1...v0.4.2
[0.4.1]: https://github.com/foxzi/sendry/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/foxzi/sendry/compare/v0.3.4...v0.4.0
[0.3.4]: https://github.com/foxzi/sendry/compare/v0.3.3...v0.3.4
[0.3.3]: https://github.com/foxzi/sendry/compare/v0.3.2...v0.3.3
[0.3.2]: https://github.com/foxzi/sendry/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/foxzi/sendry/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/foxzi/sendry/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/foxzi/sendry/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/foxzi/sendry/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/foxzi/sendry/releases/tag/v0.1.0
