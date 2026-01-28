# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Sendry MTA: IP filtering for API and SMTP (allowed_ips config option)
- Ansible: sendry-web installation and configuration support
- Ansible: Caddy reverse proxy with automatic Let's Encrypt certificates
- Ansible: web-only deployment mode (sendry_mta_enabled: false)
- Ansible: API load balancer with health checks and IP whitelist
- Ansible: allowed_ips support for SMTP and API

### Changed
- Ansible: added ACME on_demand option support (v0.3.4 feature)
- Ansible: added recipient domain rate limiting support (v0.4.3 feature)
- Ansible: updated Molecule test version to 0.4.4

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
