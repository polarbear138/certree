# Changelog

## 0.1.0 — unreleased candidate

Initial implementation: administrator sessions/password rotation, relational inventory and tree projection, React console, bounded bundle parsing/versioning/approval, encrypted secrets, pinned SSH/SFTP Nginx adapter, recoverable backups and staging, persistent deployment and verification jobs, actual TLS fingerprint verification, IMAP collection and deduplication, monitoring, audit events, OpenAPI documentation, Docker installation/lab, and CI definitions.

Verified locally: backend tests, frontend tests/build, and a Chromium administrator workflow. Screenshots show actual local test inventory. Fixed router pattern conflicts, ancestry/status filtering, missing target server identity, and mobile logout access during verification. Strengthened temporary key staging so private directories exist before files are populated.

Release gates still outstanding: execute the real Docker Nginx/OpenSSH/SMTP/IMAP suite; run Linux race checks and Go vulnerability checks in CI; configure repository private vulnerability reporting and dependency/secret scanning; review dependency/container provenance and target privilege policies. No GitHub release has been published.

See docs/VERIFICATION.md for the precise test evidence and environmental limitations. Future adapters and features are listed separately in ROADMAP.md.
