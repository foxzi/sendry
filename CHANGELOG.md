# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Rate limiting tests and documentation
- Dead Letter Queue (DLQ) configuration with max_age and max_count
- Delivered messages retention policy with automatic cleanup
- HTTP API documentation (EN/RU)

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

[Unreleased]: https://github.com/foxzi/sendry/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/foxzi/sendry/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/foxzi/sendry/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/foxzi/sendry/releases/tag/v0.1.0
