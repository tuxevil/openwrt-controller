# Plan de Sentinel residente para un homelab off-grid

Propuesta para revisión. El operador abre Nerve Center desde la LAN, pregunta qué ocurrió, recibe un informe y puede encargar una acción. Con el navegador cerrado, el controlador sigue vigilando. OpenClaw y Hermes consultan al mismo Sentinel con identidades propias. La desconexión de Internet forma parte de las pruebas de aceptación desde la primera entrega.

## Punto de partida

Las [rutas de Sentinel](../internal/api/routes.go) ya exponen conversaciones, mensajes, ejecuciones, casos y aprobación de propuestas. El [servicio de alertas](../internal/services/alerts.go) y el [análisis de logs](../internal/services/sentinel_core.go) ya pueden abrir casos. El [registro de herramientas](../internal/services/sentinel_tool_registry.go) y el [enrutamiento de modelos](SENTINEL_MODEL_ROUTING.md) son puntos de integración. Confirmar en la versión desplegada qué recorrido funciona de extremo a extremo antes de ampliar la interfaz.

El controlador Go conserva la autoridad sobre permisos, tareas, auditoría y acceso a los nodos. El modelo, servido en el controlador o en otro host LAN, interpreta preguntas y resume evidencia. Los agentes OpenWrt conservan sus funciones de telemetría y ejecución. Evitar instalar un runtime de inferencia en cada AP. Usar la [guía del operador](SLM_OPERATOR_CHEATSHEET.md) como referencia para el modelo y reflejar cada límite crítico en código.

## Entregas y pruebas en orden

| Entrega | Depende de | Resultado y prueba de aceptación |
|---|---|---|
| 1. Arranque local | Inventario del host | Medir RAM, latencia, consumo en reposo e inferencia con gemma4:e2b. Precargar pesos, runtime, recursos web y confianza TLS. Con WAN bloqueada desde el arranque, verificar login, modelo, DNS/hora locales y ausencia de intentos de salida en la ruta del agente. Deshabilitar escalado remoto y revisar también los proveedores de ambos asistentes. |
| 2. Conversación con evidencia | 1 | Desde la interfaz preguntar por un nodo y por un intervalo pasado. Responder con sitio, nodo, hora, fuente y antigüedad. Ante datos vencidos, indicar qué se desconoce. Con el modelo detenido, conservar acceso a métricas y casos, y mostrar inferencia indisponible. |
| 3. Vigilancia residente | 2 | Reutilizar incidentes y casos. Añadir un barrido periódico configurable de frescura, empezando por un minuto en laboratorio. Separar último contacto del nodo, última ejecución del supervisor y salud del modelo. Con UI cerrada, simular heartbeats perdidos, repetición y recuperación. Verificar un solo caso con cronología y cierre. |
| 4. Avisos e informes | 3 | Bandeja LAN persistente con severidad, evidencia y acuse. Informe bajo demanda y resumen diario con cobertura, intervalos sin muestras y casos pendientes. Persistir entregas con reintentos acotados. Reiniciar el controlador y verificar que el aviso pendiente se entrega una vez al usuario. |
| 5. Asistentes personales | 2 y 4 | Adaptador MCP sobre HTTPS LAN. Empezar con salud, consulta de caso e informe. OpenClaw y Hermes deben consultar el mismo caso desde sus versiones instaladas. Revocar un cliente y comprobar que el otro conserva acceso. Probar denegación entre sitios y denegación de aprobación. |
| 6. Acción solicitada | 2 y política de identidad verificada | El backend traduce una intención admitida a un plan tipado con destino, diff, caducidad y reversión. El operador aprueba ese plan en Nerve Center. Probar primero en simulación y luego, con permiso específico, una operación reversible en un nodo de prueba. Comprobar estado aplicado y efecto observado. |
| 7. Autonomía limitada | 3, 4 y 6 | Reproducir incidentes históricos y operar inicialmente en modo observación. Evaluar falsos positivos, evidencia insuficiente y frecuencia de acciones. Habilitar individualmente reglas aceptadas por el operador. Probar presupuesto de reintentos, exclusión por nodo, parada manual y recuperación tras reinicio. |

