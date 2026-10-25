#!/bin/sh
# Disposable production-image probe; no host ports or persistent volumes.
set -eu
image=${1:?Usage: sh scripts/ci-image-smoke.sh IMAGE}
container="certree-ci-smoke-$$"
cleanup() {
  docker logs "$container" 2>/dev/null || true
  docker rm -f "$container" >/dev/null 2>&1 || true
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
if [ "$(docker image inspect --format '{{.Config.User}}' "$image")" != '10001:10001' ]; then
  echo 'Production image must use the unprivileged certree user' >&2
  exit 1
fi
docker run -d --name "$container" --network none --read-only \
  --tmpfs /data:rw,noexec,nosuid,size=32m,uid=10001,gid=10001,mode=0700 \
  --tmpfs /tmp:rw,noexec,nosuid,size=32m,mode=1777 \
  -e CERTREE_MASTER_KEY="$(openssl rand -base64 32)" \
  -e CERTREE_ADMIN_USER=ci-admin \
  -e CERTREE_ADMIN_PASSWORD=ci-image-smoke-password-only \
  -e CERTREE_ORIGIN=https://localhost:8443 \
  "$image"
attempt=0
while [ "$attempt" -lt 30 ]; do
  if docker exec "$container" certree -healthcheck; then
    echo 'Production image healthcheck passed'
    exit 0
  fi
  if [ "$(docker inspect --format '{{.State.Running}}' "$container")" != true ]; then
    echo 'Production image exited before becoming healthy' >&2
    exit 1
  fi
  attempt=$((attempt + 1))
  sleep 1
done
echo 'Production image healthcheck timed out' >&2
exit 1
