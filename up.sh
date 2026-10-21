#!/bin/bash
# Bring up the compose stack in ../webmail-deploy (or WEBMAIL_E2E_DEPLOY) and
# wait until every Domain answers. Its setup service makes the certificates,
# keys, clock and databases first, and images that are missing are built from
# the sibling checkouts. Arguments go to `docker compose up`, so
# `./up.sh --build` rebuilds after a change to the server or the client.

set -euo pipefail
cd "$(dirname "$0")"

deploy=${WEBMAIL_E2E_DEPLOY:-../webmail-deploy}

compose=(docker compose -f "$deploy/docker-compose.yml")

"${compose[@]}" up -d --remove-orphans "$@"
# The workers have their healthcheck disabled, which `up --wait` refuses, so
# wait for everything else: the APIs and nginx are what answer.
mapfile -t services < <("${compose[@]}" config --services | grep -v '^worker-')
exec "${compose[@]}" up -d --wait --no-recreate "${services[@]}"