Revisar la utilidad del chat tras la entrega 2, la calidad de detección tras la 4 y los límites de autorización tras la 6. Cada entrega debe funcionar con la WAN bloqueada. Una copia local de los artefactos de instalación permite repetir las pruebas después de restaurar el host.

## Comportamiento durante la operación

- En cada barrido, reglas deterministas comparan las muestras con su cadencia esperada. Una anomalía sostenida abre o actualiza un caso persistente por sitio, nodo y síntoma. El modelo se activa por consulta, informe o cambio relevante. Registrar duración del barrido y última ejecución satisfactoria.
- Cuando varios nodos dejan de reportar juntos, comprobar antes reloj, colector y controlador. Un heartbeat vencido justifica informar pérdida de telemetría. Para atribuir una caída de red se necesita evidencia adicional. Registrar ventanas de mantenimiento y velocidades esperadas por enlace.
- Si falla la inferencia, el supervisor sigue detectando, conservando evidencia y avisando mediante plantillas. Si cae el controlador completo, también cae su vigilante. Cuando se necesite detectar ese fallo, usar un testigo en otro host LAN con canal local independiente.
- Mantener por defecto autonomía de observación, investigación e informes. Un primer acto autónomo reversible podría ser reintentar una recopilación de solo lectura después de un fallo transitorio. Para futuras acciones de escritura, exigir regla habilitada por el operador con destino, condiciones, caducidad, presupuesto, verificación y comportamiento ante fallo. Mantener aprobación humana para cambios de conectividad, credenciales y firmware.
- Tratar logs y respuestas de herramientas como datos. Aplicar esquemas, permisos por sitio y límites en el servidor. Excluir secretos y minimizar identificadores de clientes en informes. Registrar solicitante, acción, evidencia, decisión y resultado. Probar que una instrucción incrustada en un log no concede permisos.

## Contrato con OpenClaw y Hermes

Cada cliente recibe credencial revocable propia, sitio autorizado y permisos mínimos. Validar TLS con la CA local. El adaptador llama a los servicios de Sentinel bajo esa identidad. Las peticiones largas devuelven un identificador persistente para consultar progreso y resultado. Usar una clave de idempotencia para encargos repetidos. Mantener las credenciales de nodos y la aprobación humana dentro del controlador.

