# Maintaining Certree

This is the repository's working maintenance guide. It describes review and release
practices, not a promise of staffing, response times or a completed security audit.
[Contributing](../CONTRIBUTING.md) explains how to submit a change.

## Responsibility and scope

Maintainers with repository write permissions are responsible for merge/release
choices, dependency changes, credential-handling boundaries and published claims.
Repository affiliation and permissions are verified through the hosting platform;
this document does not grant them. Security reporting follows [SECURITY.md](../SECURITY.md).
No response SLA or dedicated security team has been established.

Current work prioritizes Nginx certificate deployment, independent activation
verification and dependable installation updates. New adapters and broader
access-control/distributed architectures need a scoped design and real integration
coverage before they are presented as supported.

## Triage and review

1. Request the revision, platform, minimal reproduction, expected result and
   sanitized diagnostics. Separate support/configuration issues from regressions.
2. Move sensitive findings to the private reporting path. Never request real
   certificate private keys, tokens, mail contents or an installation `.env`.
3. For a change, state the trigger and before/after behavior. Include schema,
   upgrade, queue ownership and rollback implications where applicable.
4. Require evidence proportionate to the behavior changed. Keep deterministic
   tests separate from real SSH, TLS, Nginx and Docker evidence.
5. Review the public diff and actual test results before merging. Record known
   limits instead of treating missing execution as a successful check.

Suggested first contributions are installation troubleshooting, accessible tree
interaction, recovery regression fixtures and repeatable Linux integration runs.
These are suggested areas, not claims that labeled issues or reviewers already exist.

## Invariants to preserve

- No deployment without an approved, currently trusted and valid matching bundle.
- A reviewed bulk version ID is not silently exchanged for a newer one.
- Active work is excluded per server; queued targets and cancellation remain durable.
- A reload is not Success until independent trusted TLS/hostname/fingerprint checks pass.
- Uploaded CA material does not become a trust anchor automatically.
- Metadata and logs do not reveal private keys, credentials or enrollment tokens.
- Updating an installation retains the DB identity, encryption key and required
  trust mounts; missing storage does not silently create a replacement installation.
- Recovery evidence and unknown/partial states stay distinguishable from health.

## Using coding and security assistants

Codex can assist with implementation, reproduction, regression tests, documentation
and review. Maintainers remain responsible for accepting the change and judging
whether verification is adequate. AI-generated test output or a proposed fix is
not independent evidence that the fix ran successfully.

Use locally generated fixtures and sanitized reports. Do not submit production
private keys, installation profiles, mailbox payloads or credential values to
assistant prompts. Security tools must operate only on repositories and disposable
systems the maintainer is authorized to review. API-backed issue/PR automation is
possible future maintenance work, not a current Certree runtime dependency.

## Record evidence against a revision

For each meaningful release candidate record:

- Commit SHA, UTC/declared-local date, OS/architecture and tool versions.
- Exact commands and exit results for Go tests/vet/race, frontend tests/build,
  update-tool tests, browser workflows and API-contract validation.
- Real Nginx/SSH/mail and agent/systemd results separately from test doubles.
- Vulnerability/secret/dependency scan results, scope and unresolved findings.
- Updated screenshots marked as actual fixture data or simulated API data.
- Any unexecuted/blocked checks, reason, and the next verification step.

Use [VERIFICATION.md](VERIFICATION.md) as an append-only historical record plus a
current-status summary. Do not carry a passing result from an older revision
forward as proof that a later change was tested.

## Release and installation handoff

Before tagging a production release:

1. Complete the Linux/Docker gates in [TESTING.md](TESTING.md) and review privilege,
   recovery and dependency risks. Link actual CI runs, not only workflow definitions.
2. Verify the real private vulnerability-reporting channel and maintainer ownership.
3. Review LICENSE/NOTICE, dependency notices, container provenance and release SBOMs.
4. Test clean installation and an upgrade that retains populated inventory,
   encrypted secrets, trust mounts and queued-job identity.
5. Document backup/restore and rollback compatibility. Do not imply automated
   image/schema rollback if none is implemented.
6. Publish revision-specific release notes, supported-platform limits and checksums
   for any distributed artifacts. Generated secrets and local profiles stay out.

The [roadmap](../ROADMAP.md) tracks remaining work.
