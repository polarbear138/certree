# Verification report

## Current reading guide — 2026-10-25 (Asia/Seoul)

This file retains historical checkpoints. A passing result describes the revision
and scope at that checkpoint; it is not a blanket claim for all later source.
A successful remote CI run is now recorded below. No independent audit or actual
systemd service installation was verified. The application remains a candidate.

| Area | Latest recorded evidence and limit |
| --- | --- |
| App backend | Local full Go tests/vet and remote Linux race tests passed with Go 1.27.3; earlier blocked attempts remain historical |
| Frontend and UI | Typecheck, 10 unit tests, production build, audit and browser workflows passed in the successful remote run |
| Installation updater | Six Python tests passed using mocked Docker calls; no live Linux update execution from the development workspace |
| Static/API checks | `go vet ./...` and OpenAPI validation passed at the 2026-10-24 checkpoint |
| Real deployment gates | Linux Nginx agent deployment/rollback, Docker Nginx/OpenSSH/mail lifecycle and production-image probe passed remotely; systemd installation, populated-install upgrade and publication/security review remain separate gates |
| This documentation revision | Local Markdown link/anchor and documented-command consistency review; runtime behavior was not changed |

For a release or support application, attach the actual public commit SHA and CI
run links. Historical counts are not current repository-adoption metrics. See
[TESTING.md](TESTING.md) for execution instructions.

## Agent development preview — 2026-10-15 (Asia/Seoul)

Implemented the outbound Linux agent, interactive CLI/commands, systemd worker and optional read-only GUI, central enrollment/revocation UI, scoped bearer API, durable deployment/result/recovery protocol, and additive schema version 2. The existing SSH transport remains supported.

Executed successfully during this change:

- Full `go test ./...` passed, including existing TLS, SSH and IMAP socket fixtures and the new real-HTTPS agent → local file replacement → central live TLS verification flow. The latter substitutes the Nginx command with a fixture that changes a real TLS listener's active certificate.
- Focused final agent/CLI/GUI/protocol tests passed after recovery and security refinements: one-time/expired enrollment, environment and cross-agent isolation, revocation, durable replay, central restart, local recovery, central rollback, credential redaction, HTTPS trust/redirect refusal, and GUI authentication/Host/method checks.
- `go vet ./...` passed. Linux amd64 agent and the Linux `agentintegration` test binary cross-compiled; Linux agent vet passed.
- Frontend TypeScript/production build passed. Vitest: **7 passed**. Chromium administrator workflow: **1 passed**, extended to issue a registration key, enroll an agent, reject key reuse, display heartbeat status, hide the key, revoke access, and reject further polling. Mobile overflow check passed. Screenshots were refreshed from the isolated test application; `artifacts/agents-settings.png` records the new panel.
- OpenAPI 3.1 validated with **36 paths**, including agent bearer security and protocol schemas. The CLI executable's help output was exercised locally.

Initial sandboxed runs encountered denied loopback connections, temporary-file rename restrictions and a frontend temporary-cache error. Re-running with normal local execution permissions succeeded. These earlier sandbox failures are not counted as passing evidence.

Still unverified on this machine: **real Linux Nginx agent integration, actual systemd installation/service execution, Linux race checks and the existing Docker Nginx/OpenSSH/mail suite**. Docker is unavailable and no usable WSL distribution is configured. CI now includes `go test -tags=agentintegration -v ./internal/agent` against installed Nginx, but no remote CI success is claimed. No production server, installation or release was changed. Run these platform gates before production use.

The older report below is historical; its final-backend/browser-rerun limitation is superseded by the successful tests above, while its unexecuted Docker and publication/security gates remain outstanding.

---

## Original candidate report — 2026-10-05 (Asia/Seoul)

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

## Tree bulk deployment, expiry and update profiles (2026-10-24)

- Added backend tests for explicit reviewed versions, approval revalidation,
  duplicate target admission, same-server queue exclusion/FIFO, cancellation,
  and expiry calculated from applied rather than newly uploaded versions.
- `go test ./internal/app` passed in the Windows environment with local socket
  access enabled. A subsequent full `go test ./...` run compiled packages but
  Windows Application Control blocked agent/app test executables; it did not
  complete and is not a substitute for the Linux Nginx integration gate.
- `go vet ./...` passed. Frontend unit tests (10), production build, and the
  isolated History/Tree browser tests (2, using mocked API responses) passed.
- Update profile tests (6) cover preserving project/database/key/trust/host values,
  literal dollar signs in stored config, missing-volume refusal, key conflicts,
  build failure before replacement, and refusing Git pulls over tracked edits.
  These mock Docker calls; a live Linux update was not executed from this workspace.

## Go standard-library security update — 2026-10-25 (Asia/Seoul)

The supplied CI scan reported 13 reachable standard-library vulnerabilities while
using Go 1.27.1. The project minimum and production/lab Go build images now use
1.27.3. GitHub setup-go reads the version from go.mod instead of a second hardcoded
version. The vulnerability scan remains required in the workflow.

Executed locally on Windows/amd64 using Go 1.27.3:

- `go test ./...`: passed, including app and agent packages.
- `go vet ./...`: passed.
- `go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...`: exit 0, zero reachable
  vulnerabilities and zero vulnerable imported packages. One vulnerability remains
  in a required module whose affected code is not called, according to this scan.

Sources: [Go release history](https://go.dev/doc/devel/release) and an example
[reported advisory](https://pkg.go.dev/vuln/GO-2026-6613).
These are local results; the updated remote GitHub workflow, Linux race/real-Nginx
and Docker image/integration execution still require their own recorded runs.

## Complete pipeline validation — 2026-10-25 (Asia/Seoul)

Source: `b55fb747d7f291dc7b1b06fc408f51fa7f68a63e` on the verification branch.
[GitHub Actions run 40154857361](https://github.com/polarbear138/certree/actions/runs/40154857361)
completed with **success**. All five jobs completed successfully:

| Job | Recorded scope |
| --- | --- |
| workflow-lint | actionlint workflow/expression/action-input validation |
| backend | Go formatting/vet, Linux race tests, server/agent builds, actual isolated Nginx agent deployment and rollback, reachable Go vulnerability scan |
| web | Locked npm install, Python updater tests, OpenAPI contract, TypeScript, component tests, production build, dependency audit, compiled-API browser workflows |
| integration | Both Compose configurations, production image build, unprivileged/read-only image startup and healthcheck, Docker Nginx/OpenSSH/SMTP/IMAP lifecycle, diagnostics and cleanup |
| secret-scan | Full-history Gitleaks scan |

The earlier `bdb4855` run failed on Go/npm vulnerabilities and an integration-test
cleanup that closed the restarted DB before later scenarios. Those failures were
reproduced/diagnosed and corrected, not ignored or excluded. Local npm audit found
zero vulnerabilities after compatible patches. The Go scanner reported zero
reachable vulnerabilities; its separate unused-module finding remains documented.

This supersedes older claims that Linux race, the Docker transport suite and the
real Nginx agent integration have never run. It does not establish actual systemd
installation, a real populated-install updater run, an independent security audit,
release artifact provenance or production reliability. No registry image publishing
or deployment to a user's installation was configured or executed.

PR: [#13](https://github.com/polarbear138/certree/pull/13). The verified source and
its evidence are reviewable before master is updated.
