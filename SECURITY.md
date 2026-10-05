# Security policy

The v0.1.0 candidate has not received an independent security audit. Production acceptance requires passing the Docker integration release gate and your own review of target privileges and the documented threat model.

Report vulnerabilities privately using the repository's GitHub **Report a vulnerability** feature once the maintainer enables private vulnerability reporting. Until that channel is enabled, request a private reporting channel without posting exploit details, credentials, keys, or production hosts in public issues. A maintainer contact address has not yet been configured; configuring private reporting is a publication checklist item.

Include affected versions, a minimal isolated reproduction, impact, and expected/observed behavior. Do not send real private keys or passwords. Supported fixes initially target the latest v0.1 release line. A response SLA has not been established.

Operational assumptions and residual risks are documented in [docs/SECURITY.md](docs/SECURITY.md). Do not expose the management API to the public internet or deploy to production before validating the integration suite and your least-privilege configuration.
