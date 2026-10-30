# GoGuacamole all-in-one (Docker)

One container with **guacd**, **goguacamole** and **Redis** (supervisord), built on the official `guacamole/guacd`
image. Everything lives in this folder:

| Path | Contents |
|---|---|
| `./data` | SQLite database, `secret.key`, `recordings/`, `drive/` (RDP file transfer) |
| `./mariadb`, `./postgres` | database files, only if you switch from SQLite |
| `./nginx` | reverse proxy config (`goguacamole.conf`) and `certs/` |

## Quick start

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

## Building the all-in-one image

The image is the `allinone` target of the repository's `Dockerfile`: it builds the frontend and the Go binary (with
the same checks and tests as the regular image) and copies the binary, the supervisord config and the entrypoint
(`docker/allinone/`) onto the official `guacamole/guacd` image. The regular `docker build .` is not affected.

**With Compose** (this folder): `docker compose up -d --build` builds it as `goguacamole:allinone`.

**With make** (repository root): tags `ghcr.io/jniltinho/go-guacamole:<version>-allinone` and `:allinone`:

```bash
make docker-allinone                   # VERSION defaults to git describe; IMAGE=registry/name to change the name
```

**With docker build** (repository root):

```bash
docker build --target allinone \
  --build-arg VERSION=$(git describe --tags --always) --build-arg GIT_COMMIT=$(git rev-parse --short HEAD) \
  -t goguacamole:allinone .
```

**Other architectures** (for example an ARM server), with Buildx:

```bash
docker buildx build --target allinone --platform linux/amd64,linux/arm64 \
  -t registry.example.com/goguacamole:allinone --push .
```

**Run without Compose:**

```bash
mkdir -p data
docker run -d --name goguacamole --restart unless-stopped \
  -p 8080:8080 -v "$PWD/data:/data" \
  -e APP_BASE_URL=http://localhost:8080 -e APP_ADMIN_PASSWORD='Change-me-2026!' \
  goguacamole:allinone
```

What the image contains and how it starts:

| Piece | Where |
|---|---|
| guacd 1.6.0 (official build) | `/opt/guacamole/sbin/guacd`, listening on `127.0.0.1:4822` |
| goguacamole | `/usr/local/bin/goguacamole serve`, port 8080 |
| Redis | `redis-server` on `127.0.0.1:6379`, no persistence (off with `REDIS_EMBEDDED=false`) |
| supervisord | `/etc/supervisord.conf` (`docker/allinone/supervisord.conf`) |
| entrypoint | `docker/allinone/entrypoint.sh`: prepares `/data` (owner uid 1000), then starts supervisord |

All three programs run as `guacd` (uid 1000); the entrypoint starts as root only to fix the ownership of a freshly
created `./data`. To change versions, edit the `FROM guacamole/guacd:...` line of the `allinone` stage.

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
