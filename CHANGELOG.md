# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Docker: separate Dockerfile.web and docker-compose service for sendry-web
- Documentation: Docker Compose guide (EN/RU)

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

[Unreleased]: https://github.com/foxzi/sendry/compare/v0.3.4...HEAD
[0.3.4]: https://github.com/foxzi/sendry/compare/v0.3.3...v0.3.4
[0.3.3]: https://github.com/foxzi/sendry/compare/v0.3.2...v0.3.3
[0.3.2]: https://github.com/foxzi/sendry/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/foxzi/sendry/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/foxzi/sendry/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/foxzi/sendry/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/foxzi/sendry/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/foxzi/sendry/releases/tag/v0.1.0
