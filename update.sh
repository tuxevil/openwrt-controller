#!/bin/bash
set -e

echo "[*] Deteniendo instancias previas..."
systemctl stop openwrt-controller 2>/dev/null || true
# Matar cualquier proceso zombie que haya quedado en el puerto 3000
fuser -k 3000/tcp 2>/dev/null || true
sleep 1

echo "[*] Reconstruyendo frontend (Vue/Vite)..."
cd ${OPENWRT_CONTROLLER_DIR:-/opt/openwrt-controller}/web
npm install
npm run build

echo "[*] Reconstruyendo backend (Go)..."
cd ${OPENWRT_CONTROLLER_DIR:-/opt/openwrt-controller}
go build -o openwrt-controller ./cmd/openwrt-controller/main.go

echo "[*] Reiniciando el servicio openwrt-controller..."
systemctl start openwrt-controller
sleep 2
systemctl status openwrt-controller --no-pager | grep -E "Active:|Main PID"

echo "[+] ¡Actualización completada con éxito!"
