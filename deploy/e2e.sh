#!/usr/bin/env bash
# End-to-end runner: starts the compose lab stack (guacd + SSH/VNC/RDP targets) for
# each database driver, runs the Go API/tunnel suite and, optionally, the Playwright suite.
#   ./deploy/e2e.sh all | sqlite | mysql | postgres | browser
set -euo pipefail
cd "$(dirname "$0")/.."

PORT=${E2E_PORT:-18081}
export HTTP_PORT=$PORT HTTPS_PORT=$((PORT + 1)) APP_ADMIN_PASSWORD=${E2E_ADMIN_PASSWORD:-adminadmin1}
export APP_BASE_URL="http://127.0.0.1:$PORT"
COMPOSE=(docker compose -p gg-e2e)

up() { # $1 = driver
  local profiles=(--profile lab)
  case $1 in
    mysql) export DB_DRIVER=mysql DB_DSN='goguacamole:goguacamole@tcp(mariadb:3306)/goguacamole'; profiles+=(--profile mariadb) ;;
    postgres) export DB_DRIVER=postgres DB_DSN='postgres://goguacamole:goguacamole@postgres:5432/goguacamole?sslmode=disable'; profiles+=(--profile postgres) ;;
    *) export DB_DRIVER=sqlite DB_DSN=/data/goguacamole.db ;;
  esac
  "${COMPOSE[@]}" "${profiles[@]}" up -d --build app guacd ssh-target ${2:-}
  for _ in $(seq 1 60); do curl -fs "http://127.0.0.1:$PORT/ready" >/dev/null && return 0; sleep 2; done
  echo "app not ready"; "${COMPOSE[@]}" logs app | tail -50; return 1
}

down() { "${COMPOSE[@]}" --profile lab --profile mariadb --profile postgres down -v --remove-orphans >/dev/null 2>&1 || true; }
trap down EXIT

api() { # $1 = driver
  echo "=== e2e API suite: $1"
  down; up "$1"
  GG_E2E_URL="http://127.0.0.1:$PORT" GG_E2E_ADMIN_PASSWORD="$APP_ADMIN_PASSWORD" go test -tags e2e -count=1 ./e2e/...
}

browser() {
  echo "=== e2e browser suite (Playwright)"
  down; up sqlite "vnc-target rdp-target"
  (cd frontend && GG_E2E_URL="http://127.0.0.1:$PORT" GG_E2E_ADMIN_PASSWORD="$APP_ADMIN_PASSWORD" npx playwright test)
}

case ${1:-all} in
  sqlite | mysql | postgres) api "$1" ;;
  browser) browser ;;
  all) api sqlite; api mysql; api postgres; browser ;;
  *) echo "usage: $0 all|sqlite|mysql|postgres|browser"; exit 2 ;;
esac
echo "e2e OK"
