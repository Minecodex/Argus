#!/bin/sh
set -eu
systemd-machine-id-setup
ssh-keygen -A >/dev/null
printf 'PLAN9_RUN=%s\nPLAN9_NODE=%s\n' "$PLAN9_RUN" "$PLAN9_NODE" > /etc/plan9-web.env
mkdir -p /var/lib/plan9
if [ ! -s /var/lib/plan9/traffic-deadline ]; then
    printf '%s\n' "$PLAN9_TRAFFIC_DEADLINE" > /var/lib/plan9/traffic-deadline
fi
printf 'PLAN9_TRAFFIC_DEADLINE=%s\n' "$(cat /var/lib/plan9/traffic-deadline)" >> /etc/plan9-web.env
chmod 600 /etc/plan9-web.env
exec /sbin/init
