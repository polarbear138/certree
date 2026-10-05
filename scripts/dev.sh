#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
if [ ! -f .env.dev ]; then go run ./cmd/setup -dev -out .env.dev; fi
set -a
. ./.env.dev
set +a
export CERTREE_DB=data/dev.db
(cd apps/web && npm ci && npm run build)
go run ./apps/server
