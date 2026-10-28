#!/bin/sh
set -eu
mkdir -p /run/plan9 /var/lib/plan9
if [ "$PLAN9_ROLE" = probe ]; then
    cp /pki/ca.crt /usr/local/share/ca-certificates/plan9-b02b.crt
    update-ca-certificates >/dev/null
    systemd-machine-id-setup
    ssh-keygen -A >/dev/null
    exec /sbin/init
fi
printf '%s app.b02b.test\n' "$PLAN9_DNS_ANSWER" > /var/lib/plan9/dns-hosts
printf 'normal\n' > /var/lib/plan9/tls-variant
exec /usr/bin/supervisord -n -c /etc/plan9-supervisord.conf
