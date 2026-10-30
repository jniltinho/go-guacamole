# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Stage 1: frontend (Vue 3 + Vite+ + Tailwind v4) -> /src/web/dist (architecture independent)
# ---------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM node:24-bookworm-slim AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npx vp check \
    && npm run typecheck \
    && npx vp test run \
    && npx vp build

# ---------------------------------------------------------------------------
# Stage 2: static Go binary with the frontend embedded (built on the build platform)
# ---------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS backend
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG TARGETOS=linux
ARG TARGETARCH=amd64
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
COPY cmd/ cmd/
COPY internal/ internal/
COPY --from=frontend /src/web/dist web/dist
RUN go vet ./... \
    && go test ./... \
    && BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
    && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -o /out/goguacamole \
       -ldflags "-s -w -X github.com/jniltinho/go-guacamole/cmd.Version=${VERSION} -X github.com/jniltinho/go-guacamole/cmd.GitCommit=${GIT_COMMIT} -X github.com/jniltinho/go-guacamole/cmd.BuildDate=${BUILD_DATE}" .

# `make docker-binary` exports only the binary to ./bin (no local Go or Node.js needed)
FROM scratch AS binary
COPY --from=backend /out/goguacamole /goguacamole

# ---------------------------------------------------------------------------
# All-in-one (docker/docker-compose.yml): guacd + goguacamole + Redis in one container under supervisord, on the
# official guacd image. Build with --target allinone; the default target stays the minimal runtime below.
# ---------------------------------------------------------------------------
FROM guacamole/guacd:1.6.0 AS allinone
USER root
# guacd's FreeRDP config dir exists and belongs to guacd, whatever else runs with that HOME (supervisord.conf)
RUN apk add --no-cache supervisor redis ca-certificates tzdata \
    && install -d -o guacd -g guacd /home/guacd/.config /home/guacd/.config/freerdp
COPY --from=backend /out/goguacamole /usr/local/bin/goguacamole
COPY docker/allinone/supervisord.conf /etc/supervisord.conf
COPY docker/allinone/entrypoint.sh /usr/local/bin/allinone-entrypoint
COPY config.toml.example /app/config.toml.example
ENV TZ=America/Sao_Paulo \
    APP_ADDR=:8080 \
    DB_DRIVER=sqlite \
    DB_DSN=/data/goguacamole.db \
    APP_SECRET_KEY_FILE=/data/secret.key \
    GUACD_ADDR=127.0.0.1:4822 \
    RECORDING_PATH=/data/recordings \
    REDIS_ADDR=127.0.0.1:6379
VOLUME ["/data"]
EXPOSE 8080 8443
HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \
    CMD ["goguacamole", "healthcheck"]
# starts as root only to prepare the bind-mounted /data; every program runs as guacd (uid 1000)
ENTRYPOINT ["/usr/local/bin/allinone-entrypoint"]

# ---------------------------------------------------------------------------
# Stage 3: minimal non-root runtime
# ---------------------------------------------------------------------------
FROM alpine:3.22
LABEL org.opencontainers.image.source="https://github.com/jniltinho/go-guacamole" \
      org.opencontainers.image.description="GoGuacamole: web gateway for Apache guacd (RDP, SSH, VNC)" \
      org.opencontainers.image.licenses="MIT"
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S goguacamole && adduser -S -G goguacamole -h /app goguacamole \
    && mkdir -p /data && chown goguacamole:goguacamole /data
ENV TZ=America/Sao_Paulo \
    APP_ADDR=:8080 \
    DB_DRIVER=sqlite \
    DB_DSN=/data/goguacamole.db \
    APP_SECRET_KEY_FILE=/data/secret.key \
    GUACD_ADDR=guacd:4822
WORKDIR /app
COPY --from=backend /out/goguacamole /usr/local/bin/goguacamole
COPY config.toml.example /app/config.toml.example
USER goguacamole
VOLUME ["/data"]
EXPOSE 8080 8443
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD ["goguacamole", "healthcheck"]
ENTRYPOINT ["goguacamole"]
CMD ["serve"]
