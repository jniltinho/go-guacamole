#!/bin/sh
# GoGuacamole all-in-one (docker/README.md): prepares the bind-mounted /data, then supervisord runs guacd, Redis and
# goguacamole as the guacd user (uid 1000).
#
#   REDIS_EMBEDDED   true (default): run Redis inside the container; false: use the external REDIS_ADDR
#   GUACD_LOG_LEVEL  guacd log level (default info)
set -eu

# ./data on the host may be new (owned by root): recordings, drive and the database live there
mkdir -p /data/recordings /data/drive
[ "$(stat -c %u /data)" = 1000 ] || chown 1000:1000 /data
for d in /data/recordings /data/drive; do
  [ "$(stat -c %u "$d")" = 1000 ] || chown 1000:1000 "$d"
done
# guacd writes recordings group-readable; goguacamole reads them as the same user
chmod 2770 /data/recordings

export GUACD_LOG_LEVEL="${GUACD_LOG_LEVEL:-info}"
case "${REDIS_EMBEDDED:-true}" in
  false | 0 | no) export REDIS_AUTOSTART=false ;;
  *) export REDIS_AUTOSTART=true REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:6379}" ;;
esac

exec /usr/bin/supervisord -c /etc/supervisord.conf
