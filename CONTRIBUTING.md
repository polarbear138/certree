# Contributing

Certree welcomes bug fixes, tests, documentation, accessibility improvements, and carefully scoped adapters. Use Go 1.27 and Node.js 26. Read README, docs/ARCHITECTURE.md and docs/SECURITY.md first.

Create a focused branch, explain the concrete behavior being changed, and include a regression test for correctness/security changes. Format Go with `gofmt`; format web code with `npx prettier --write src`. Run `go test ./...`, `go vet ./...`, `npm test`, `npm run lint`, and `npm run build`. Deployment/SSH changes also require the Docker lab suite. CI runs race checks on Linux with a C compiler.

Never include `.env`, real certificates with private keys, SSH credentials, mailbox passwords, production endpoints, or raw mail messages in a contribution. Fixtures must be generated locally and ignored. Do not disable trust verification, host-key checks, approval, rollback, or fingerprint comparison to make a test pass. Clearly distinguish test doubles from real transport/integration evidence.

Describe what changed, why, what was tested, and known limits in the pull request. No contributor license agreement is required; contributions are submitted under Apache-2.0. Security issues follow SECURITY.md instead of public issue templates.
