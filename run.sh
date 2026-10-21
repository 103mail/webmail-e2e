#!/bin/bash
# Run the stories against the compose stack in ../webmail-deploy (./up.sh),
# from a container on its federation network. Arguments go to `go test`:
#
#   ./run.sh                                   # every story not tagged @wip
#   ./run.sh -godog.tags='@M1 && ~@wip'        # one milestone
#
# Plain `go test`, not `go test ./...`: in this local directory mode the
# stories' output streams as they run and is never cached, where a package
# list shows only "ok" for a run that passes.
#
# The workspace is mounted at the same path as on the host, because the steps
# run `docker compose` against the deploy directory and the daemon resolves its
# bind mounts on the host.

set -euo pipefail
cd "$(dirname "$0")"

root=$(cd .. && pwd)
socket=${DOCKER_HOST:-}
socket=${socket#unix://}
[[ "$socket" == /* ]] || socket=/var/run/docker.sock

if ! docker network inspect webmail_federation >/dev/null 2>&1; then
    echo "the test stack is not up: run ./up.sh" >&2
    exit 1
fi

docker build -q -t webmail-e2e:local . >/dev/null

tty=()
[[ -t 1 ]] && tty=(-t)

exec docker run --rm "${tty[@]}" \
    --network webmail_federation \
    --user "$(id -u):$(id -g)" \
    --group-add "$(stat -c %g "$socket")" \
    -v "$socket:/var/run/docker.sock" \
    -v "$root:$root" \
    -v webmail-e2e-cache:/cache \
    -w "$root/webmail-e2e" \
    -e WEBMAIL_E2E_DEPLOY \
    -e WEBMAIL_E2E_MAILSINK \
    -e WEBMAIL_E2E_FAULTS \
    webmail-e2e:local \
    go test "$@"
