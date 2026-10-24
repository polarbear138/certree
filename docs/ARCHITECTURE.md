# Architecture

## Agent transport extension

The central application now also serves `/api/agent/v1`. Linux agents poll outbound HTTPS using individual bearer credentials provisioned with single-use, environment-scoped registration keys. `agents`, `agent_enrollment`, `agent_targets` and `agent_tasks` are additive schema version 2 tables. Existing SSH servers and jobs are unchanged.

Agent targets reuse servers/endpoints/deployment_targets and the per-server active job index. SSH workers exclude agent Deployment jobs but continue independent Verification jobs. Agent tasks progress through `deploy → verify-deploy → done`, or `rollback → verify-rollback → done` after a failed deployment/central verification. The persisted task phase and local backup journal survive central/client restarts. Agent completion cannot set Success: the central worker verifies trusted live TLS first.

Destination paths and Nginx commands are local policy; task payloads contain only a target ID and certificate material. The optional loopback GUI is a separate read-only process. See [AGENT.md](AGENT.md) for configuration, trust, interruption and revocation semantics.

## Domain and storage

`organizations → environments → certificates → certificate_versions` establishes inventory scope. Certificate identity uses the environment and sorted SAN set; renewals retain the same certificate entity and create immutable version rows. The SHA-256 fingerprint deduplicates versions within that entity. Encrypted key material and PEM chains live in separate version-linked tables.

`servers → server_endpoints ↔ certificates`, through `deployment_targets`, models the many-to-many deployment relationship. Each target contains paths and reload policy. Each endpoint contains a verification host, TLS port, and SNI, independently of the server SSH address. Matching server/host/port/SNI endpoints are reused automatically; `endpoint_id` can also refer to one explicitly. A second target for the same endpoint/certificate is rejected. Different certificate entities cannot share managed certificate/key paths on a server.

SQLite has foreign keys, WAL, a busy timeout, and a single pooled connection. Schema migration 1 is idempotent and recorded in `schema_migrations`. Deployment jobs/results and TLS verification results are separate. Partial unique indexes prevent active jobs for the same target or server. These locks are deliberately more conservative than target-only exclusion because Nginx reload affects the whole service.

## Authentication and trust boundaries

The administrator is bootstrapped from installation variables only when the user table is empty. Passwords use bcrypt cost 12. Random session tokens are stored as SHA-256 hashes; sessions expire after eight hours. State-changing requests require both the configured browser Origin and a session-bound CSRF header. Password rotation revokes every session. Health is public and only reports availability/version.

AES-256-GCM encrypts secrets; the base64 master key is supplied outside the database. Existing encrypted data is checked against that key at startup. Database-only theft does not disclose stored secrets. Host/container compromise and database integrity manipulation remain outside this protection.

## Deployment state machine

```text
Pending → Running → Success
                ├→ Failed (no managed files replaced)
                ├→ recovery → Rolled Back
                └→ recovery → Recovery Failed
Pending → Cancelled
Running on restart → Recovering → restoration + real TLS check
```

Jobs validate the bundle and SNI again immediately before work. A pinned SSH connection creates a private job backup directory, copies the old certificate/key/config, fsyncs files, then writes a backup manifest. Only after durable job-stage recording can replacement begin. Private staging directories protect temporary keys before they are populated. Each destination replacement is atomic; the certificate/key pair is not one filesystem transaction. Nginx reload happens only after both files and `nginx -t` succeed.

The independent verifier requires a normal validated TLS 1.2+ handshake and compares the served leaf fingerprint. Reload and verification statuses are stored independently. Live verification retries three times with bounded backoff. Verification-only jobs retry up to three attempts. Deployments are not blindly retried after partial writes; they restore a known backup. Recovery gets a separate deadline and requires a restored TLS fingerprint to be recorded as Rolled Back. A failed job retains failure visibility even when recovery made the old service healthy.

On restart, Running deployments become Recovering; verification jobs return to Pending. The remote ready manifest decides whether a backup is usable. An OS file lock rejects a second process for the same database and releases automatically when its owner exits. Multi-instance/HA support is not implemented; use local storage with working advisory file locks. Successful backup directories are retained for operator retention policies. Manual restore uses a previously approved version through the normal validated deployment pipeline. Job final state, target state, result, and worker audit are committed together; verification evidence must persist before a Success outcome is allowed.

## Collection and background scheduling

Upload parsing is bounded and synchronous. IMAP collection uses verified implicit TLS, read-only mailbox selection, UID fetch with PEEK, sender/subject/date search, and a 100-message window. The deduplication key combines UIDVALIDITY and UID. MIME and archive expansion limits apply before import. Collection never grants approval. Transport errors back off, with a daily cooldown after five failures. Permanent invalid bundles are recorded as rejected events rather than automatically resent forever.

A single worker serializes deployment and collector operations. Every five minutes it queues verification for known approved active versions, prioritizing endpoints with the oldest measurement so shared-server locks do not starve siblings. Shared-server monitoring rotates across cycles. Inventory reads compute expiration status. This favors predictable v0.1 behavior over parallel throughput.

## Tree and web console

Stable IDs derive from entity IDs (`organization:…`, `environment:…`, `certificate:…`, `target:…`). Counts preserve leaf statuses and certificate warning/failure/pending conditions. Parent priority is Failed, Deploying, Pending, Warning, Unknown, Healthy. No endpoint implies Unknown; absence of a measurement never means Healthy.

The server constructs the tree in linear passes over its tables. The React tree flattens expanded nodes and renders at most roughly 30 rows around its scroll viewport. Search keeps ancestors; status filtering removes nonmatching leaves. Arrow keys, Home/End, Enter/Space, `aria-level`, selected/expanded state, and visible labels support keyboard/assistive interaction. Expansion and theme preferences persist locally; secrets do not.

TanStack Query polls tree data every three seconds and lists every five seconds. Mutations invalidate affected cached data. Certificate details support version selection, comparisons, approval, per-target deployment, verification, and history. Higher tree levels offer read-only previews.

## Reviewed bulk deployment and applied expiry

Bulk preview resolves explicit approved version IDs for selected targets. Submission
rechecks each reviewed version and returns per-target results. Durable `Queued` jobs
are covered by the target uniqueness index; promotion admits only the oldest waiting
job per free server into the existing Pending/Running/Recovering server exclusion.
Both SSH and agent workers retain their usual validation and verification paths.
Target expiry metadata is derived from `active_version`; server summaries aggregate
the earliest known applied expiry and retain unknown/undeployed counts.
