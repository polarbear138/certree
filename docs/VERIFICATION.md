# Verification report — 2026-10-05 (Asia/Seoul)

## Status

This is a **v0.1.0 release candidate**, not a fully verified production release. Source implementation, installation/lab definitions, tests, API contract, and publication documents are present. This report records local validation before the initial source publication. No production deployment or tagged release was performed, and no production server was contacted.

## Executed successfully

- Backend `go test ./...` passed at the checkpoint after parsing/auth/collection/worker/tree/password/cancellation tests were added. The app package reported 56.0% statement coverage at that checkpoint. This figure does **not** cover later process-lock/SSH-fixture tests or the real Nginx adapter through Docker.
- That successful backend checkpoint exercised real local TLS handshakes and fingerprint comparison, trust/hostname/expiry failures, actual TLS IMAP collection, deduplication/backoff, encryption/version persistence, approval/concurrency checks, tree aggregation/many-to-many relationships, a 1,000-target projection, password rotation, and cancellation. Worker deployment failures used explicit deployer doubles while TLS connections remained real.
- Final frontend Vitest/React Testing Library suite: **7 tests passed**, including search/status filtering, flattening, rejected-login rendering, and keyboard expansion/selection. JSDOM scrolling is emulated in test setup because JSDOM lacks the browser method; Chromium performs the actual browser workflow.
- Frontend TypeScript checking and production Vite build passed. The production bundle is approximately 342 kB before gzip.
- Chromium Playwright administrator workflow: **1 test passed** (login → organization/environment → upload → approval → tree selection/search/status filters → light/dark → settings → responsive layout → logout). Screenshots were captured from that real local application and temporary SQLite inventory. The web entry-point refactor initially produced the same bundle; the later pending-job cancellation control and final backend changes require a browser/API rerun. Windows policy prevented that rerun.
- `npm audit --audit-level=high` reported **0 vulnerabilities** for the installed locked web dependency graph.
- OpenAPI **3.1.0 validated**, with **27 paths**, using Swagger Parser.
- Both Compose files and four GitHub configuration/workflow YAML files parse successfully. YAML parsing does not validate Docker behavior or workflow execution.
- The final Go source, ordinary test suite, and integration-tagged suite compile. `go vet ./...` passed. A Linux amd64 server cross-build is included in final build verification; compilation alone does not establish successful Linux/Docker operation.
- Runtime license texts were collected for the Windows/Linux server and web dependency graphs. MIT/ISC/BSD/public-domain runtime notices were retained. Container package and release artifact provenance review remains a publication gate.

## Blocked or not executed

- Docker Engine/Compose are not installed, and WSL has no installed Linux distribution. **The real Nginx/OpenSSH/SMTP/IMAP Docker suite was not executed.** Its test code is compile-checked only. Consequently, actual Nginx replacement/reload, filesystem durability semantics, and rollback against the lab require validation on a supported Docker host.
- Windows Application Control later blocked newly compiled `certree.exe`, Go test executables, and `govulncheck.exe`. A permitted elevated healthcheck attempt was also blocked by the same OS policy. The policy was not disabled or bypassed. **Final backend runtime regression and the latest browser/API rerun remain unverified.** Earlier passing results are not presented as proof that every final change ran.
- `go test -race` cannot run on this machine because a C compiler is absent. CI defines a Linux race gate.
- Go vulnerability scanner compilation succeeded, but OS execution policy prevented the scanner from running. No successful Go vulnerability scan is claimed.
- At the local validation checkpoint, Gitleaks and GitHub Actions had not run in a configured remote repository. Workflow definitions and dependency update configuration are supplied; check the remote runs after source publication. No green CI or comprehensive secret-scan claim is made here.
- Docker image build/start/health and Caddy local HTTPS trust have not been tested on this machine. Compose and Dockerfile definitions still require their integration release gate.

## Defects found and addressed

- A Go HTTP route pattern conflict prevented startup; the fallback route now uses a compatible method pattern and rejects unsupported static methods explicitly.
- Status filtering initially retained healthy descendants of an aggregated failed parent. Both server and client filtering now preserve matching ancestry while removing nonmatching leaves.
- Tree target details initially omitted the registered server ID; the projection now includes it for SSH connection actions.
- Mobile navigation hid the only logout button; the top bar now provides a mobile logout control.
- Vitest initially discovered Playwright tests; its include pattern now scopes unit/component tests to `src`.
- Temporary SFTP key creation originally chmodded a file after open. Staging now first establishes a private directory, then creates/restricts/fsyncs the payload before atomic rename.
- Backup manifests preserve original modes; recovery validates manifest/modes and checks restored TLS.
- Finalization now commits job, target, result, and worker audit together, and verification cannot succeed if its evidence fails to persist.
- A cross-platform OS lock now excludes another process before migration/worker startup. This change and the added SSH transport fixture have been compile/vet checked, but final runtime tests are blocked as described above.
- Final source also applies the initial schema transactionally, uses sortable fixed-precision timestamps, reuses matching endpoints to reject duplicate targets, rotates monitoring fairly across shared-server endpoints, and keeps unchanged root-owned configuration read-only during recovery. These latest safeguards require the pending runtime regression gate.
- IMAP imports now reject invalid/untrusted/expired bundles before persistence; target registration rejects wildcard SNI. The lab fixture service remains alive until the test runner exits, and non-root Nginx temporary directories are placed under its writable fixture directory. These final corrections are compile/static checked and still require runtime integration validation.

## Remaining release gates and risks

Run the Docker lab and all Linux CI checks, repeat the final browser/API workflow on an execution-permitted host, and configure private vulnerability reporting/maintainer contacts before tagging v0.1.0. Review container digests and dependency SBOMs when publishing.

The security model trusts administrators and the management host. v0.1 has no RBAC, OCSP/CRL verification, HA, automatic secret rotation, or automatic remote backup retention. Remote backups contain restricted private keys and require disk encryption/retention controls. Certificate/key replacement is atomic per file; external Nginx reloads/edits must be coordinated. A sampled load-balancer handshake does not prove every backend is updated. See SECURITY.md and docs/SECURITY.md.

## Next development priorities

First complete the release gates and obtain an independent security review. Next add safe inventory/collector editing, secret rotation, retention policies, IMAP pagination, and multiple verification vantage points. Additional web-server adapters should follow only with equivalent real integration coverage.
