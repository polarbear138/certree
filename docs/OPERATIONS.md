# Installation and operations

## First installation

Run `go run ./cmd/setup` once, restrict `.env` access, and run `docker compose up -d --build`. The default reverse proxy is loopback only. Trust the exported public Caddy CA for local use or configure an organization-issued HTTPS certificate and matching Origin. Data persists in `certree_data`. The initial account is created only on an empty database. Rotate its password in Settings; updating the bootstrap environment variable does not change an existing account.

For a custom trust root, mount the operator's PEM trust bundle read-only into the API service and set `CERTREE_TRUST_BUNDLE` to that container path. Certificates from that CA must still include a matching key and valid chain. Never copy a private CA key into the API container.

## Updating an existing installation

Use the [installation-preserving update command](UPDATING.md). Capture the existing
application container once; later updates can pull source and rebuild only the app
without reconstructing environment variables or changing the database volume.

## Nginx target prerequisites

Use a dedicated SSH account with public-key authentication. Configure verified host-key pinning, SFTP, fsync and POSIX rename extensions. Its writable certificate and private key paths must be on filesystems supporting atomic rename. The target configuration path must be the main Nginx configuration, since the adapter uses `nginx -c` for both test and reload. Nginx must be on the SSH account's PATH.

Certree requires existing working certificate, key, and configuration files for a recoverable baseline. It does not create an initial Nginx site. Path ancestors must not be symlinks or writable by group/others. Existing private keys must exclude group/other access. The SSH account requires read access to old files and write access to certificate/key destination directories and backups. Normal deployment never edits Nginx configuration; recovery skips an unchanged configuration, allowing it to remain read-only/root-owned. Restoring an externally changed configuration needs suitable write privileges; otherwise recovery fails explicitly. Backups and staging directories require chmod support. Restrict external edits/reloads during work.

For a non-root Nginx master, the account can run the fixed commands directly, as demonstrated by the isolated lab. For a root master, an operator should provide an exact narrow sudo policy for its main configuration, then enable the target's sudo flag. Review arguments and path permissions carefully: write access to a root-parsed arbitrary Nginx config can confer broad privileges. Use a dedicated target service/configuration and an organizationally reviewed policy.

## Verification endpoints and restore

SSH host and TLS verification host are configured separately. Set a concrete domain as SNI, never a wildcard literal. A trusted handshake is required even for a fingerprint match. Test both internal and external endpoints when a load balancer or proxy can serve a different certificate.

Deploying a certificate creates a persistent job. Success requires real activation. Rolled Back means the requested deployment failed but the previous files, Nginx config/reload, and restored fingerprint were verified. Recovery Failed requires immediate manual investigation. Job details include stages and TLS evidence; no private key or remote configuration content is shown.

Select a previous approved, still-valid version to restore it through the ordinary pipeline. Expired or untrusted historical versions cannot be manually deployed. Queued and Pending jobs can be cancelled via the API. Running jobs complete or recover; killing a process leaves the durable backup for restart recovery.

## Backup and restore

Stop the single API worker before filesystem-level backups. Back up the entire SQLite data volume, including its WAL when present, or use an operator-controlled consistent SQLite backup. Protect and back up `CERTREE_MASTER_KEY` separately, plus Caddy trust state if retaining local HTTPS trust. Restart with the same key; a key that cannot decrypt stored data causes startup failure.

Remote Nginx backup directories are retained under `backup_path/certree-<job ID>`. Do not remove the backup of any Pending, Running, or Recovering job. Once historical jobs are terminal and retention requirements are met, operators can remove obsolete backup directories using their established secure maintenance process. Certree does not perform automatic retention deletion.

## IMAP operations

Use a dedicated least-privilege TLS mailbox or application token. Configure From/Subject filters to reduce irrelevant messages. Filtering is not a trust decision. Complete bundles require cryptographic validation before email import and a separate administrator approval before deployment. Invalid email bundles are rejected in collection history; manual uploads may retain invalid versions for inspection. Certificate-only mail needs manual combining with its matching key; automatic association with previously stored keys is future work. Mail bodies/attachments are not exposed in collection history. Errors trigger bounded backoff and a daily cooldown after five unsuccessful polls; Collect now resets the next poll time.

## Process limits

One API/worker process per database is required. There is no HA. The worker serializes jobs, mail collection, and monitoring; long remote timeouts can delay polling. Tune infrastructure connectivity rather than bypassing verification. Readiness checks only test the database and API; they do not imply managed services are healthy.
