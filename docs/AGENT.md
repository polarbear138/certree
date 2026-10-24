# Certree Linux agent — development preview

Certree now supports two deployment transports: the existing central SSH/SFTP adapter and a Linux client that connects outbound to the central server over HTTPS. The central Go API, React console and SQLite database remain the host; no second management application is required. This addition is a development preview, not a verified production release.

The first local adapter supports **Nginx only**. Apache, Tomcat, WebtoB and other WAS adapters are not selectable until their implementations and integration tests exist. One agent can manage multiple certificate targets. Registered targets are immutable in this version; `settings` adds targets rather than editing existing paths.

## Install the client

Build on Linux with the repository's Go version, or cross-compile for the target architecture:

```sh
go build -o certree ./cmd/certree
# Example cross-build:
GOOS=linux GOARCH=amd64 go build -o certree-linux-amd64 ./cmd/certree
```

Copy the binary to the target Linux server. The client contains no central web server, SQLite inventory or SSH credentials. On a systemd host:

```sh
sudo install -m 0755 ./certree /usr/local/bin/certree
sudo certree settings
sudo certree service install
sudo systemctl enable --now certree-agent
```

Alternatively, `sudo sh scripts/install-agent.sh ./certree` installs the executable and worker unit; complete Settings before starting the service. The installer does not download binaries or change existing certificates.

The default state directory is `/var/lib/certree-agent` (0700). Config, agent credential, logs and backup payloads are stored with mode 0600. Use `--state-dir /absolute/path` consistently if overriding it. The service uses `/usr/sbin/nginx`; currently it runs as root to access the explicitly configured certificate/key and signal the existing Nginx master. The local adapter executes argument arrays directly, never a command string received from the central server.

## Connect and configure

1. In the central console, create an organization/environment and upload the certificate bundle.
2. Open **Settings → Client agents**, select the allowed environment, and create a registration key. It is single-use and expires in 15 minutes.
3. On the target server, run `sudo certree` and choose **Settings**, or run `sudo certree settings` directly.
4. Enter the central HTTPS origin, an optional locally provisioned trusted CA PEM file, the agent name and the registration key. On a Linux terminal, key input is hidden. TLS certificate validation is mandatory; there is no insecure bypass option.
5. Select a certificate from the assigned environment and the Nginx adapter. Enter existing certificate, private-key and main configuration paths, plus the hostname/IP, port and SNI that the central server should verify.
6. The CLI checks the existing certificate/key pair, paths and permissions, and runs `nginx -t`. Review the configuration and enter `yes` to register it. This does not deploy or replace a certificate.
7. Start the worker. The target appears in the central inventory tree. Approve a valid certificate version and queue deployment from that tree.

For the bundled Caddy installation, securely copy and verify its public root CA on the agent, then select that file during Settings. Do not pass a private CA key. The central server also needs to trust the service certificate issuer through its existing trust configuration.

The configured verification endpoint must be reachable **from the central server**. The agent only needs outbound HTTPS to the central server; it does not listen for deployment requests. The first enrollment exchanges the short-lived key for a per-agent bearer credential; the central database stores credential hashes, not their plaintext values.

To add another target, stop `certree-agent`, run Settings, then restart it. The process lock prevents settings changes or another worker while the service is running. Target registration is idempotent: if a request is interrupted after saving locally, rerunning Settings or starting the worker retries it.

## Menu and commands

```text
Certree Agent
1. Settings
2. Status
3. History
4. Install Client GUI
0. Quit
```

```sh
certree settings
certree status
certree logs
certree -logs                   # compatibility alias; --logs also works
certree logs -f
certree logs --tail 100
certree history                 # local execution results
certree history --remote        # central, TLS-verified outcomes
certree run                     # foreground worker; systemd normally runs this
certree service install
certree gui install
certree version
certree help
```

Quitting the menu does not stop the background service. `status` reports disconnected when the last successful worker check is older than 90 seconds. Operational logs rotate at approximately 4 MiB, retaining one previous file. Local `Reloaded` means the local reload finished; only the central history can report verified deployment `Success`.

## Optional client GUI

The GUI is included in the same binary; `gui install` installs a separate optional systemd unit. It does not install the central management server.

```sh
sudo certree gui install
sudo systemctl enable --now certree-gui
sudo cat /var/lib/certree-agent/gui-token
```

Visit `http://127.0.0.1:9473`. The username is `certree`; the password is the locally generated value in `gui-token`. For a remote machine, use SSH port forwarding (`ssh -L 9473:127.0.0.1:9473 user@server`) and open the loopback URL on your workstation. The listener is always loopback-only, checks the Host header, requires authentication and exposes only status and local history. It is read-only. CLI operation does not depend on installing it.

## Deployment, recovery and disconnection

The existing per-server active-job exclusion covers both transports. An agent polls approximately every ten seconds; errors back off to one minute. Each task contains a local target identifier, certificate payload and expected fingerprint, never an executable command or destination path. The agent resolves paths exclusively from its protected local configuration.

Before changing files, it validates the new key pair, hostname, lifetime and fingerprint. Central approval and trust validation happen before dispatch. It stores original files and modes in a restricted per-job directory and fsyncs the backup and journal before atomically replacing each file. Replacement is atomic **per file**, not across the pair. It then runs Nginx validation and reload. If a local operation fails after backup, it immediately attempts to restore the old files using a fresh bounded context.

Job results are durable locally. A repeated delivery returns the saved result without another deployment. An interrupted deployment with no durable result restores its backup rather than blindly deploying again. The central server keeps agent jobs across its own restart; it does not attempt SSH recovery on them. Central verification checks the live trusted TLS handshake, hostname and exact approved fingerprint. Failure queues a durable rollback task; the central server verifies the restored fingerprint before recording `Rolled Back`.

An offline agent's dispatched job remains Running and blocks conflicting jobs until it reconnects and reports. It is deliberately not timed out and reissued to another worker. Pending, undispatched jobs can be cancelled. Revoking access in Settings immediately rejects future authenticated requests, but cannot stop a task already downloaded. Revocation during a running job can therefore require manual inspection/recovery. Re-enrollment and credential rotation/migration are not automated in this preview; retain the previous state/backup directory when investigating recovery.

Do not delete the state directory while a job is unresolved. There is no automatic backup retention or unattended agent update yet. Keep backups under the same storage-encryption and retention controls as managed private keys. External configuration edits and reloads must be coordinated with Certree.

## Verification

```sh
go test ./...
go vet ./...
# On Linux with /usr/sbin/nginx installed:
go test -tags=agentintegration -v ./internal/agent
```

Ordinary tests cover enrollment/reuse/expiry, scoped inventory, cross-agent isolation, revocation, durable polling/results, local replay prevention, failure/interruption recovery, HTTPS trust and redirect rejection, and a combined real HTTPS + file replacement + live TLS workflow with a substituted reload command. The Linux-specific test uses a real isolated Nginx master to verify deployment and rollback fingerprints. CI includes that gate; adding the workflow does not establish that it has run.

See [the verification report](VERIFICATION.md) for executed checks and remaining platform gates. The existing real SSH/Docker integration suite remains a separate release gate.
