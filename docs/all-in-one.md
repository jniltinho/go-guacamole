# All-in-one image

One container with **guacd**, **goguacamole** and **Redis**, started by supervisord, built on the official
`guacamole/guacd` image. It is the quickest way to run GoGuacamole on a single host; the regular image
(`ghcr.io/jniltinho/go-guacamole`) stays single-process for Kubernetes and Compose stacks with separate services.

| | Regular image | All-in-one |
|---|---|---|
| Processes | goguacamole | guacd + goguacamole + Redis (supervisord) |
| guacd | separate container | inside, `127.0.0.1:4822` |
| Redis (job queue) | separate service (`redis` in `docker-compose.yml`) or none | inside, `127.0.0.1:6379`, no persistence |
| Use for | Compose / Kubernetes, replicas | one host, demos, small installs |

## Quick start

The published image (`ghcr.io/jniltinho/go-guacamole`, tags `allinone`, `X.Y.Z-allinone`, `X.Y-allinone`) starts in
seconds, with nothing to build:

```bash
docker run -d --name goguacamole -p 8080:8080 -v goguacamole-data:/data ghcr.io/jniltinho/go-guacamole:allinone
docker logs goguacamole 2>&1 | grep 'bootstrap administrator'   # generated admin password, printed once
```

Open <http://localhost:8080>, sign in as `admin` and choose a new password. Use `-v "$PWD/data:/data"` instead of the
named volume to keep the database and recordings in a folder, and `-e APP_BASE_URL=https://guac.example.com` (plus
the other [options](#options)) as with any `docker run`. To update: `docker pull`, then remove and run the container
again (the data stays in the volume).

### Compose project

The Compose project in [`docker/`](../docker/README.md) keeps everything in that folder (`./data` for SQLite,
recordings and RDP drives; `./mariadb` or `./postgres` if you switch databases; `./nginx` for the reverse proxy):

```bash
git clone https://github.com/jniltinho/go-guacamole.git && cd go-guacamole/docker
docker compose up -d --build
docker compose logs goguacamole | grep 'bootstrap administrator'   # generated admin password, printed once
```

Open <http://localhost:8080>, sign in as `admin` and choose a new password.

### From the latest release (fast, no build)

[`docker/Dockerfile.release`](../docker/Dockerfile.release) builds the same container from the latest GitHub release
instead of the source: it downloads `goguacamole_<version>_linux_amd64.tar.gz`, checks it against the release's
`checksums.txt` and copies the binary onto `guacamole/guacd` with the same supervisord setup. It takes seconds and
needs no Go or Node.js.

```bash
cd go-guacamole/docker            # only this folder is used as the build context
docker compose -f compose.release.yml up -d --build
RELEASE=v0.4.1 docker compose -f compose.release.yml up -d --build    # a specific release
docker build -f Dockerfile.release --build-arg VERSION=v0.4.1 -t goguacamole:allinone .   # without Compose
```

`latest` is resolved when the image is built: rebuild with `--no-cache` to move to a newer release. The image prints
its release with `docker run --rm --entrypoint goguacamole goguacamole:allinone-release version`.

## Building the image

The image is the `allinone` target of the repository's `Dockerfile`: the same frontend and Go build (with checks and
tests) as the regular image, then the binary, `docker/allinone/supervisord.conf` and `docker/allinone/entrypoint.sh`
are copied onto `guacamole/guacd`. Building it does not change the regular image.

**make** (repository root) — tags `ghcr.io/jniltinho/go-guacamole:<version>-allinone` and `:allinone`:

```bash
make docker-allinone                      # VERSION defaults to git describe
make docker-allinone IMAGE=registry.example.com/goguacamole VERSION=1.2.3
```

**docker build:**

```bash
docker build --target allinone \
  --build-arg VERSION=$(git describe --tags --always) --build-arg GIT_COMMIT=$(git rev-parse --short HEAD) \
  -t goguacamole:allinone .
```

**Several architectures** (Buildx), pushed to a registry:

```bash
docker buildx build --target allinone --platform linux/amd64,linux/arm64 \
  -t registry.example.com/goguacamole:allinone --push .
```

**Compose** (`docker/`): `docker compose up -d --build` builds it as `goguacamole:allinone`.

To use another guacd release, change the `FROM guacamole/guacd:...` line of the `allinone` stage.

## Running without Compose

```bash
mkdir -p data
docker run -d --name goguacamole --restart unless-stopped \
  -p 8080:8080 -v "$PWD/data:/data" \
  -e APP_BASE_URL=http://localhost:8080 -e APP_ADMIN_PASSWORD='Change-me-2026!' \
  goguacamole:allinone
```

`/data` holds the SQLite database, `secret.key`, `recordings/` and `drive/`. Run CLI commands as the `guacd` user (uid
1000), the owner of those files:

```bash
docker exec -u guacd goguacamole goguacamole user passwd admin
docker exec goguacamole supervisorctl status          # guacd, redis, goguacamole
```

## Inside the image

| Piece | Where |
|---|---|
| guacd (official build) | `/opt/guacamole/sbin/guacd`, `127.0.0.1:4822` |
| goguacamole | `/usr/local/bin/goguacamole serve`, port 8080 |
| Redis | `redis-server`, `127.0.0.1:6379`, no persistence |
| supervisord | `/etc/supervisord.conf` |
| entrypoint | prepares `/data` (owner uid 1000), then starts supervisord |

All three programs run as `guacd` (uid 1000); the entrypoint starts as root only to fix the ownership of a freshly
created `/data`. Logs of all three go to the container's stdout.

Redis needs no persistence: work that matters (uploads owed, deletions pending) is in the database and is queued again
by the reconciliation job after a restart.

## Options

Every [configuration](configuration.md) variable works as usual. Specific to this image:

| Variable | Default | Meaning |
|---|---|---|
| `REDIS_EMBEDDED` | `true` | `false`: do not start the embedded Redis; set `REDIS_ADDR` to an external one |
| `GUACD_ADDR` | `127.0.0.1:4822` | the embedded guacd |
| `REDIS_ADDR` | `127.0.0.1:6379` | the embedded Redis |
| `GUACD_LOG_LEVEL` | `info` | guacd log level |
| `DB_DSN` / `RECORDING_PATH` / `APP_SECRET_KEY_FILE` | under `/data` | database, recordings and secret key locations |

**Several replicas** must share one Redis (and one database and one recordings volume): `REDIS_EMBEDDED=false` and the
same `REDIS_ADDR` everywhere. See [operations](operations.md#job-queue-redis).

## Database, reverse proxy, upgrades

- MariaDB / PostgreSQL, nginx in front, client IPs behind a proxy: [`docker/README.md`](../docker/README.md).
- Upgrade: `docker compose pull && docker compose up -d --build` (migrations run at start).
- Back up the whole `docker/` folder; stop the stack first for a consistent SQLite copy.