MCP ya está documentado en [OpenClaw](https://docs.openclaw.ai/tools/mcp) y [Hermes](https://hermes-agent.nousresearch.com/docs/user-guide/features/mcp). Verificar transporte y autenticación en ambos clientes instalados. Evaluar [A2A](https://a2a-protocol.org/latest/topics/key-concepts/) cuando se necesite interoperabilidad estándar para delegar tareas entre agentes, descubrir capacidades o intercambiar resultados por etapas. Ambos adaptadores pueden usar el mismo caso, identidad y política.

## Primera demostración

En laboratorio, arrancar sin WAN, cerrar la interfaz y simular pérdida de telemetría de un AP. Abrir Nerve Center y preguntar qué pasó. Pedir el mismo informe desde Hermes y OpenClaw. Los tres deben citar el mismo caso, ventana temporal y evidencia, reconocer incertidumbre y mostrar la recuperación. Repetir con el modelo detenido y verificar que el aviso y la cronología siguen disponibles. Esta demostración se completa antes de habilitar cambios autónomos de infraestructura.

### Endpoint MCP de lectura

`/api/sentinel/mcp` usa Streamable HTTP sobre el listener TLS de Nerve Center. Para activarlo, configura `SENTINEL_MCP_IDENTITIES_FILE` con la ruta de un archivo JSON de permisos `0600` legible por el controlador. Cada entrada tiene `name`, `token_sha256`, `tenant_schema` y `site_id`. Usa un token aleatorio independiente de al menos 32 bytes por cliente, guarda solo el SHA-256 hexadecimal del texto del token en el archivo y configura el token original como `Authorization: Bearer …` en cada cliente. Usa el hostname del certificado y confía en la CA local. Actualiza el archivo por reemplazo atómico. Eliminar una entrada revoca su acceso en la siguiente petición.

Las herramientas iniciales son `get_site_health` y `get_case`. La primera devuelve la frescura de la telemetría, la hora de observación y los conteos de incidentes abiertos y resueltos del sitio en las últimas 24 horas. La segunda devuelve un resumen exclusivamente para el sitio asignado por el controlador. Los clientes no eligen el tenant ni el sitio en la llamada. El endpoint funciona en modo stateless y registra las consultas sin incluir el token.

En OpenClaw 2026.9.4, establece `transport: "streamable-http"` junto a
`url` y la cabecera de autorización. En un perfil de laboratorio,
`openclaw mcp probe sentinel --json` descubrió las herramientas
`get_site_health` y `get_case` con TLS y una credencial temporal.
La selección automática intentó SSE y recibió HTTP 405. El endpoint
stateless atiende las operaciones del cliente mediante POST.

El checkout de Hermes Agent `755a2804` también descubrió ambas
herramientas contra el endpoint TLS de laboratorio. Su función de
diagnóstico usó `url` y `headers.Authorization` de la configuración
de `mcp_servers` y el certificado de prueba en el almacén de confianza
de su proceso. En la LAN, usa la CA que firmó el certificado del controlador.

### Arranque de laboratorio sin conectividad externa

El 2026-11-21 se compiló y arrancó Nerve Center en un espacio de red Linux
con solo loopback y sin rutas externas. El laboratorio usó PostgreSQL
desechable, un tenant sintético y un proveedor HTTP de prueba. Se
verificaron la respuesta HTML de `/global/sentinel`, el login, ambos
informes, dos solicitudes de acuse con el mismo resultado y una
conversación asíncrona completada y consultable por su identificador.

La prueba cubre el arranque y las APIs del controlador con aislamiento de
red. La prueba de LiteLLM real se realizó por separado en la LAN. Queda
por comprobar el recorrido en navegador y los clientes MCP con el
controlador y LiteLLM activos en una LAN sin salida a Internet.

La segunda ejecución usó LiteLLM real mediante un relé TLS dedicado a
su IP LAN. El controlador seguía en un espacio de red sin rutas externas,
con PostgreSQL y telemetría sintéticos. Tras el login, una consulta
asíncrona ejecutó `get_device_status` y respondió citando el nodo de
prueba. El registro muestra dos rondas, 3.777 tokens y 5,09 s totales.
También pasaron la respuesta HTML, ambos informes y el acuse idempotente.
El relé mantuvo el certificado de LiteLLM y no dio salida general al
controlador. La salida a Internet del host de LiteLLM se debe verificar
por separado.

El cliente de Hermes también ejecutó ambas herramientas sobre PostgreSQL
desechable con dos sitios. El informe contó únicamente el nodo y el
incidente del sitio asignado. El caso de ese sitio se devolvió y la
consulta del caso del segundo sitio devolvió `case unavailable`.

El laboratorio aislado verificó también la vigilancia sin clientes de
dashboard conectados por WebSocket. Un nodo sintético con 12 minutos sin
telemetría generó un caso `NODE_DOWN`. Dos barridos conservaron el mismo
identificador. Tras una petición válida a `POST /api/telemetry` que devolvió
HTTP 202, el mismo caso pasó a `RESOLVED`.

También se abrió el dashboard compilado en Chromium dentro del espacio de
red aislado. La prueba inició sesión con un usuario sintético, abrió
Sentinel, leyó el resumen diario y el informe bajo demanda, recuperó una
conversación guardada, completó otra consulta y leyó el acuse persistido.
Esta ejecución usó el proveedor simulado. El ciclo con LiteLLM real se
verificó por API en la prueba anterior.
