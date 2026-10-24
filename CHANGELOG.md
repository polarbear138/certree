# Changelog

## Documentation and maintainer readiness — unreleased

- Rework the English README around the operational problem, implemented scope,
  explicit candidate status, reproducible onboarding and evidence links.
- Add a Korean README and English/Korean first-install, remote-access and
  troubleshooting guides; link installation-preserving Git/ZIP updates.
- Expand contribution, maintenance and roadmap guidance with release evidence,
  security/approval invariants and proposed observable maintenance milestones.
- Summarize current verification scope separately from historical test checkpoints.
  No application submission, program acceptance, audit or new CI success is claimed.

## Agent development preview — unreleased

- Add `cmd/certree`: interactive Settings/Status/History/GUI menu, command equivalents, `-logs`/`--logs` aliases, log following, and systemd worker/optional GUI installation.
- Add environment-scoped, single-use enrollment keys, hashed per-agent credentials, heartbeat inventory and access revocation in the central Settings screen.
- Add outbound HTTPS agent transport with immutable local targets, durable task/result replay, process exclusion, restricted local backup journals, Nginx file replacement/reload/recovery, and independent central TLS verification.
- Keep the SSH transport and existing approval/tree/job flows. Support multiple targets per agent; first local adapter is Nginx only.
- Add protocol/OpenAPI documentation, security and recovery tests, a real HTTPS/file/TLS flow, browser enrollment/revocation coverage, and a Linux real-Nginx integration gate. See docs/VERIFICATION.md for executed evidence.

## 0.1.0 — unreleased candidate

Initial implementation: administrator sessions/password rotation, relational inventory and tree projection, React console, bounded bundle parsing/versioning/approval, encrypted secrets, pinned SSH/SFTP Nginx adapter, recoverable backups and staging, persistent deployment and verification jobs, actual TLS fingerprint verification, IMAP collection and deduplication, monitoring, audit events, OpenAPI documentation, Docker installation/lab, and CI definitions.

Verified locally: backend tests, frontend tests/build, and a Chromium administrator workflow. Screenshots show actual local test inventory. Fixed router pattern conflicts, ancestry/status filtering, missing target server identity, and mobile logout access during verification. Strengthened temporary key staging so private directories exist before files are populated.

Release gates still outstanding: execute the real Docker Nginx/OpenSSH/SMTP/IMAP suite; run Linux race checks and Go vulnerability checks in CI; configure repository private vulnerability reporting and dependency/secret scanning; review dependency/container provenance and target privilege policies. No GitHub release has been published.

See docs/VERIFICATION.md for the precise test evidence and environmental limitations. Future adapters and features are listed separately in ROADMAP.md.
