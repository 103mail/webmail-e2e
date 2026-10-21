#!/bin/bash
# Stop the compose stack in ../webmail-deploy (or WEBMAIL_E2E_DEPLOY). With
# --reset, also delete every Domain's database and the clock offset, so the
# next ./up.sh rebuilds them from the deploy repo's seed. Certificates, keys
# and attachment bytes are kept either way.

set -euo pipefail
cd "$(dirname "$0")"

deploy=${WEBMAIL_E2E_DEPLOY:-../webmail-deploy}

docker compose -f "$deploy/docker-compose.yml" down --remove-orphans
if [[ "${1:-}" == "--reset" ]]; then
    rm -f "$deploy"/data/*/webmail.db "$deploy"/data/*/webmail.db-wal "$deploy"/data/*/webmail.db-shm
    rm -rf "$deploy/clock"
fi
