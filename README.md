# 📡 openwrt-controller: The Nerve Center \[V2.0\]

**openwrt-controller** es una plataforma de orquestación masiva y SD-WAN privada diseñada para la gestión de flotas OpenWrt en entornos críticos y remotos (como infraestructuras rurales u off-grid). El sistema evoluciona de un simple monitor a un **Cerebro de Red** capaz de autogestionarse, repararse y ofrecer túneles de cristal hacia el interior profundo de cada nodo.

* * *

## 🛰️ Módulos de Operación Avanzada

| Módulo | Nombre Clave | Funcionalidad | Estatus |
| --- | --- | --- | --- |
| **VPN Overlay** | `SECURE_TUNNEL` | Red privada WireGuard (10.8.0.x) con acceso directo a LuCI. | **ACTIVO** |
| **Auto-Update** | `AGENT_UPDATE` | Sistema de despliegue de código con A/B Rollback automático. | **ACTIVO** |
| **Log Center** | `LOG_HARVESTER` | Centralización de Syslogs con búsqueda indexada (GIN Index). | **ACTIVO** |
| **Orchestrator** | `COMMAND_CENTER` | Ejecución paralela vía SSH (sync.WaitGroups) y perfiles globales. | **ACTIVO** |
| **RF Intel** | `RF_INTELLIGENCE` | Diagnóstico de SNR y optimización heurística de canales. | **ACTIVO** |
| **Topology** | `THE_GRID` | Mapeo visual de nodos y clientes con trazas de tráfico animadas. | **ACTIVO** |
| **Security** | `THE_VAULT` | Bóveda de backups SHA256 con visor diferencial (Diff). | **ACTIVO** |

Exportar a Hojas de cálculo

* * *

## 🏗️ Stack Tecnológico de Grado Operativo

-   **Core (Backend):** Go (Golang) con concurrencia nativa para gestión de miles de hilos SSH.
-   **Data Lake:** InfluxDB 2.x (Series de tiempo) + PostgreSQL 16 (Relacional/Logs).
-   **Networking:** WireGuard (X25519) para el túnel de gestión cifrado.
-   **Frontend:** Vue 3 + Vite + Tailwind (Estética Vantablack/Cobalto).
-   **Node Agent:** `agent.sh` optimizado para `ash` (BusyBox) con integración `procd`.

* * *

## 🚀 Despliegue de la Red Overlay

### 1\. Requisitos de Infraestructura

El controlador debe correr en un entorno con Docker para la persistencia:

Bash

```
docker-compose up -d  # Postgres & InfluxDB
```

### 2\. Configuración del Túnel (Secure Tunnel)

El controlador genera automáticamente las llaves para cada router. Al añadir un dispositivo, el sistema le asigna una IP en el rango `10.8.0.0/24`.

-   **Endpoint:** Configura tu IP pública/dominio en `Settings -> VPN`.
-   **Handshake:** Los routers se auto-instalan `wireguard-tools` (vía `apk` o `opkg`) al recibir la configuración.

### 3\. El Ciclo de Actualización (Safe Update)

Para actualizar la lógica de toda la flota:

1.  Edita el `agent.sh` en la vista **Agent Management**.
2.  Presiona **\[PUSH\_DEPLOY\]**.
3.  Los routers descargarán la versión, verificarán el Checksum y reiniciarán.
4.  Si un router pierde conexión, volverá automáticamente a la versión anterior (**Rollback Guard**).

* * *

## 📑 Manual de Operaciones (Runbook de Emergencia)

-   **Diagnóstico de Caídas:** Si un nodo entra en `OUT_OF_SYNC`, revisa el **Log Explorer**. Filtra por severidad `ERROR` para buscar fallos de kernel o kernel panics.
-   **Acceso de Emergencia:** Usa el botón **\[OPEN\_LUCI\]** en la Matrix VPN para entrar directamente a la IP 10.8.0.x del dispositivo. No necesitas abrir puertos en el router.
-   **Interferencia de Radio:** En caso de degradación de señal en Pallatanga, activa `RF_FIX` desde el panel de inteligencia para re-escanear el espectro.

* * *

## 🌲 Visión del Proyecto

Este controlador es la columna vertebral de la infraestructura de **Sebastián Real**, integrando la telemetría de red con la resiliencia necesaria para operar en la montaña, asegurando que los 6 sitios de la corporación funcionen como una sola red local unificada y segura.

> **"Status: Omega Active. Traffic: Encrypted. Control: Absolute."**

