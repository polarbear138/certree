# CI/CD for a self-hosted installation

Certree is installed by each operator. CI runs in disposable GitHub-hosted runners;
it does not connect to installed customer servers or require their certificates,
SSH keys, database, encryption key or mailbox credentials.

## Pipeline contract

The [CI workflow](../.github/workflows/ci.yml) runs on pushes, pull requests and
manual workflow dispatch. Jobs use Ubuntu 24.04, bounded timeouts, a read-only
repository token and commit-pinned actions. New runs cancel superseded runs for
the same branch/PR. Runtime Go follows `go.mod`; production and lab Docker builds
use the same patched Go release.

| Check | What must pass |
| --- | --- |
| workflow-lint | Pinned actionlint checks workflow syntax, expressions and action inputs; runner ShellCheck is used when installed |
| backend | Formatting, vet, race tests, server/agent builds, real isolated Nginx agent deployment/rollback, reachable-vulnerability scan |
| web | Locked npm install, update-tool tests, OpenAPI validation, typechecking, component tests, production web build, dependency audit, real-API browser workflows |
| integration | Production/lab Compose validation, actual production image build, unprivileged/read-only isolated image startup/healthcheck, real Nginx/OpenSSH/SMTP/IMAP lifecycle |
| secret-scan | Full-history Gitleaks with redacted findings; PR comments disabled so fork PRs need no write permission |

Use the actual check names above in a master branch ruleset. Run a branch/PR and
confirm the checks exist before making them required. A deployment-environment
approval gate is not needed for this self-hosted distribution model.

Browser failures retain traces/screenshots as a short-lived GitHub artifact. Lab
logs and cleanup are separate always-run steps, so failure to collect diagnostics
does not prevent cleanup. Production smoke tests expose no host port and use only
temporary in-container data. The lab's `down -v` removes only disposable lab volumes.

## CI is separate from image publishing

There is currently no release-tag publishing workflow, registry login, GHCR push,
remote-host deployment or automatic update of user installations. Validating an
image in CI is not publishing it. The current update mechanism is source retrieval
and local Docker rebuilding; see [UPDATING.md](UPDATING.md).

A future publishing workflow should build versioned images after release gates,
record checksums/provenance and publish to a chosen registry. Operators would then
explicitly select a version and pull it. Registry permissions, signing, supported
architectures and the release policy must be defined before enabling that workflow.
No such publishing has been enabled by this CI review.

## Reproduce the checks

From the repository root:

```sh
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
go vet ./...
go test -race ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
python3 -m unittest discover -s tests -p test_update.py
node scripts/validate-openapi.mjs
```

On Linux with Nginx installed:

```sh
go test -tags=agentintegration -v ./internal/agent
```

From `apps/web`, run locked install, lint/tests/build/audit and the real API browser
workflow as documented in [TESTING.md](TESTING.md). `npm audit --audit-level=high`
remains a failing gate for high/critical findings; it has not been disabled.

On a disposable Docker test host:

```sh
docker build --tag certree-ci-smoke .
sh scripts/ci-image-smoke.sh certree-ci-smoke
docker compose -f compose.lab.yaml up --build --abort-on-container-exit --exit-code-from tests tests
# Always clean this isolated lab after collecting logs:
docker compose -f compose.lab.yaml logs --no-color
docker compose -f compose.lab.yaml down -v --remove-orphans
```

The smoke helper verifies the image's unprivileged user and startup with a read-only
root filesystem, isolated network and tmpfs database. It never mounts installation
volumes. The Docker lab exercises service deployment rather than merely validating
YAML or compiling tests.

## Review findings, 2026-10-25

The previous public run
[40104549925](https://github.com/polarbear138/certree/actions/runs/40104549925),
for source `bdb4855`, failed in three jobs:

- backend: Go 1.27.1 standard-library vulnerabilities;
- web: npm audit found vulnerable Vite, undici and fast-uri versions;
- integration: a restarted DB was closed by a subtest cleanup before subsequent
  lifecycle scenarios, causing an authentication query to fail as session expired.

Corrections raise Go to 1.27.3, patch compatible npm dependencies, retain the
restarted DB through the parent lifecycle test, and report helper failures on the
correct subtest. The workflow also adds the missing production-image probe,
maintained action runtimes, independent named steps and reliable diagnostics.

Local actionlint, web checks/audit and Git-history secret scan passed during the
review. Linux race, real Nginx and Docker execution require the corresponding
remote run; see [VERIFICATION.md](VERIFICATION.md) for the latest observed outcome.
No failed or unexecuted gate should be represented as green solely because this
workflow file exists.
