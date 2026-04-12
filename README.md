# 📡 openwrt-controller: The Nerve Center

**openwrt-controller** es una plataforma de orquestación, telemetría y gestión distribuida para flotas de dispositivos OpenWrt. Diseñado para operar en entornos críticos y remotos, combina una arquitectura de microservicios ligera con una interfaz de mando **Neo-Brutalista** de alta visibilidad.

## 🛠️ Arquitectura del Sistema

El sistema se divide en tres componentes clave que operan en perfecta sincronía:

1.  **The Core (Backend):** Motor en **Go** de alto rendimiento que gestiona la persistencia en **PostgreSQL**, telemetría en **InfluxDB** y orquestación paralela vía **SSH criptográfico**.
2.  **The Interface (Frontend):** Dashboard en **Vue 3** optimizado para baja fatiga visual (Vantablack) con visualizaciones en tiempo real mediante WebSockets.
3.  **The Agent (`agent.sh`):** Script ultra-ligero en shell/awk que reside en los nodos OpenWrt, encargado de la recolección multimodal y la auto-inyección de identidades.

* * *

## 🛰️ Módulos Operativos

| Módulo | Nombre Clave | Funcionalidad | Color Táctico |
| --- | --- | --- | --- |
| **Shell** | `MATRIX_SHELL` | Terminal SSH embebida mediante WebSockets y xterm.js. | Verde Neón |
| **Telemetry** | `CHRONOS_VIEW` | Análisis histórico de series de tiempo (Signal, Traffic, CPU). | Verde Neón |
| **Alerts** | `THE_SIGNAL` | Motor de incidentes reactivo con notificaciones vía Telegram. | Rojo/Naranja |
| **Topology** | `THE_GRID` | Grafo dinámico de la red basado en tablas de Bridge y Wireless. | Cian Neón |
| **Mass Mgmt** | `ORCHESTRATOR` | Ejecución de comandos en lote y gestión de perfiles globales. | Amarillo Neón |
| **RF Intel** | `RF_INTELLIGENCE` | Optimización heurística de canales y análisis de SNR. | Cian Brillante |
| **Backups** | `THE_VAULT` | Bóveda de configuraciones con control de versiones y visualización Diff. | Blanco Plata |

* * *

## 🔒 Seguridad y Hardening

-   **Identidad Criptográfica:** El controlador utiliza un par de llaves Ed25519/RSA para gestionar los nodos; las contraseñas planas están prohibidas en el plano de control.
-   **Site-Isolation:** Cada sitio geográfico requiere una `X-Site-Key` única para reportar telemetría.
-   **Auth de Usuario:** Acceso protegido por **JWT (JSON Web Tokens)** con roles granulares (Admin/Viewer).
-   **Data Integrity:** Los backups en `The Vault` se validan mediante checksums **SHA256**.

* * *

## 🚀 Instalación y Despliegue

### Requisitos Previos

-   **Docker & Docker Compose** (para PostgreSQL e InfluxDB).
-   **Go 1.21+** (para compilar el Core).
-   **Node.js 18+** (para compilar la Interface).

### 1\. Levantar Infraestructura

Bash

```
docker-compose up -d
```

### 2\. Compilar el Controlador

Bash

```
# Generar llaves SSH maestras
mkdir -p certs
ssh-keygen -t ed25519 -f ./certs/id_controller -N ""

# Compilar Core
go build -ldflags="-s -w" -o openwrt-controller ./cmd/openwrt-controller
```

### 3\. Compilar Interface

Bash

```
cd web
npm install
npm run build
```

### 4\. Configurar el Agente

Copia el archivo `agent.sh` a tus routers en `/root/agent.sh`, configura la `CONTROLLER_IP` y la `SITE_KEY`, y ejecútalo:

Bash

```
chmod +x /root/agent.sh
/root/agent.sh &
```

* * *

## 📑 Procedimientos de Emergencia (Runbook)

-   **Pérdida de Nodo:** Consultar `THE_GRID` para identificar el punto de fallo. Si el hardware ha muerto, reemplazar y usar `THE_VAULT` para restaurar la última configuración conocida.
-   **Saturación de Radio:** Ejecutar `RF_FIX` desde el módulo de Inteligencia de Radio para reubicar canales automáticamente.
-   **Actualización de Flota:** Utilizar el `ORCHESTRATOR` para enviar comandos `sysupgrade` masivos tras cargar el firmware en la Bóveda.

* * *

## 🌲 Créditos y Desarrollo

Desarrollado por **Sebastián Real** como parte del ecosistema de infraestructura distribuida para **nexOS** y gestión de activos rurales en Pallatanga, Ecuador.

> _"En la red, como en la montaña, la visibilidad es la clave de la supervivencia."_

