#!/bin/sh
# Run inside the production DB container. No automatic pruning.
set -eu
umask 077
cd /backups
mixed=$(psql -XAt -v ON_ERROR_STOP=1 -U ukinfo -d ukinfo -c "SELECT count(*) FROM datasets WHERE COALESCE((manifest->>'sample')::boolean,false) OR key='synthetic-demo' OR COALESCE(manifest->>'edition','')='synthetic-demo' OR position(':sample:' IN key)>0")
[ "$mixed" = 0 ] || { echo 'Remove sample/demo imports before making a production backup' >&2; exit 3; }
name="catalogue-$(date -u +%Y%m%dT%H%M%SZ)-$$.dump"
(set -C; : > "$name.partial")
chmod 600 "$name.partial"
pg_dump -U ukinfo -d ukinfo -Fc --compress=zstd:1 --no-owner --no-acl --file="$name.partial"
pg_restore --list "$name.partial" >/dev/null
mv "$name.partial" "$name"
sha256sum "$name" > "$name.sha256"
chmod 600 "$name.sha256"
# Provision monitor/ with the monitor UID before use. Only an aggregate timestamp is shared.
if [ -d monitor ]; then
 date +%s > monitor/backup-epoch.tmp
 chmod 600 monitor/backup-epoch.tmp
 chown "$(stat -c '%u:%g' monitor)" monitor/backup-epoch.tmp
 mv monitor/backup-epoch.tmp monitor/backup-epoch
fi
printf '%s\n' "$name"
