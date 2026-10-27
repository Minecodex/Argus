package collectormanager

import "strings"

// Only a validated Collector UUID/revision-derived directory is substituted.
// Private configuration is uploaded over stdin, never interpolated into shell.
func sshActivateCommand(directory string) string {
	return strings.ReplaceAll(`set -eu
stage='__STAGE__'
root=/var/lib/argus-otelcol
cleanup_sensitive() { rm -f "$stage/private.json" "$stage/enrollment-token"; }
trap cleanup_sensitive EXIT
trap 'exit 1' HUP INT TERM
tar -xf - -C "$stage"
command -v flock >/dev/null
command -v curl >/dev/null
IFS= read -r revision < "$stage/revision"
IFS= read -r config_hash < "$stage/config-hash"
IFS= read -r collector_id < "$stage/collector-id"
case "$revision" in ''|*[!0-9]*) exit 85;; esac
mkdir -p "$stage/release" "$root" /etc/argus-otelcol
tar -xzf "$stage/artifact.tar.gz" -C "$stage/release"
test -x "$stage/release/argus-otelcol"
if ! "$stage/release/argus-otelcol" validate --config="$stage/config.json" --config="$stage/private.json" >/dev/null 2>&1; then rm -f "$stage/private.json" "$stage/enrollment-token"; exit 86; fi
exec 9>"$root/.collector-control.lock"
flock -x -w 30 9
owner=''
if [ -f "$root/.active-collector-id" ]; then owner=$(cat "$root/.active-collector-id"); fi
if [ -n "$owner" ] && [ "$owner" != "$collector_id" ]; then exit 85; fi
active_revision=0
if [ -f "$root/.effective-revision" ]; then active_revision=$(cat "$root/.effective-revision"); fi
case "$active_revision" in ''|*[!0-9]*) exit 85;; esac
if [ "$active_revision" -gt "$revision" ]; then exit 85; fi
if [ "$active_revision" -eq "$revision" ] && [ -f "$root/.effective-config-hash" ] && [ "$(cat "$root/.effective-config-hash")" != "$config_hash" ]; then exit 85; fi
backup="$stage/rollback"
mkdir -p "$backup"
chmod 700 "$backup"
save() { if [ -f "$1" ]; then cp -p "$1" "$backup/$2"; fi; }
restore() { if [ -f "$backup/$2" ]; then cp -p "$backup/$2" "$1.restore" && mv -f "$1.restore" "$1"; else rm -f "$1"; fi; }
save /usr/local/bin/argus-otelcol binary
save /etc/argus-otelcol/config.yaml config
save /etc/argus-otelcol/monitoring-private.json private
save /etc/argus-otelcol/server-ca.pem ca
save /etc/argus-otelcol/enrollment-token token
save /etc/systemd/system/argus-otelcol.service unit
save "$root/.active-collector-id" owner
save "$root/.effective-revision" revision
save "$root/.effective-config-hash" hash
was_running=0
if systemctl is-active --quiet argus-otelcol.service; then was_running=1; fi
ready() {
 i=0
 while [ "$i" -lt 90 ]; do
  if systemctl is-active --quiet argus-otelcol.service && test -s /var/lib/argus-otelcol/identity/client.pem && test -s /var/lib/argus-otelcol/identity/client-key.pem && test -s /var/lib/argus-otelcol/identity/ca.pem && test ! -e /etc/argus-otelcol/enrollment-token && curl --fail --silent --max-time 2 http://127.0.0.1:13133/ >/dev/null; then return 0; fi
  i=$((i+1)); sleep 1
 done
 return 1
}
rollback() {
 rc=$?
 trap - EXIT HUP INT TERM
 if [ "$rc" -ne 0 ]; then
  set +e
  systemctl stop argus-otelcol.service >/dev/null 2>&1
  failed=0
  restore /usr/local/bin/argus-otelcol binary || failed=1
  restore /etc/argus-otelcol/config.yaml config || failed=1
  restore /etc/argus-otelcol/monitoring-private.json private || failed=1
  restore /etc/argus-otelcol/server-ca.pem ca || failed=1
  restore /etc/argus-otelcol/enrollment-token token || failed=1
  restore /etc/systemd/system/argus-otelcol.service unit || failed=1
  restore "$root/.active-collector-id" owner || failed=1
  restore "$root/.effective-revision" revision || failed=1
  restore "$root/.effective-config-hash" hash || failed=1
  systemctl daemon-reload || failed=1
  if [ "$was_running" -eq 1 ]; then systemctl restart argus-otelcol.service || failed=1; ready || failed=1; fi
  if [ "$failed" -ne 0 ]; then rc=87; fi
 fi
 rm -rf "$stage"
 exit "$rc"
}
trap rollback EXIT
trap 'exit 1' HUP INT TERM
install -m 0755 "$stage/release/argus-otelcol" /usr/local/bin/argus-otelcol.next
mv -f /usr/local/bin/argus-otelcol.next /usr/local/bin/argus-otelcol
install -m 0600 "$stage/config.json" /etc/argus-otelcol/config.yaml
install -m 0600 "$stage/private.json" /etc/argus-otelcol/monitoring-private.json
install -m 0600 "$stage/server-ca.pem" /etc/argus-otelcol/server-ca.pem
install -m 0644 "$stage/service.unit" /etc/systemd/system/argus-otelcol.service
install -d -m 0700 "$root/identity"
printf '%s' "$collector_id" > "$root/.active-collector-id"
if [ ! -s "$root/identity/client.pem" ]; then test -s "$stage/enrollment-token"; install -m 0600 "$stage/enrollment-token" /etc/argus-otelcol/enrollment-token; else rm -f /etc/argus-otelcol/enrollment-token; fi
systemctl daemon-reload
systemctl enable argus-otelcol.service
systemctl restart argus-otelcol.service
ready
printf '%s' "$revision" > "$root/.effective-revision"
printf '%s' "$config_hash" > "$root/.effective-config-hash"
`, "__STAGE__", directory)
}
