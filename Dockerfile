# --- Stage 1: Build the Vue.js frontend SPA ---
FROM node:20-alpine AS frontend-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- Stage 2: Build the Go backend ---
FROM golang:1.24-alpine AS backend-builder
WORKDIR /app
ENV GOTOOLCHAIN=auto
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o openwrt-controller ./cmd/openwrt-controller/main.go

# --- Stage 3: Production final container ---
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy built assets
COPY --from=backend-builder /app/openwrt-controller /app/openwrt-controller
COPY --from=frontend-builder /app/web/dist /app/web/dist

# Expose server port
EXPOSE 3000

# Set production environment variables defaults
ENV PORT=3000
ENV DATABASE_URL=postgres://postgres:postgres@openwrt_postgres:5432/openwrthub
ENV INFLUX_URL=http://openwrt_influx:8086
ENV INFLUX_ORG=openwrthub
ENV INFLUX_BUCKET=telemetry

# Run application
CMD ["/app/openwrt-controller"]
