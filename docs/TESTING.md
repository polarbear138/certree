# Testing and release gates

## Local Go tests

`go test ./...` covers parsing, DER/PEM/PKCS#12/ZIP formats, wrong keys, invalid periods, wildcard constraints, unknown roots, archive traversal/symlinks/size/part limits, encrypted persistence/tampering, version deduplication, authentication/session expiry/CSRF/Origin/rate limiting, explicit approval, job exclusion, many-to-many tree projection, status filtering/aggregation, cancellation, and password rotation.

Real local TLS sockets verify valid and mismatched fingerprints, hostname failures, unknown issuers, and expired service certificates. Real local TLS IMAP tests exercise search/fetch, MIME attachment parsing, import, deduplication, approval boundaries, and backoff. A real in-process SSH/SFTP fixture checks pinned host identities and public-key authentication. Worker tests explicitly replace the deployer with a test double to cover success, failure, rollback, recovery failure, TLS mismatch, and restart state recovery; their TLS verifier still opens real sockets. These are not claims that Nginx commands were run locally.

One test constructs 1,000 target rows and verifies the tree projection without duplicating server/certificate entities. This is a basic scalability check, not a production throughput benchmark.

## Web tests

Vitest checks tree flattening, stable IDs, search ancestry, status filters, and empty results. Playwright starts the compiled Go API with a temporary SQLite database and generated CA, then runs login, organization/environment creation, bundle upload, approval, selection, status/search changes, theme switching, settings changes, responsive layout, and logout. Browser page errors fail the test. Screenshots are generated from actual fixture data.

## Docker integration

Use `compose.lab.yaml` with `--abort-on-container-exit --exit-code-from tests tests`. The fixture container generates a disposable CA, two valid versions, invalid-period versions, SSH identities, and a mail server certificate. The lab Nginx master runs as a service account; OpenSSH validates the generated client key. The local mail fixture accepts SMTP and appends to its TLS IMAP mailbox. It is isolated test infrastructure, not a production mail server.

The integration test sends real SMTP mail, collects it using verified IMAP TLS, verifies approval blocking, registers/pins SSH, queues a deployment, checks the real Nginx file replacement/config-test/reload/live TLS fingerprint and key permissions, manually restores a previous version, and tests configuration failure recovery. It also interrupts real replacement before reload, reopens the database/application, verifies restart recovery, removes a backup to exercise Recovery Failed, and checks an initial SSH failure. A live TLS mismatch is injected by routing the verifier to a separate actual TLS server still serving the original certificate, to model a proxy/LB stale endpoint. All verification results are measured rather than hardcoded. The fixture generator remains alive after initialization so Compose's abort-on-exit behavior waits for the test runner instead of stopping at fixture completion.

The integration build tag fails if no lab is configured; ordinary tests do not silently claim integration success. To compile-check the tagged suite without running transports, use `go test -tags=integration ./internal/app -run '^$'`; that command is explicitly not the integration release gate.

## CI and publication

CI definitions run backend race tests/vet/build, web typechecking/tests/build/audit, Chromium workflows, the Docker lab, Go vulnerability scanning, and Gitleaks. Linux race tests require a C compiler. Workflow definitions do not constitute evidence of a successful CI run. The publication owner must configure private vulnerability reporting and execute all gates before release.
