# Despliegue en Producción con Coolify

Nerve Center CE está completamente preparado para ser desplegado en producción de manera sencilla y automatizada utilizando **Coolify**.

Tienes dos opciones principales para realizar el despliegue. Elige la que mejor se adapte a tus necesidades.

---

## Opción 1: Despliegue Todo-en-Uno (Docker Compose) - *Recomendado*

Esta opción levanta el controlador, la base de datos PostgreSQL y la base de datos InfluxDB de manera conjunta en un único stack dentro de Coolify.

### Pasos:
1. En tu panel de Coolify, entra a tu proyecto y haz clic en **New Resource** (Nuevo Recurso).
2. Selecciona **Docker Compose**.
3. En la sección de configuración de origen, selecciona tu repositorio de Git o pega directamente el contenido de nuestro archivo `docker-compose.yml` optimizado:
   ```yaml
   # Pega el contenido de docker-compose.yml de la raíz del proyecto
   ```
4. Define las variables de entorno necesarias en la pestaña **Environment Variables** de Coolify:
   - `JWT_SECRET`: Llave secreta segura para firmar los tokens JWT (ej. `tu_clave_secreta_super_segura_de_produccion`).
   - `INFLUX_TOKEN`: Token de autenticación de InfluxDB (ej. `REPLACE_WITH_INFLUX_TOKEN`).
   - `POSTGRES_PASSWORD`: Cambia la contraseña por defecto de PostgreSQL por una segura.
5. Haz clic en **Deploy** (Desplegar). Coolify clonará el repositorio, detectará el `Dockerfile` multi-stage, construirá la SPA de Vue, compilará el backend de Go y levantará todo el ecosistema automáticamente.

---

## Opción 2: Despliegue por Separado (Para máxima flexibilidad)

Si prefieres separar las bases de datos de la lógica de la aplicación para poder gestionarlas o escalarlas por separado en Coolify, sigue este flujo:

### Paso 1: Crear las Bases de Datos en Coolify
1. Crea un servicio de **PostgreSQL** usando la base de datos preconfigurada de Coolify ("New Resource" -> "PostgreSQL").
2. Crea un servicio de **InfluxDB 2.x** ("New Resource" -> "Service" -> buscar "InfluxDB"). Anota el Token, la Organización (`openwrthub`) y el Bucket (`telemetry`) creados.

### Paso 2: Crear la Aplicación
1. En tu proyecto de Coolify, haz clic en **New Resource** -> **Application** (Aplicación).
2. Selecciona tu repositorio de Git donde tienes `openwrt-controller`.
3. Selecciona la rama de producción (ej. `main` o `master`).
4. Coolify detectará el `Dockerfile` en la raíz automáticamente.
5. Configura el **Port** (Puerto de escucha) en `3000`.

### Paso 3: Configurar Variables de Entorno de la App
En la pestaña **Environment Variables** de la aplicación en Coolify, ingresa los siguientes valores obtenidos de los servicios creados en el Paso 1:
- `DATABASE_URL`: La URL de conexión de Postgres, por ejemplo: `postgres://postgres:<contraseña>@<host-postgres-de-coolify>:5432/openwrthub`
- `INFLUX_URL`: La URL de conexión de InfluxDB, por ejemplo: `http://<host-influx-de-coolify>:8086`
- `INFLUX_ORG`: `openwrthub`
- `INFLUX_BUCKET`: `telemetry`
- `INFLUX_TOKEN`: El token secreto generado en la base de datos InfluxDB.
- `JWT_SECRET`: Una clave aleatoria larga para firmar sesiones.

### Paso 4: Desplegar
Haz clic en **Deploy** en la interfaz de la Aplicación. ¡Listo! Coolify se encargará del resto de forma totalmente desatendida.

---

## Características de Producción Incluidas:
* **Construcción Multi-Stage Optimizada:** La imagen final está basada en Alpine Linux ultra-liviano (peso de la imagen final menor a ~50MB).
* **Seguridad:** Los estáticos del frontend de Vue y la API de Go se sirven desde el mismo puerto y origen (`3000`), evitando problemas de CORS y simplificando el enrutamiento y la configuración de certificados SSL en Coolify.
* **Caché Eficiente:** Uso de `.dockerignore` inteligente que evita la subida de carpetas de desarrollo (`node_modules`, `dist`, archivos temporales), reduciendo el tiempo de build de minutos a solo unos segundos.
