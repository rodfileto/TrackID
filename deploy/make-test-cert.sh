#!/usr/bin/env bash
# A self-signed certificate for trying the stack out: browsers will warn about it, and it proves
# nothing about the server. NOT FOR PRODUCTION: there, put the institute's certificate (full
# chain) and key in ./certs as tls.crt and tls.key.
#
#   ./make-test-cert.sh trackid.test
set -euo pipefail

NAME="${1:?usage: $0 <host name>}"
DIR="$(dirname "${BASH_SOURCE[0]}")/certs"
mkdir -p "$DIR"
openssl req -x509 -newkey rsa:3072 -sha256 -days 30 -nodes \
  -keyout "$DIR/tls.key" -out "$DIR/tls.crt" \
  -subj "/CN=$NAME" -addext "subjectAltName=DNS:$NAME,DNS:localhost,IP:127.0.0.1" 2>/dev/null
chmod 600 "$DIR/tls.key"
echo "self-signed certificate for $NAME in $DIR (30 days; for testing only)"
