# Contributing to Certree

Contributions are welcome in certificate validation, reliable deployment/recovery,
Linux integration, installation documentation and accessibility. Start with the
[README](README.md), [architecture](docs/ARCHITECTURE.md),
[security assumptions](docs/SECURITY.md) and [maintenance guide](docs/MAINTAINING.md).
Participation follows the [Code of Conduct](CODE_OF_CONDUCT.md).

## Choose a focused change

For a bug, provide the revision, operating system, expected/actual result and a
minimal reproduction using disposable resources. Public
[GitHub Issues](https://github.com/polarbear138/certree/issues) are for ordinary
bugs and feature discussion. Follow [SECURITY.md](SECURITY.md) for vulnerabilities.

Discuss new transport/adaptor or schema/permission changes before undertaking a
large implementation. Useful starting areas include a reproducible Linux release
gate, upgrade regressions, accessible tree selection and clearer operating guides.
There is no fixed review or response SLA.

## Set up locally

Use Go pinned by `go.mod`, Node.js 26 and Python 3 for update-tool tests. Native
Linux work on the deployment adapter also needs Nginx; Docker Engine/Compose v2
runs the isolated transport lab. Go race tests on Linux require a C compiler.

```sh
sh scripts/dev.sh
```

On Windows:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/dev.ps1
```

These helpers keep development configuration/data separate from Compose
installation data. For Vite HMR, set the API's development `CERTREE_ORIGIN` to
`http://127.0.0.1:5173`, run the API, and run `npm run dev` in `apps/web`.

## Validate the behavior changed

From the repository root:

```sh
gofmt -w apps/server cmd internal
go test ./...
go vet ./...
python3 -m unittest discover -s tests -p test_update.py
node scripts/validate-openapi.mjs
```

Then in `apps/web`:

```sh
npm ci
npm test
npm run lint
npm run build
```

Use [TESTING.md](docs/TESTING.md) for browser tests, the real Docker lab, agent
integration and Linux race checks. Deployment/SSH changes require the real
transport gate, not only a passing test double. API changes update
`scripts/generate-openapi.py` and regenerate `docs/openapi.json`.
For document-only changes, check links, examples and implementation consistency;
a full runtime suite is not required solely for prose edits.

## Keep credentials and approval boundaries intact

Never commit `.env`, installation profiles, certificate private keys, SSH or agent
credentials, mailbox passwords, production endpoints or raw mail messages. Use
locally generated fixtures and sanitize screenshots/logs. Do not disable trust,
host-key checks, approval, queue exclusion, recovery or fingerprint comparison to
make a test pass. AI-assisted contributions follow the same review/evidence rules;
assistant output is not a substitute for executed tests.

## Submit a pull request

Create a focused branch. Explain the concrete problem and resulting behavior;
include a regression test for correctness/security changes. Format Go with
`gofmt`; format changed web code with `npx prettier --write <changed-files>`.
Use the repository PR template to describe actual validation and operational
implications, including migration, trust, queue/recovery and compatibility changes.

Name blocked or unexecuted tests and distinguish real integration evidence from
mocked APIs or test doubles. Link the revision and applicable CI runs. Small,
reviewable changes help maintainers assess sensitive certificate operations.

No contributor license agreement is required. Contributions are made under the
repository's Apache-2.0 license; preserve applicable third-party notices.
