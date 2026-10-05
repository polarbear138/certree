# Roadmap

## Before v0.1.0 release

- Pass the real Docker integration suite, Linux race checks, Go dependency scan, and repository secret scan.
- Review Nginx permissions/reload policies and perform independent security review.
- Configure GitHub private security reporting and maintainer contacts.
- Pin production container digests during release, generate release SBOMs, and publish verified binaries/images.

## Next releases

- Inventory/collector editing and deletion with safe reference checks; key and credential rotation.
- Explicit stale/partial deployment reporting and multi-vantage/load-balancer verification.
- Automatic backup retention with recovery-aware safeguards; distributed worker ownership before multi-instance use.
- IMAP cursor pagination beyond the newest 100 messages, OAuth authentication, certificate-only renewal matching against securely stored keys, mail/inventory quotas, and password-protected mail bundle workflows. Password/application-token TLS IMAP authentication already exists.
- Accessibility audit, tree pagination for larger fleets, notifications, and richer audit searches.
- Apache, HAProxy, Tomcat, WebtoB, and IIS adapters, added only with transport-level integration tests.
- RBAC, PostgreSQL, HA, Kubernetes integration, ACME issuance, and reviewed bulk deployment policies.

No roadmap item is claimed as implemented support in this candidate.
