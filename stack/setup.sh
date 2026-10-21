#!/bin/bash
# Prepare what the test stack needs that cannot be checked in: a local CA and
# a certificate per Federation hostname, a federation keypair per Domain, the
# shared clock offset, and an empty data directory per Domain. Each API creates
# the schema in its own database file when it first starts (ADR 19), and the
# suite adds the rows a story needs, so there is no seed.
#
# Idempotent: what exists is kept, so `../down.sh --reset` is how the Domains
# start over. Needs only openssl. ../up.sh runs it.

set -euo pipefail
cd "$(dirname "$0")"

DOMAINS=(103mail.com example.com mail.acme.test acme.test peer.test)
CA_KEY=certs/webmail-local-ca.key
CA_CRT=certs/webmail-local-ca.crt

mkdir -p certs keys

# ── A local CA, which every Domain trusts through extraCaFile ───────────────
if [[ ! -f "$CA_CRT" ]]; then
    echo "creating the stack's CA"
    openssl req -x509 -newkey rsa:4096 -sha256 -days 3650 -nodes \
        -keyout "$CA_KEY" -out "$CA_CRT" \
        -subj "/CN=Webmail end-to-end CA/O=webmail-e2e" \
        -addext "basicConstraints=critical,CA:TRUE,pathlen:0" \
        -addext "keyUsage=critical,keyCertSign,cRLSign" 2>/dev/null
fi

# ── A certificate per Federation hostname (RFC 9.2) ─────────────────────────
for d in "${DOMAINS[@]}"; do
    host="webmail.$d"
    [[ -f "certs/$host.crt" ]] && continue
    echo "creating the certificate for $host"
    openssl req -newkey rsa:2048 -nodes \
        -keyout "certs/$host.key" -out "certs/$host.csr" \
        -subj "/CN=$host" 2>/dev/null
    openssl x509 -req -in "certs/$host.csr" -days 825 -sha256 \
        -CA "$CA_CRT" -CAkey "$CA_KEY" -CAcreateserial \
        -extfile <(printf 'subjectAltName=DNS:%s\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\n' "$host") \
        -out "certs/$host.crt" 2>/dev/null
    rm -f "certs/$host.csr"
done
# Read by the containers' unprivileged users.
chmod 644 certs/*.key certs/*.crt

# ── An Ed25519 federation keypair per Domain (RFC 4.1) ──────────────────────
# domain/*/federation.json names these files. The peer descriptor is rendered
# by the server from the key, so the published key and the signing key cannot
# drift apart.
for d in "${DOMAINS[@]}"; do
    key="keys/$d.ed25519.pem"
    [[ -f "$key" ]] && continue
    echo "creating the federation keypair for $d"
    openssl genpkey -algorithm ed25519 -out "$key" 2>/dev/null
    chmod 644 "$key"
done

# ── The shared clock offset, which the suite moves forward ──────────────────
mkdir -p clock
[[ -s clock/offset ]] || echo 0s > clock/offset

# ── A data directory per Domain, for its database and attachment bytes ──────
for d in "${DOMAINS[@]}"; do
    mkdir -p "data/$d/attachments" "data/$d/documents"
done
