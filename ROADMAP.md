# Roadmap

Certree is an early v0.1.0 candidate. This roadmap separates shipped source from
proposed work. It is a maintenance plan, not a release-date or funding promise.
Completion requires public, revision-specific evidence rather than an unchecked
feature list. See [verification](docs/VERIFICATION.md) and
[maintenance practices](docs/MAINTAINING.md).

## Current implementation

- Certificate import, validation, version approval and encrypted secret storage.
- Nginx deployment through pinned SSH/SFTP and a preview outbound HTTPS Linux agent.
- Independent trusted TLS fingerprint verification, durable jobs and recovery evidence.
- Tree selection and reviewed bulk deployment with per-server queues.
- Applied-certificate expiry bars and server summaries, separate deployment/verification history.
- Installation-preserving update profiles and optional fast-forward Git source updates.

Implemented source does not imply completion of all platform or production gates.

## Priority 1: reproducible release evidence

| Work | Completion evidence |
| --- | --- |
| Real Docker Nginx/OpenSSH/mail lab | Passing run on the candidate SHA, tool versions and reproducible logs |
| Agent on Linux/Nginx/systemd | Actual installation, deployment, replay, restart and recovery results |
| Race, dependency and secret checks | Revision-specific CI runs, scan scope and dispositions for findings |
| Populated-install upgrade | Inventory, encrypted keys, trust mounts and queue state retained across replacement |
| Security reporting and review | Working private contact path; documented privilege/recovery review and unresolved risks |
| Release provenance | Dependency notices, container provenance, artifact checksums and SBOMs before published binaries/images |

## Priority 2: operational reliability and onboarding

- Safe inventory/collector editing and deletion with reference checks.
- Key and credential rotation, re-enrollment and clear agent-offline handling.
- Stale/partial-deployment reporting and multi-vantage/load-balancer verification.
- Backup retention that respects unresolved recovery jobs.
- IMAP pagination beyond the newest 100 messages, OAuth, certificate-only bundle
  matching and password-protected mail import workflows.
- Accessibility audit, larger-inventory pagination, notifications and richer audit search.

Each change needs a scoped failure model, migration/rollback notes and regression
coverage. TLS IMAP password/application-token authentication already exists.

## Priority 3: additional deployment and ownership models

Apache, HAProxy, Tomcat, WebtoB and IIS adapters need equivalent real transport
integration tests. RBAC, PostgreSQL, HA, Kubernetes and ACME issuance require
separate designs. More advanced bulk policies should follow the current explicit
version review and per-server exclusion rather than weakening either boundary.

## Proposed first 90 days of supported maintenance

The relative periods below are a proposed work sequence for planning support,
not fixed deadlines or work already completed.

| Period | Focus | Observable output |
| --- | --- | --- |
| Days 1–30 | Reproduce Linux gates and close installation gaps | Linked candidate CI/lab results; minimal reproductions and reviewed fixes; refreshed first-install/update guides |
| Days 31–60 | Recovery and upgrade hardening | Regression PRs for interrupted/partial work and populated-install upgrades; reviewed security findings and remaining risks |
| Days 61–90 | Contributor and release readiness | Reproducible release checklist, sanitized operating examples, documented artifact provenance and a release-or-block decision |

Codex would assist reproduction, focused patches, tests and documentation. Optional
API-backed maintenance automation starts as a bounded, measured pilot only if
selected and separately scoped. No runtime AI service is needed by Certree.
Progress should be documented with public revision-specific evidence.
