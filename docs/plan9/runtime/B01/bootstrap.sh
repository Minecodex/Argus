#!/bin/sh
set -eu
systemd-machine-id-setup
ssh-keygen -A >/dev/null
printf 'PLAN9_PEER=%s\nPLAN9_RUN=%s\n' "$PLAN9_PEER" "$PLAN9_RUN" > /etc/plan9.env
chmod 600 /etc/plan9.env
exec /sbin/init
