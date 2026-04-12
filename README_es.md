# 📡 openwrt-controller: The Nerve Center \[V2.0 - Extended\]

Plaintext

```
  _   _                       _____           _            
 | \ | |                     / ____|         | |           
 |  \| | ___ _ ____   _____ | |     ___ _ __ | |_ ___ _ __ 
 | . ` |/ _ \ '__\ \ / / _ \| |    / _ \ '_ \| __/ _ \ '__|
 | |\  |  __/ |   \ V /  __/| |___|  __/ | | | ||  __/ |   
 |_| \_|\___|_|    \_/ \___| \_____\___|_| |_|\__\___|_|   
 [ SYSTEM_STATUS: OMEGA_ACTIVE ] [ OVERLAY: WIREGUARD_ENCRYPTED ]
```

**openwrt-controller** es un sistema de gestión centralizado (C&C) de grado industrial diseñado específicamente para flotas de routers OpenWrt operando en entornos geográficamente dispersos y con conectividad desafiante.

* * *

## 🏗️ Arquitectura del Ecosistema

El sistema opera bajo un modelo de **Tres Capas Tácticas**:

### 1\. The Core (Backend - Go)

-   **Concurrencia:** Utiliza `sync.WaitGroups` y canales de Go para la ejecución de comandos SSH en paralelo sobre cientos de nodos sin bloquear el hilo principal.
-   **Criptografía:** Implementa curvas `X25519` nativas para la red WireGuard y `Ed25519` para identidades de terminal.
-   **Middleware:** Sistema de autenticación mediante JWT (JSON Web Tokens) con rotación automática de llaves.

### 2\. The Grid (Data Lake - Influx & Postgres)

-   **Series Temporales:** InfluxDB 2.x procesa telemetría de alta frecuencia (señal, ruido, tráfico).
-   **Capa Relacional:** PostgreSQL 16 gestiona la persistencia de perfiles, inventario de hardware y auditoría de incidentes.
-   **Búsqueda Forense:** Índice GIN (Generalized Inverted Index) sobre la tabla de logs para búsquedas de texto completo en milisegundos.

### 3\. The Shadow Agent (Node - agent.sh)

-   **Eficiencia:** Escrito en `ash` puro, optimizado para ejecutarse en dispositivos con tan solo 32MB de RAM (como el WNDR3700).
-   **Resiliencia:** Sistema de gestión de procesos vía `procd` con watchdog integrado.

* * *

## 🛰️ Desglose Detallado de Módulos

### 🔒 \[SECURE\_TUNNEL\] - SD-WAN Criptográfica

Crea una red Overlay cifrada entre el controlador y la flota.

-   **Protocolo:** WireGuard.
-   **Asignación:** Pool estático `10.8.0.0/24`.
-   **Acceso:** Mapeo directo a interfaces LuCI locales mediante túneles punto a punto.
-   **Auto-Aprovisionamiento:** El agente detecta la falta de herramientas (`wireguard-tools`) y las instala vía `apk/opkg` automáticamente.

### 📈 \[LOG\_HARVESTER\] - Inteligencia Forense

Centraliza el flujo de `logread` de toda la flota.

-   **Parser:** Algoritmo de detección de patrones para clasificar severidad (Critical, Error, Warning, Info).
-   **Escapado:** Tratamiento de strings avanzado para inyectar logs complejos en JSON sin romper el buffer de comunicación.
-   **Alertas:** Integración directa con el motor de incidentes; un "Kernel Panic" en Pallatanga dispara una alerta inmediata.

### 🔄 \[AGENT\_UPDATE\_SERVICE\] - Despliegue de Código

Permite evolucionar la lógica de telemetría sin intervención física.

-   **Mecanismo:** Descarga vía Raw-API con verificación de Checksum SHA256.
-   **A/B Testing:** Mantiene una copia `.old` funcional en el router.
-   **Rollback:** Si el nuevo script falla en reportar telemetría exitosa 3 veces seguidas, el router restaura la versión anterior y se reinicia.

### ☢️ \[RF\_INTELLIGENCE\] - Optimización de Espectro

Análisis heurístico de la capa física inalámbrica.

-   **SNR Analysis:** Cálculo dinámico de la relación señal-ruido.
-   **Remediación:** Botón `RF_FIX` que reasigna canales (1, 6, 11) basado en el menor suelo de ruido reportado por los drivers de radio.

### 🛡️ \[THE\_VAULT\] - Gestión de Configuración

Bóveda de archivos `/etc/config` con integridad garantizada.

-   **Backups:** Snapshots diarios empaquetados en `.tar.gz`.
-   **Diff Engine:** Visor monospaced que resalta cambios en configuraciones de Firewall, Red o WiFi entre dos fechas.

* * *

## 📑 Guía de Operaciones Tácticas

### Despliegue de un Nuevo Nodo

1.  **Aprovisionamiento:** El router contacta al controlador con su ID de Hardware.
2.  **Identidad:** El controlador genera el par de llaves WireGuard y asigna la IP `10.8.0.x`.
3.  **Inyección:** El agente descarga el perfil, instala dependencias y levanta el túnel `wg0`.

### Procedimiento ante Fallo Crítico

1.  **Observación:** Revisar el **Global Health Score** en el Dashboard.
2.  **Aislamiento:** Entrar al **Log Explorer** para buscar errores de sincronización.
3.  **Acceso:** Usar el botón **\[OPEN\_LUCI\]** para entrar por la IP del túnel.
4.  **Recuperación:** Si la configuración está corrupta, aplicar un `Restore` desde **The Vault**.

* * *

## 🌲 Contexto de Infraestructura (Pallatanga)

El sistema está optimizado para las condiciones específicas del usuario:

-   **Energía:** Notificaciones de bajo impacto para evitar despertar innecesariamente radios en nodos secundarios.
-   **Conectividad:** Tolerancia extrema a latencias altas y micro-cortes en el enlace de montaña.
-   **Seguridad:** Aislamiento total de las 6 empresas corporativas mediante reglas de firewall inyectadas por el Orquestador.

* * *

**Status:** `READY_FOR_PRODUCTION` **Binary:** `v2.0.4-omega` **Maintainer:** Sebastián Real

