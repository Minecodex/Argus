#!/bin/bash
set -euo pipefail
export SQLCMDPASSWORD="$(cat /run/secrets/sa-password)"
exec /opt/mssql-tools18/bin/sqlcmd -C -b -l 5 -S localhost -U sa -Q 'SET NOCOUNT ON; SELECT 1' -o /dev/null
