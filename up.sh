#!/bin/bash
# Bring up the test stack in stack/, from the images named in
# stack/docker-compose.yml, and wait until every Domain answers. Run it again
# after rebuilding an image to recreate what changed.

set -euo pipefail
cd "$(dirname "$0")"

missing=()
for image in "${WEBMAIL_SERVER_IMAGE:-webmail-server:local}" "${WEBMAIL_CLIENT_IMAGE:-webmail-client:local}" \
    "${WEBMAIL_FAULTS_IMAGE:-webmail-faults:local}" "${WEBMAIL_MAILSINK_IMAGE:-webmail-mailsink:local}"; do
    [[ "$image" == *:local ]] && ! docker image inspect "$image" >/dev/null 2>&1 && missing+=("$image")
done
if (( ${#missing[@]} )); then
    echo "these images are not built yet: ${missing[*]}" >&2
    echo "build them from their repos, or point WEBMAIL_*_IMAGE at a registry (see README.md)" >&2
    exit 1
fi

stack/setup.sh
exec docker compose -f stack/docker-compose.yml up -d --wait --remove-orphans
