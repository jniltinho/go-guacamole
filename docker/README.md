# GoGuacamole all-in-one (Docker)

One container with **guacd**, **goguacamole** and **Redis** (supervisord), built on the official `guacamole/guacd`
image. Everything lives in this folder:

| Path | Contents |
|---|---|
| `./data` | SQLite database, `secret.key`, `recordings/`, `drive/` (RDP file transfer) |
| `./mariadb`, `./postgres` | database files, only if you switch from SQLite |
| `./nginx` | reverse proxy config (`goguacamole.conf`) and `certs/` |

## Quick start

No build at all, with the published image:

```bash
docker run -d --name goguacamole -p 8080:8080 -v "$PWD/data:/data" ghcr.io/jniltinho/go-guacamole:allinone
```

To build it here instead (from your checkout):

```bash
cd docker
echo 'APP_ADMIN_PASSWORD=Change-me-2026!' > .env   # optional; without it a password is generated (see below)
docker compose up -d --build
```

Open <http://localhost:8080> and sign in as `admin` (you are asked to choose a new password).

Without `APP_ADMIN_PASSWORD`, the first start prints a generated password once:

```bash
docker compose logs goguacamole | grep 'bootstrap administrator'
```

Lost it (for example the container was recreated)? Set a new one:

```bash
docker compose exec -u guacd goguacamole goguacamole user passwd admin
```

Always run commands as `-u guacd` (uid 1000): the files in `./data` belong to that user.

## From the latest release (no build)

`Dockerfile.release` and `compose.release.yml` make the same all-in-one container from a **published release**: the
release tarball is downloaded and checked against its `checksums.txt`, so the image is ready in seconds, with no Go
or Node.js build. SQLite, data in `./data`, `linux/amd64` (the architecture of the release tarballs).

```bash
cd docker
docker compose -f compose.release.yml up -d --build
docker compose -f compose.release.yml logs goguacamole | grep 'bootstrap administrator'
```

- **Pin a release:** `RELEASE=v0.4.1 docker compose -f compose.release.yml up -d --build` (or `RELEASE=` in `.env`).
- **Update to a newer release:** `docker compose -f compose.release.yml build --no-cache && docker compose -f
  compose.release.yml up -d` (`latest` is resolved when the image is built).
- **Without Compose:** `docker build -f Dockerfile.release -t goguacamole:allinone .` (in this folder; add
  `--build-arg VERSION=v0.4.1` to pin).

Use `docker-compose.yml` instead to build from your checkout, or for MariaDB/PostgreSQL and nginx (the same options
apply to both files).

## Building the image and what is inside

See [docs/all-in-one.md](../docs/all-in-one.md): `make docker-allinone`, `docker build --target allinone`, other
architectures with Buildx, running without Compose, the processes inside and the image's options.

## Database

SQLite is the default. For MariaDB or PostgreSQL, edit `docker-compose.yml`: comment the two SQLite lines
(`DB_DRIVER`, `DB_DSN`), uncomment the pair for your database, the database service and `depends_on`, then
`docker compose up -d`. The database files go to `./mariadb` or `./postgres`. Set `DB_PASSWORD` in `.env` to change the
default password.

## Behind nginx

`nginx/goguacamole.conf` proxies HTTPS to goguacamole, upgrades the remote session WebSocket (`/api/v1/tunnel`, no
buffering, 12 h timeouts) and forwards the client's address (`X-Forwarded-For`, `X-Real-IP`, `X-Forwarded-Proto`) and
the request id, which goguacamole logs too.

GoGuacamole uses forwarded addresses **only from trusted proxies**, so its access log, audit log and session history
record the client's IP instead of nginx's, and nobody can forge an address by sending the header directly.

**nginx in this Compose file:** put `fullchain.pem` and `privkey.pem` in `nginx/certs`, set `server_name` in
`nginx/goguacamole.conf`, uncomment the `nginx` service, and in `.env`:

```bash
APP_BASE_URL=https://guac.example.com
APP_TRUSTED_PROXIES=10.249.49.20
APP_COOKIE_SECURE=true
```

**nginx on the host:** change the upstream in `goguacamole.conf` to `127.0.0.1:8080`, bind the port to the host only
(`"127.0.0.1:8080:8080"` in `docker-compose.yml`) and trust the Docker gateway: `APP_TRUSTED_PROXIES=10.249.49.1`.

**nginx behind a load balancer or CDN:** also uncomment `set_real_ip_from` / `real_ip_header` in `goguacamole.conf` so
nginx itself sees the client's address.

The network is `10.249.49.0/24` (goguacamole `.10`, nginx `.20`); if that range is taken on your machine, set
`GG_NET=10.250.50` in `.env` and use the matching addresses above.

## Options

| Variable (`.env`) | Default | Meaning |
|---|---|---|
| `HTTP_PORT` | `8080` | host port |
| `APP_BASE_URL` | `http://localhost:8080` | URL users type (checked as the request origin) |
| `APP_ADMIN_PASSWORD` | generated | first administrator's password |
| `APP_TRUSTED_PROXIES` | empty | proxies whose `X-Forwarded-For` is trusted |
| `RECORDING_RETENTION_DAYS` | `90` | default retention of recordings |
| `REDIS_EMBEDDED` | `true` | `false` to use an external `REDIS_ADDR` |
| `GG_NET` | `10.249.49` | first three octets of the Compose network |
| `TZ` | `America/Sao_Paulo` | time zone |

## Operations

```bash
docker compose exec goguacamole supervisorctl status     # guacd, redis, goguacamole
docker compose logs -f goguacamole                       # all three, on stdout
docker compose pull && docker compose up -d --build      # update
```

Back up the whole `docker/` folder (stop the stack first for a consistent SQLite copy).
