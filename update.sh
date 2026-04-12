#!/bin/bash
set -e

echo "[*] Reconstruyendo frontend (Vue/Vite)..."
cd /root/openwrt-controller/web
npm install
npm run build

echo "[*] Reconstruyendo backend (Go)..."
cd /root/openwrt-controller
go build -o openwrt-controller ./cmd/openwrt-controller/main.go

echo "[*] Reiniciando el servicio openwrt-controller..."
service openwrt-controller restart

echo "[+] ¡Actualización completada con éxito!"
