#!/bin/sh
# Runs beside the API in its network namespace. No DB password, socket or records mount.
set -eu
umask 077
state=${UKINFO_MONITOR_STATE:-/state}
origin=${UKINFO_PUBLIC_ORIGIN:?Set the public origin}
interval=${UKINFO_MONITOR_INTERVAL:-60}
minimum=${UKINFO_MIN_FREE_GIB:-20}
max_age=${UKINFO_BACKUP_MAX_DAYS:-30}
webhook_file=${UKINFO_ALERT_FILE:-/run/secrets/alert_webhook}
for value in "$interval" "$minimum" "$max_age"; do
 case "$value" in ''|*[!0-9]*) echo 'Monitor thresholds must be integers' >&2; exit 2;; esac
done
[ "$interval" -ge 10 ] && [ "$minimum" -ge 1 ] && [ "$max_age" -ge 1 ]
mkdir -p "$state"
chmod 700 "$state"
# State lives on the same filesystem as the database/backup staging for NAS deployment.
failures=0
previous=unknown
last_delivery=0
previous_total=0
previous_bad=0
while :; do
 now=$(date +%s)
 health=ok; metrics=ok; disk=ok; backup=ok; traffic=ok; delivery=disabled
 if ! wget -q -T 5 -O /dev/null --header="Host: ${origin#*://}" http://127.0.0.1:8080/api/health; then health=failed; fi
 if wget -q -T 5 -O "$state/metrics.tmp" http://127.0.0.1:9091/metrics; then
  counts=$(awk '/^ukinfo_query_requests_total\{/ {line=$0; sub(/.* /,"",line); total+=line; if ($0 ~ /status="503"/) bad+=line} END {printf "%.0f %.0f",total,bad}' "$state/metrics.tmp")
  total=${counts% *}; bad=${counts#* }
  delta_total=$((total-previous_total)); delta_bad=$((bad-previous_bad))
  if [ "$delta_total" -ge 20 ] && [ "$delta_bad" -ge 5 ] && [ "$((delta_bad*100))" -ge "$((delta_total*10))" ]; then traffic=failed; fi
  previous_total=$total; previous_bad=$bad
 else metrics=failed
 fi
 rm -f "$state/metrics.tmp"
 available=$(df -Pk "$state" | tail -1 | awk '{print $4}')
 case "$available" in ''|*[!0-9]*) disk=failed; available=0;; esac
 [ "$available" -ge "$((minimum*1024*1024))" ] || disk=failed
 backup_epoch=0
 if [ -r "$state/backup-epoch" ]; then read -r backup_epoch < "$state/backup-epoch"; fi
 case "$backup_epoch" in ''|*[!0-9]*) backup_epoch=0;; esac
 if [ "$backup_epoch" -eq 0 ] || [ "$((now-backup_epoch))" -gt "$((max_age*86400))" ]; then backup=stale; fi
 current=healthy
 if [ "$health" != ok ] || [ "$metrics" != ok ] || [ "$disk" != ok ] || [ "$traffic" != ok ]; then current=critical
 elif [ "$backup" != ok ]; then current=warning
 fi
 if [ "$current" = healthy ]; then failures=0; else failures=$((failures+1)); fi
 if [ -s "$webhook_file" ]; then
  delivery=idle
  # Two consecutive bad samples; one recovery notification; repeat after 30 minutes.
  if { [ "$failures" -ge 2 ] && { [ "$current" != "$previous" ] || [ "$((now-last_delivery))" -ge 1800 ]; }; } || { [ "$current" = healthy ] && [ "$previous" != healthy ] && [ "$previous" != unknown ]; }; then
   printf '{"service":"UK-Info","status":"%s","time":%s,"health":"%s","metrics":"%s","disk":"%s","backup":"%s","timeouts":"%s"}\n' "$current" "$now" "$health" "$metrics" "$disk" "$backup" "$traffic" > "$state/alert.tmp"
   url=$(cat "$webhook_file")
   case "$url" in https://*|http://127.0.0.1:*) ;; *) echo 'Alert URL must use HTTPS' >&2; exit 2;; esac
   if wget -q -T 10 -O /dev/null --header='Content-Type: application/json' --post-file="$state/alert.tmp" "$url" 2>/dev/null; then
    delivery=sent; last_delivery=$now; previous=$current
   else delivery=failed
   fi
   rm -f "$state/alert.tmp"
  fi
 fi
 printf '{"time":%s,"status":"%s","health":"%s","metrics":"%s","disk":"%s","available_kib":%s,"backup":"%s","timeouts":"%s","delivery":"%s"}\n' "$now" "$current" "$health" "$metrics" "$disk" "$available" "$backup" "$traffic" "$delivery" > "$state/status.json.tmp"
 chmod 600 "$state/status.json.tmp"
 mv "$state/status.json.tmp" "$state/status.json"
 printf '%s\n' "$now" > "$state/heartbeat"
 chmod 600 "$state/heartbeat"
 [ "${1:-}" != --once ] || break
 sleep "$interval"
done
