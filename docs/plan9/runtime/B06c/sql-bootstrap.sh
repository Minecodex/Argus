#!/bin/bash
set -euo pipefail
export MSSQL_SA_PASSWORD="$(cat /run/secrets/sa-password)"
exec /opt/mssql/bin/launch_sqlservr.sh "$@"
