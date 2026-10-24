# Getting started with Certree

This walkthrough covers a new evaluation installation. For an existing DB or
installation use [UPDATING.md](UPDATING.md), not the setup command. The candidate
is not a completed production release; read [SECURITY.md](../SECURITY.md) and the
[verification report](VERIFICATION.md) before connecting non-disposable targets.

[한국어](GETTING_STARTED.ko.md) · [README](../README.md)

## Prerequisites

| Component | Requirement |
| --- | --- |
| Management host | Linux with Docker Engine and Compose v2; permission to use Docker |
| Setup helper | Go version pinned by `go.mod`; `go run ./cmd/setup` generates local secrets |
| Client browser | A current browser able to trust the management HTTPS certificate |
| SSH targets | POSIX Nginx with pinned OpenSSH identity and SFTP durability extensions; see [operations](OPERATIONS.md) |
| Agent targets | Linux/Nginx, outbound HTTPS to management, and reachable verification endpoints; see [agent guide](AGENT.md) |
| Update tool | Python 3 and Docker Compose v2 with `up --wait`; Git for `--pull` |

The Docker build installs the Node frontend build dependencies. Native frontend
work requires Node.js 26 and `npm ci` in `apps/web`.

## Install and sign in

```sh
git clone https://github.com/polarbear138/certree.git
cd certree
go run ./cmd/setup
docker compose up -d --build
docker compose ps
```

The setup helper creates `.env` once with a random 32-byte encryption key and
initial administrator password. It does not overwrite an existing file. Keep
`.env` private and back up the encryption key separately. Do not publish it in an
issue, support transcript, source archive or program application.

On the installation host, open **https://localhost:8443**. Caddy initially uses
its internal CA. Export only the public root certificate:

```sh
docker compose cp proxy:/data/caddy/pki/authorities/local/root.crt ./certree-local-root.crt
```

Verify and import that root using your operating system/browser certificate tools,
or provision your organization's trusted management certificate. Read the initial
credentials from `.env` locally, sign in, and change the password in Settings.
The bootstrap password does not reset an account in an existing database.

## Remote access

There are two separate controls: host firewall access and Docker's listening
address. The default Compose mapping is `127.0.0.1:8443:443`; opening a firewall
port does not change that loopback binding.

For a managed remote installation:

1. Choose the hostname operators will use, for example `certree.example.com`,
   and arrange DNS to the intended management interface.
2. In `compose.yaml`, change `services.proxy.ports` to publish on that interface.
   `8443:443` publishes on all host interfaces; scope network access to your
   intended administrators using your infrastructure's access controls.
3. Set `.env` to `CERTREE_ORIGIN=https://certree.example.com:8443`.
4. Change the site address in `deployments/docker/Caddyfile` to the same hostname
   on Caddy's **container** port, not host port 8443:

   ```caddyfile
   https://certree.example.com {
       tls internal
       reverse_proxy certree:8080
   }
   ```

5. Apply environment/port changes and restart the proxy to load its file:

   ```sh
   docker compose up -d
   docker compose restart proxy
   ```

6. Provision trust for the management certificate on the remote workstation and
   any enrolled agents. Visit `https://certree.example.com:8443`.

For IP-only access, replace the hostname consistently with the actual address and
set Caddy's default TLS server name for clients that omit SNI:

```caddyfile
{
    default_sni 203.0.113.10
}

https://203.0.113.10 {
    tls internal
    reverse_proxy certree:8080
}
```

`203.0.113.10` is a documentation example; replace it. Match `CERTREE_ORIGIN` to
`https://YOUR_ACTUAL_IP:8443`. Internal-CA trust must still be provisioned.
Caddy documents [default SNI behavior](https://caddyserver.com/docs/caddyfile/options#default-sni).
These are management HTTPS settings, distinct from the CA used to verify managed
service certificates.

## First certificate workflow

1. In Tree Explorer, create an organization and environment.
2. In Certificates, import a complete certificate/private-key bundle into that
   environment. Certree retains versions and reports validation findings.
3. Review the certificate's names, validity, key match and trust before approving
   the version. Importing a CA inside a bundle does not make it a trusted root.
4. Register a pinned SSH server and deployment target, or enroll an agent through
   Settings and follow [AGENT.md](AGENT.md). Targets need existing recoverable
   Nginx certificate, key and configuration files. Registration is not deployment.
5. Specify a concrete verification hostname/SNI and port reachable from the
   **management server**, even when deployment travels over the outbound agent.
6. Deploy one target, or check multiple targets and choose **Review deployment**.
   The preview identifies each approved version and blocked target. Submitting
   deploys only ready targets with the reviewed version IDs.
7. Inspect **History → Deployments** for per-target jobs and recovery evidence.
   A local reload is not final success; the central trusted TLS fingerprint check
   must pass. Jobs on the same server run one at a time.
8. Use **History → Verification** for recurring checks. The default tab shows
   verification jobs; it does not indicate repeated certificate replacement.

If the service certificate uses a private issuer, the operator must provision a
read-only trust bundle for the management API and set `CERTREE_TRUST_BUNDLE` to
its container path. The management proxy CA and service issuer may be different.

## Reading expiry and queue state

- A target's expiry bar uses its last successfully verified applied version.
  Uploading a newer bundle does not change that applied expiry.
- Server expiry is the earliest known applied target expiry. Counts retain unknown
  and undeployed targets; a partial summary is not evidence of fleet-wide health.
- Bar length is remaining lifetime divided by full certificate lifetime. Text
  gives days left, and the warning color uses the configured warning window.
- `Queued` waits for the server, `Pending` is admitted for execution, and `Running`
  is in progress. Queued/Pending jobs can be cancelled; running work may recover.
- Checkbox selection survives filtering. The selection count states when hidden
  targets remain selected; clear selection if changing the intended scope.

## Troubleshooting without losing data

| Symptom | Check or action |
| --- | --- |
| `port is already allocated` | `docker ps --filter publish=8443`; identify the old proxy before starting another project |
| Remote browser cannot connect | `docker ps` published address, `ss -ltnp`, host/cloud firewall and the intended management address |
| TLS protocol error | Actual Caddy site address/default SNI and `docker compose logs --tail=80 proxy`; test using the configured hostname, not an unrelated loopback hostname |
| Certificate trust warning | Provision the public management CA root or a trusted certificate; do not confuse trust failure with an application login error |
| Login rejects Origin | Match scheme, hostname and port in `CERTREE_ORIGIN` to the browser URL and recreate the app |
| Empty inventory after an update | Compare Compose project and `/data` mount; a new volume may be a second installation, not deleted data |
| Missing `CERTREE_MASTER_KEY` or `TEST_HOST_IP` | Use the existing-container update tool to capture application and proxy runtime settings; do not generate a replacement key |
| UI still shows older features | Confirm the running app image was rebuilt and replaced, then refresh the browser |

Read-only diagnostics:

```sh
docker ps -a --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
docker compose ps
docker compose logs --tail=80 certree proxy
```

Review logs for secrets and infrastructure identifiers before sharing them. Do not
run broad container/volume prune commands or `down -v` against installation data.
Backups, recovery and privilege requirements are covered in [OPERATIONS.md](OPERATIONS.md).

## Next steps

Follow [UPDATING.md](UPDATING.md) for Git-based updates,
[TESTING.md](TESTING.md) for a disposable integration lab, and
[CONTRIBUTING.md](../CONTRIBUTING.md) to help complete release gates.
