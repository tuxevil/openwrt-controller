# syntax=docker/dockerfile:1.7
#
# Multi-stage build for openwrt-controller.
#   1. Build the Vue.js frontend SPA
#   2. Build the Go backend (static, CGO disabled)
#   3. Minimal Alpine runtime, non-root user, read-only friendly
#
# Pinned versions are intentional. Bump via PR after reviewing the
# upstream release notes; do not use `latest` / `alpine` rolling tags.

# ── Stage 1: Frontend builder ────────────────────────────────────────────────
FROM node:22.11.0-alpine AS frontend-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ── Stage 2: Go backend builder ──────────────────────────────────────────────
FROM golang:1.25.4-alpine3.21 AS backend-builder
WORKDIR /app
# Allow auto toolchain switch if go.mod requests a newer Go than the
# base image ships with. Pinned in go.mod to a single toolchain version.
ENV GOTOOLCHAIN=auto
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-w -s" \
    -o /out/openwrt-controller \
    ./cmd/openwrt-controller

# ── Stage 3: Runtime image ───────────────────────────────────────────────────
FROM alpine:3.21.3

# Runtime dependencies only. tzdata is needed for correct local-time
# timestamps in the audit log; ca-certificates for outbound HTTPS
# (InfluxDB over TLS, GitHub-style webhooks, etc.).
RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S -g 10001 app \
    && adduser  -S -u 10001 -G app app

WORKDIR /app

# Copy built assets
COPY --from=backend-builder /out/openwrt-controller /app/openwrt-controller
COPY --from=frontend-builder /app/web/dist           /app/web/dist

# Ownership: the binary and the embedded SPA must be readable by the
# non-root user. A read-only rootfs (see docker-compose) means these
# files don't need to be writable at runtime.
RUN chown -R app:app /app

# Drop privileges. HEALTHCHECK runs as app, server runs as app.
USER app:app

# Expose server port (also documented in docker-compose.yml).
EXPOSE 3000

# Defaults. All secrets MUST be provided at runtime — no defaults for
# DATABASE_URL, JWT_SECRET, INFLUX_TOKEN, etc.
ENV PORT=3000 \
    INFLUX_ORG=openwrthub \
    INFLUX_BUCKET=telemetry

# Liveness probe — process is up and serving HTTP. This is anonymous
# (no auth) and is the right signal for "is the container alive?".
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -qO- http://127.0.0.1:3000/healthz || exit 1

# Run application
ENTRYPOINT ["/app/openwrt-controller"]
