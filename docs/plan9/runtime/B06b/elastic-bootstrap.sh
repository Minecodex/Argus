#!/bin/bash
set -euo pipefail
umask 077
cp /run/secrets/elastic-password /run/plan9/elastic-password
chmod 600 /run/plan9/elastic-password
exec /usr/local/bin/docker-entrypoint.sh "$@"
