# Updating an existing installation

Source updates do not require deleting containers, database volumes, or installation
secrets. Certree has no in-process automatic source updater. `git pull` retrieves
source; the Docker image still needs to be rebuilt and the application replaced.

## Existing ZIP installation (including the two-server test environment)

For the first update with this tool, extract the new ZIP into a separate source
directory. Leave the original installation directory, `.env`, `testkit/`, generated
trust files, and volumes in place. This avoids overwriting customized proxy settings.
From the new source root run:

```sh
sh scripts/update.sh --container certree-two-server-test-certree-1
```

For a standard installation the container name is usually `certree-certree-1`.
Use the actual existing application container name, never the proxy container.
The first run:

1. Reads the existing Compose project, resolved runtime environment and config files.
2. Recovers both `CERTREE_*` and proxy `TEST_*` variables without printing secrets.
3. Saves the resolved Compose configuration under
   `~/.config/certree/<existing-project>/compose.json`, with directory mode 0700
   and file mode 0600. This file contains installation secrets; back it up securely.
4. Pins `/data` to the existing named volume with `external: true`, or the existing
   bind directory. Missing storage stops the update instead of creating a new DB.
5. Builds the current source, then replaces only the `certree` application service
   in the same Compose project, preserving its key, trust mounts and environment.
   It waits for application health. The proxy and client servers are not restarted.

A small `.local/update-profile.json` remembers the profile location; it contains
no runtime secrets and is excluded from Git, Docker builds and source archives.
After subsequent ZIP source updates in this new source directory, retain its
`.local/update-profile.json` and run:

```sh
sh scripts/update.sh
```

No manual environment recovery is needed. To save the profile without updating:

```sh
sh scripts/update.sh --container certree-two-server-test-certree-1 --capture-only
```

If the original Compose file references host variables not present in container
environments, provide its original environment file with `--env-file /path/to/file`.
The original Compose files must exist at capture time. Existing bind-mounted CA
and configuration files must remain at their absolute paths after capture. Running
as the same Linux account keeps the saved profile location consistent.

The command builds before replacing the running application. A build failure leaves
it running. A failure after container replacement is reported; there is no automatic
rollback of application images or database schema. Back up the DB and encryption key
using the installation backup procedure before upgrading a production installation.

## Switch from ZIP to Git once

Changes must first be committed and pushed to the remote repository. Local edits
made by a coding assistant are not automatically published. Once the desired
revision is available remotely, clone into a separate directory so existing
installation files and bind mounts remain available:

```sh
cd /home/ubuntu
git clone https://github.com/polarbear138/certree.git certree-source
cd certree-source
sh scripts/update.sh --container certree-two-server-test-certree-1
```

The captured installation uses the source directory where the update script runs
as its new build context. It continues to use the original DB and absolute trust
mounts. Keep the original installation directory if it contains mounted CA/config
files. For each later update:

```sh
cd /home/ubuntu/certree-source
sh scripts/update.sh --pull
```

This uses `git pull --ff-only`, refuses to pull over modified tracked source files,
then builds and replaces only the app. It does not switch branches or reset files.
You can also run `git pull --ff-only` separately, then `sh scripts/update.sh`.
The update script never runs `down -v`, creates a new encryption key, or changes
proxy port bindings.

For a Git checkout already in use, capture the profile once in that checkout;
cloning again is unnecessary.

## Tree Explorer operations

Tree checkboxes select deployment targets. Parent checkboxes select their matching
filtered descendants, including collapsed branches; they show mixed selection when
only some descendants are selected. “Select matching targets” applies to the current
search/status result. Selections outside the current filter remain selected and are
counted explicitly. Clear selection removes them. A batch supports up to 200 targets.

Review shows each target's newest approved version, SHA-256 fingerprint, expiry and
blocking reason. Submitting queues only ready targets with the exact reviewed version
IDs. The server revalidates approval, trust, hostname, agent scope and target activity
at submission, and workers recheck approval/validity before execution. Partial
submission failures remain selected; accepted targets receive durable job IDs and
appear in History → Deployments. Waiting jobs are `Queued`; one job per server is
promoted to `Pending` at a time. Existing active-server uniqueness remains enforced.
Queued and Pending jobs can be cancelled.

Expiry bars use the last successfully verified applied version (`active_version`),
not the latest uploaded bundle. Their fill is the remaining fraction of total
certificate lifetime; text shows days until expiry. Warning color follows the
configured expiry warning period. Missing deployment and missing metadata display
separate states without fabricated progress. Server summaries show the earliest
known applied expiry and also count undeployed and unknown targets. A known expiry
is not evidence that currently failing TLS verification is healthy.
