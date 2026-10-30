#!/usr/bin/env bash
# End-to-end runner: starts the compose lab stack (guacd + SSH/VNC/RDP targets) for
# each database driver, runs the Go API/tunnel suite and, optionally, the Playwright suite.
#   ./deploy/e2e.sh all | sqlite | mysql | postgres | browser | screenshots
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

# Playwright runs in its pinned image (browsers, fonts) so references match on any host
pw() {
  docker run --rm --network host --user "$(id -u):$(id -g)" -e HOME=/tmp -v "$PWD/frontend:/w" -w /w \
    -e GG_E2E_URL="http://127.0.0.1:$PORT" -e GG_E2E_ADMIN_PASSWORD="$APP_ADMIN_PASSWORD" -e GG_VISUAL -e GG_SHOTS \
    mcr.microsoft.com/playwright:v1.64.1-noble npx playwright test "$@"
}

browser() {
  echo "=== e2e browser suite (Playwright)"
  down; up sqlite "vnc-target rdp-target"
  pw
  echo "=== e2e theme suite (axe AAA, focus, login references, remote session)"
  GG_VISUAL=1 pw e2e/themes.spec.ts --project=visual
}

# README screenshots on a clean stack with demo data (written to docs/screenshots)
screenshots() {
  echo "=== README screenshots"
  # the sidebar shows the release being documented (latest CHANGELOG version)
  export VERSION=${VERSION:-$(grep -m1 -oP '^## \[\K[0-9][0-9.]*' CHANGELOG.md)}
  down; up sqlite
  rm -rf frontend/.shots
  GG_SHOTS=1 pw e2e/screenshots.spec.ts --project=chromium
  mv frontend/.shots/*.png docs/screenshots/ && rmdir frontend/.shots
}

case ${1:-all} in
  sqlite | mysql | postgres) api "$1" ;;
  browser) browser ;;
  screenshots) screenshots ;;
  all) api sqlite; api mysql; api postgres; browser ;;
  *) echo "usage: $0 all|sqlite|mysql|postgres|browser|screenshots"; exit 2 ;;
esac
echo "e2e OK"
