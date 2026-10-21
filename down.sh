#!/bin/bash
# Stop the test stack in stack/. With --reset, also delete every Domain's
# database and attachments and the clock offset, so the next ./up.sh starts
# from empty Domains. Certificates and keys are kept either way.

set -euo pipefail
cd "$(dirname "$0")"

docker compose -f stack/docker-compose.yml down --remove-orphans
if [[ "${1:-}" == "--reset" ]]; then
    rm -rf stack/data stack/clock
fi
