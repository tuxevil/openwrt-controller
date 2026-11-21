# Conectar OpenClaw y Hermes a Sentinel

Usa el endpoint LAN `https://10.128.128.6:8443/api/sentinel/mcp` desde el
gateway de OpenClaw (`agent.tuxevil.com`) y Hermes (`bot.tuxevil.com`).
Instala el certificado de confianza de Nerve Center en ambos procesos.
Desde cada host, comprueba que TLS funciona y devuelve `{"status":"ready"}`.

    curl --cacert /ruta/nerve-center.crt https://10.128.128.6:8443/readyz

## Emitir una credencial por cliente

El controlador ya lee `/etc/openwrt-controller/sentinel-mcp-identities.json`
en cada petición. Genera dos tokens independientes con `openssl rand -hex 32`
en una sesión privada. Guarda cada token original en el almacén de secretos
del cliente correspondiente. Calcula el SHA-256 del texto exacto del token,
sin salto de línea, con `printf '%s' "$TOKEN" | sha256sum`. Guarda solo los
hashes en las entradas del controlador.

    [
      {
        "name": "openclaw",
        "token_sha256": "HASH_SHA256_DEL_TOKEN_OPENCLAW",
        "tenant_schema": "tenant_dragontec",
        "site_id": "d982665c-8fd8-4c32-815f-257cf5bb450a"
      },
      {
        "name": "hermes",
        "token_sha256": "HASH_SHA256_DEL_TOKEN_HERMES",
        "tenant_schema": "tenant_dragontec",
        "site_id": "d982665c-8fd8-4c32-815f-257cf5bb450a"
      }
    ]

Conserva las entradas existentes. Crea el archivo nuevo en el mismo
directorio con permisos `0600`, valida su JSON con
`python3 -m json.tool archivo.json >/dev/null` y reemplaza el original
con `mv`. El cambio se aplica en la siguiente petición.

## OpenClaw

Agrega el servidor dentro de `mcp.servers` en el perfil del gateway.
Sustituye el marcador de la cabecera mediante el mecanismo de secretos
de tu instancia. Conserva las demás entradas de configuración.

    {
      "mcp": {
        "servers": {
          "sentinel": {
            "transport": "streamable-http",
            "url": "https://10.128.128.6:8443/api/sentinel/mcp",
            "headers": { "Authorization": "Bearer TOKEN_DE_OPENCLAW" }
          }
        }
      }
    }

Si Node requiere un certificado adicional, configura `NODE_EXTRA_CA_CERTS`
con el PEM en el entorno del gateway. Ejecuta
`openclaw mcp probe sentinel --json` desde su perfil.
OpenClaw 2026.9.4 requiere el transporte explícito para este endpoint.

## Hermes

Integra esta entrada en el `config.yaml` del perfil activo. Define
`SENTINEL_HERMES_TOKEN` en el entorno privado del proceso.

    mcp_servers:
      sentinel:
        url: https://10.128.128.6:8443/api/sentinel/mcp
        headers:
          Authorization: "Bearer ${SENTINEL_HERMES_TOKEN}"

Si Python requiere una CA adicional, configura `SSL_CERT_FILE` con un
bundle que incluya la CA de Nerve Center. Ejecuta `hermes mcp test sentinel`
y recarga la sesión para descubrir las herramientas.

## Confirmar y revocar el acceso

Ambos clientes deben descubrir `get_site_health` y `get_case`. Pide una
consulta de salud para comprobar que la credencial ve el sitio Dragontec
y sus tres nodos registrados. Para consultar un caso, usa el UUID del
dashboard. Ambas capacidades son de lectura. Las aprobaciones siguen
en Nerve Center.

HTTP 401 indica una credencial ausente, inválida o revocada. HTTP 405 al
conectar suele indicar que el cliente intentó usar SSE. Un error de TLS
requiere instalar la confianza o usar un nombre/IP incluido en el
certificado. Para revocar un cliente, elimina su entrada y reemplaza el
archivo de forma atómica.
