# Security design and deployment assumptions

## Protected assets

Certificate private keys, SSH keys, mailbox credentials, administrator passwords, browser sessions, and deployment integrity are the primary assets. Secrets are excluded from inventory/list/detail responses and audit events. Remote Nginx command output is discarded because a server configuration can include credentials. Keys/passwords are not used as log fields.

The management host and administrators are trusted. Registering a target grants network access to its SSH and verification addresses. This is intentionally suitable for internal infrastructure; administrators must control who may register targets. Network policies should limit egress to managed servers/mailboxes. RBAC and multi-tenant isolation are not part of v0.1. Lifetime inventory/mail storage quotas are not implemented; use a dedicated filtered mailbox and monitor disk usage. Invalid mail bundles are rejected before inventory import, but sender spoofing and valid unapproved certificate floods still require operational controls.

## Authentication and browser security

Production sessions require HTTPS Secure, HttpOnly, SameSite=Strict cookies. The backend expects a trusted reverse proxy; do not expose its plaintext listener to untrusted networks. Development HTTP explicitly requires loopback binding. Login requires the configured Origin and is limited to eight attempts per remote address in 15 minutes. Behind the bundled proxy this applies to the proxy address, conservatively limiting the entire console. Forwarded IP headers are not trusted. External distributed rate limiting is future work.

Authenticated mutations also require `X-CSRF-Token`. Session hashes have an eight-hour lifetime. Rotation verifies the old password, hashes a new 14–72 byte password, and deletes all sessions in a transaction. Request bodies, field names, resource IDs, ports, and server type are validated. No default administrator password exists. Header controls include CSP, clickjacking protection, no-store API responses, and HSTS in production mode.

## Certificate handling

Imports require exactly one supported private key and a matching non-CA leaf. The parser identifies content independently of extensions, orders the supplied chain by signatures, and rejects unrelated certificates. Only system roots plus an operator-managed additional trust bundle establish trust; importing a self-signed CA is not approval. Validation includes validity periods, DNS SAN presence, intended hostname when deploying, server-auth EKU, signature chain, and issuer trust.

ZIP imports reject absolute paths, drive prefixes, parent traversal, symlinks, nested ZIPs, oversized entries, excessive entry counts, and bounded aggregate expansion. Inputs are limited to 16 MiB, at most 64 parts/files, and a 32 MiB parser accounting budget that includes archive/source expansion. Files are parsed in memory; multipart overflow files are removed after handling. PKCS#12 passwords are only used during import. Legacy encrypted PEM and password-protected PKCS#12 mail attachments require manual conversion/import.

OCSP/CRL checking, certificate transparency policy, hardware key storage, and master-key rotation are not implemented. TLS handshake trust, hostname, lifetime, and fingerprint verification are mandatory and are never disabled to obtain a successful result.

## SSH and file operations

SSH public-key authentication and a pinned OpenSSH host public key are mandatory. Verify that key using an independent trusted channel. Unknown/changed keys fail the connection. There is no trust-on-first-use or insecure host-key callback.

Remote commands are fixed `nginx -t -c <quoted path>` and `nginx -s reload -c <quoted path>`, optionally prefixed by `sudo -n`. Paths are canonical absolute POSIX paths and shell-quoted. Arbitrary user-supplied commands are not accepted. Dedicated service accounts should own only managed directories and backups, or use narrowly constrained ACLs. Do not grant unrestricted root, shell-wide sudo, or world-writable managed directories.

Key staging directories are mode 0700 before any content is written. Keys are chmod 0600 before atomic rename; backups are mode 0600 inside restricted directories. Existing key files accessible to group/others are rejected. Symlink files, symlink ancestors, and ancestors writable by group/others are rejected. Recovery restores recorded file modes, preserves unchanged read-only configurations, validates Nginx again, reloads, and measures the restored TLS fingerprint.

Deletion unlinks temporary files; it cannot guarantee physical secure erasure on journaling filesystems, SSDs, snapshots, or backups. Use disk encryption. Remote backup retention is operator-managed and backups contain live key material. Interrupted writes can leave restricted staging directories until recovery; failed/unreachable recovery needs operator attention.

## Availability and residual risks

Certificate/key replacement is atomic per file, not atomic as a pair. External administrators must not reload Nginx or edit the managed files during a deployment. A load balancer verification result proves the sampled connection, not every backend or geographic vantage point. Proxy/LB endpoints may need multiple independent registered targets and operational checks.

SQLite and the worker are single-instance only. Start exactly one process per database. Hardware/disk failures during final result writes can leave a job requiring recovery on restart. Maintain consistent encrypted-data backups and a separately protected master key. No independent security audit has been performed. Real Docker/Nginx integration verification is a release gate; consult the current verification report before production use.
