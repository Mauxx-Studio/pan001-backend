# pan001 · Backend

API REST escrita en Go con [Fiber v3](https://gofiber.io) (`v3.6.1`), un framework HTTP
construido sobre `fasthttp`, y [GORM](https://gorm.io) (`v1.31.2`) como ORM para
PostgreSQL.

## Requisitos

- Go 1.26+ (`go version`)
- PostgreSQL 14+ corriendo en local (o accesible por red)
- `make` (opcional, para los atajos del `Makefile`)

## Puesta en marcha

```bash
# 1. Descargar dependencias (Fiber, GORM y godotenv ya están en go.mod)
go mod download
# equivalente: make deps

# 2. Crear el archivo de configuración local y ajustar las credenciales
cp .env.example .env

# 3. Crear la base de datos si todavía no existe
createdb -h localhost -U mauri miapp

# 4. Ejecutar la API (lee .env automáticamente)
go run ./cmd/api
# o
make run

# 5. Probar (responde JSON)
curl http://localhost:8080/
# => {"message":"Hola mundo con Fiber"}

curl http://localhost:8080/health
# => {"database":"up","status":"ok"}
```

El puerto se configura con la variable de entorno `PORT` (por defecto `8080`) y la
conexión a PostgreSQL con las variables `DB_*` documentadas en `.env.example`.
Los valores reales viven en `.env`, que está en `.gitignore`: nunca se versiona.
Las variables ya definidas en el entorno tienen prioridad sobre `.env`.

El servidor hace **apagado ordenado** (`ShutdownWithContext`) al recibir `SIGINT`
(Ctrl+C) o `SIGTERM`, con un timeout de 10 segundos para las peticiones en curso,
y cierra el pool de conexiones de PostgreSQL antes de terminar.

### Endpoints

| Método | Ruta                    | Respuesta                                        |
| ------ | ----------------------- | ------------------------------------------------ |
| `GET`  | `/`                     | `200 OK` · `{"message":"Hola mundo con Fiber"}`  |
| `GET`  | `/health`               | `200 OK` · `{"database":"up","status":"ok"}`     |
| `POST` | `/api/v1/auth/register` | `201 Created` · datos del usuario + token        |
| `POST` | `/api/v1/auth/login`    | `200 OK` · datos del usuario + token             |
| `GET`  | `/api/v1/auth/me`       | `200 OK` · usuario autenticado (requiere token)  |
| `POST` | `/api/v1/contacts`      | `201 Created` · tercero creado (requiere token)  |
| `GET`  | `/api/v1/contacts`      | `200 OK` · lista de terceros con filtros         |
| `GET`  | `/api/v1/contacts/:id`  | `200 OK` · tercero (requiere token)              |
| `PUT`  | `/api/v1/contacts/:id`  | `200 OK` · tercero actualizado (requiere token)  |
| `DELETE` | `/api/v1/contacts/:id` | `204 No Content` · soft delete (requiere token) |

`/health` hace un `Ping` real contra PostgreSQL: devuelve `503 Service Unavailable`
si la base no responde.

## Autenticación (JWT)

El módulo de usuarios sigue las mismas capas que el resto: entidad en
`internal/model`, puerto `repository.UserRepository` con su implementación GORM en
`internal/infrastructure/database`, casos de uso en `internal/service` (bcrypt cost
12), handlers en `internal/handler` y rutas agrupadas en `internal/router` bajo
`/api/v1/auth`.

### Registro

Por ahora está abierto (más adelante se restringirá a usuarios `admin`).

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{
    "email": "maria@pan001.com",
    "password": "secreto123",
    "full_name": "María Pérez",
    "phone": "1155550000",
    "role": "cajero"
  }'
```

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "token_type": "Bearer",
  "user": {
    "id": "4c04cde9-8d4b-40ef-8e39-c6d032e36b9f",
    "email": "maria@pan001.com",
    "full_name": "María Pérez",
    "role": "cajero",
    "phone": "1155550000",
    "is_active": true,
    "created_at": "2026-10-29T01:58:15.754022Z",
    "updated_at": "2026-10-29T01:58:15.754022Z"
  }
}
```

### Login

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email": "maria@pan001.com", "password": "secreto123"}'
```

Responde `200 OK` con la misma forma (`token` + `user`), actualiza
`last_login_at` y compara el email sin distinguir mayúsculas ni espacios.

### Perfil del usuario autenticado

`GET /api/v1/auth/me` es la primera ruta protegida: exige el access token en el
header `Authorization` y devuelve el usuario dueño del token.

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email": "maria@pan001.com", "password": "secreto123"}' | jq -r .token)

curl http://localhost:8080/api/v1/auth/me -H "Authorization: Bearer $TOKEN"
```

```json
{
  "id": "4c04cde9-8d4b-40ef-8e39-c6d032e36b9f",
  "email": "maria@pan001.com",
  "full_name": "María Pérez",
  "role": "cajero",
  "phone": "1155550000",
  "is_active": true,
  "created_at": "2026-10-29T01:58:15.754022Z",
  "updated_at": "2026-10-29T01:58:15.754022Z"
}
```

El perfil se lee de la base con el `user_id` del token (no de los claims), así
que un usuario borrado o desactivado después de emitir el token recibe `401`
aunque el JWT siga vigente.

| Situación                                          | Código | Body                                                       |
| -------------------------------------------------- | ------ | ---------------------------------------------------------- |
| Sin header `Authorization`                         | `401`  | `{"error":"falta el header Authorization"}`                |
| Header que no es `Bearer <token>`                  | `401`  | `{"error":"formato de Authorization inválido: ..."}`       |
| Token expirado, con firma inválida o `user_id` raro | `401`  | `{"error":"token inválido o expirado"}`                    |
| Usuario del token ya eliminado                     | `401`  | `{"error":"el usuario del token ya no existe"}`            |
| Usuario del token desactivado                      | `401`  | `{"error":"el usuario está desactivado"}`                  |

### Rutas protegidas (middleware)

`internal/middleware` aporta dos middlewares que se montan sobre grupos de rutas:

- `Authenticate(tokens)` valida `Authorization: Bearer <token>` con
  `auth.JWTManager.Parse` (el mismo gestor que firma los tokens) y guarda
  `user_id` (como `uuid.UUID`), `email` y `role` en `c.Locals`. Sin token válido
  responde `401` y corta la cadena.
- `RequireRole(roles...)` se monta **después** de `Authenticate` y responde `403`
  si el rol del token no está en la lista permitida.

`router.Register` registra primero las rutas públicas (`/auth/register`,
`/auth/login`) y luego un grupo protegido, de modo que sumar un endpoint privado
es una línea:

```go
protected := api.Group("", middleware.Authenticate(tokens))
protected.Get("/auth/me", handlers.Auth.Me)
protected.Delete("/users/:id", middleware.RequireRole(model.RoleAdmin), handlers.Users.Delete)
```

Dentro de un handler, la identidad se lee con los helpers del paquete
(`middleware.GetUserID(c)`, `middleware.GetUserEmail(c)`,
`middleware.GetUserRole(c)`), que devuelven `uuid.Nil`/`""` si la petición no
pasó por `Authenticate`.

### Reglas y códigos de error

| Situación                                                   | Código | Body                                                              |
| ----------------------------------------------------------- | ------ | ----------------------------------------------------------------- |
| Body inválido o con formato incorrecto                      | `400`  | `{"error":"datos inválidos: email debe ser un email válido"}`     |
| Rol fuera de la lista permitida                             | `400`  | `{"error":"datos inválidos: role debe ser uno de: admin, ..."}`   |
| Email o contraseña incorrectos (incluye soft-deleted)       | `401`  | `{"error":"email o contraseña incorrectos"}`                      |
| Usuario con `is_active = false`                             | `403`  | `{"error":"el usuario está desactivado"}`                         |
| Email ya registrado                                         | `409`  | `{"error":"el email ya está registrado"}`                         |
| Error inesperado (se loguea en el servidor)                 | `500`  | `{"error":"error interno del servidor"}`                          |

Roles válidos: `admin`, `vendedor` (por defecto), `panadero`, `cajero`, `supervisor`.
La contraseña se guarda siempre como hash bcrypt (cost 12) y `password_hash` nunca
aparece en las respuestas: la entidad lo marca con `json:"-"` y además las
respuestas usan un DTO propio (`userResponse`).

El índice único de `email` incluye a los usuarios con soft delete, así que un email
eliminado sigue reservado (volver a registrarlo devuelve `409`); reactivar un
usuario es una operación de administración.

### Access token

- Algoritmo `HS256`, secreto en `JWT_SECRET`.
- Vigencia **24 h** (`auth.DefaultTokenTTL`).
- Claims: `user_id`, `email`, `role` y `exp` (además de `iat`, `nbf`, `sub` e `iss`).
- Si `JWT_SECRET` no está definida se usa el secreto de desarrollo
  `dev-secret-cambiar-en-produccion` y la app lo advierte en los logs; con
  `APP_ENV=production`, `config.Load` falla para no firmar tokens con un secreto
  conocido.

## Terceros (contactos)

El módulo de terceros reutiliza las mismas capas: entidad `model.Contact`, puerto
`repository.ContactRepository`, adaptador GORM en
`internal/infrastructure/database/contact_repository.go`, casos de uso en
`service.ContactService` y handlers en `internal/handler/contact*.go`. Todas las
rutas viven en el grupo protegido, así que exigen `Authorization: Bearer <token>`.

Un tercero puede ser cliente, proveedor, ambos o ninguno todavía. `name` es el
único campo obligatorio; `is_active` se guarda como `true` por defecto. `PUT` es
una actualización parcial: solo cambia los campos enviados.

### Crear

```bash
curl -X POST http://localhost:8080/api/v1/contacts \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Panadería El Sol",
    "business_name": "El Sol SRL",
    "tax_id": "30-12345678-9",
    "email": "contacto@elsol.com",
    "phone": "1122334455",
    "whatsapp": "1122334455",
    "address": "Av. San Martín 123",
    "city": "CABA",
    "notes": "Cliente mayorista",
    "is_client": true,
    "is_supplier": false
  }'
```

```json
{
  "id": "7a1d9f52-6c3b-4b0e-9c2a-6f1f0d8e4b21",
  "name": "Panadería El Sol",
  "business_name": "El Sol SRL",
  "tax_id": "30-12345678-9",
  "email": "contacto@elsol.com",
  "phone": "1122334455",
  "whatsapp": "1122334455",
  "address": "Av. San Martín 123",
  "city": "CABA",
  "notes": "Cliente mayorista",
  "is_client": true,
  "is_supplier": false,
  "is_active": true,
  "created_at": "2026-10-31T12:00:00Z",
  "updated_at": "2026-10-31T12:00:00Z"
}
```

### Listar con filtros

```bash
# Todos los terceros (ordenados por nombre)
curl http://localhost:8080/api/v1/contacts -H "Authorization: Bearer $TOKEN"

# Solo clientes activos y que coincidan con "sol"
curl "http://localhost:8080/api/v1/contacts?is_client=true&is_active=true&search=sol" \
  -H "Authorization: Bearer $TOKEN"
```

`search` ignora mayúsculas y busca en nombre, razón social, CUIT/CUIL, email,
teléfono y WhatsApp. Los filtros `is_client`, `is_supplier` e `is_active` aceptan
`true`/`false`; cualquier otro valor responde `400`. Sin coincidencias la
respuesta es `[]` (nunca `null`).

### Obtener, actualizar y eliminar

```bash
ID=7a1d9f52-6c3b-4b0e-9c2a-6f1f0d8e4b21

curl http://localhost:8080/api/v1/contacts/$ID -H "Authorization: Bearer $TOKEN"

# Actualización parcial: solo cambia el teléfono y suma el rol de proveedor
curl -X PUT http://localhost:8080/api/v1/contacts/$ID \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"phone": "1199998888", "is_supplier": true}'

# Soft delete: responde 204 y el tercero deja de aparecer en la API
curl -X DELETE http://localhost:8080/api/v1/contacts/$ID \
  -H "Authorization: Bearer $TOKEN"
```

| Situación                                     | Código | Body                                          |
| --------------------------------------------- | ------ | --------------------------------------------- |
| Body inválido o JSON malformado               | `400`  | `{"error":"datos inválidos: name es obligatorio"}` |
| `name` vacío en un `PUT`                      | `400`  | `{"error":"el nombre del tercero es obligatorio"}` |
| `:id` que no es UUID o filtro booleano inválido | `400` | `{"error":"el id debe ser un UUID válido"}`    |
| Tercero inexistente o eliminado               | `404`  | `{"error":"tercero no encontrado"}`           |
| Sin token o token inválido                    | `401`  | `{"error":"falta el header Authorization"}`   |

## Base de datos

- **Driver:** `gorm.io/driver/postgres` (usa `pgx/v5` por debajo).
- **Configuración:** `internal/config` carga `.env` con `github.com/joho/godotenv`
  y expone `Config.App` y `Config.Database` tipados, con valores por defecto
  (`DB_HOST=localhost`, `DB_PORT=5432`, `DB_SSLMODE=disable`) y validación de las
  variables obligatorias (`DB_USER`, `DB_NAME`).
- **Conexión:** `internal/infrastructure/database.New(cfg, debug)` abre el pool,
  lo configura (`MaxOpenConns`, `MaxIdleConns`, `ConnMaxLifetime`) y hace `Ping`
  antes de devolver `(*gorm.DB, error)`; cualquier fallo se propaga envuelto con
  `%w` y `main` lo loguea y termina con código 1. `database.Close(db)` cierra el pool.
- **Logger de GORM:** con `APP_ENV=development` loguea todas las consultas SQL
  (con colores y umbral de consultas lentas de 200 ms); en otros entornos solo
  warnings y errores.

### Variables de entorno

| Variable      | Default     | Descripción                                  |
| ------------- | ----------- | -------------------------------------------- |
| `PORT`        | `8080`      | Puerto HTTP de la API                        |
| `APP_ENV`     | `development` | `development` \| `staging` \| `production` |
| `DB_HOST`     | `localhost` | Host de PostgreSQL                           |
| `DB_PORT`     | `5432`      | Puerto de PostgreSQL                         |
| `DB_USER`     | — (requerido) | Usuario de PostgreSQL                      |
| `DB_PASSWORD` | —           | Contraseña del usuario                       |
| `DB_NAME`     | — (requerido) | Nombre de la base de datos                 |
| `DB_SSLMODE`  | `disable`   | `disable` \| `require` \| `verify-full` ...  |
| `JWT_SECRET`  | `dev-secret-cambiar-en-produccion` | Secreto HMAC de los access tokens (obligatorio en producción) |

### Migraciones

`database.AutoMigrate(db)` se ejecuta en `main` justo después de abrir la conexión:
crea o actualiza (sin borrar datos) las tablas del dominio: `users` (índice único
de email, índice de `deleted_at` y defaults `role = 'vendedor'`, `is_active = true`)
y `contacts` (índice de `deleted_at`; los booleanos no llevan default en la base a
propósito, los resuelve el service antes de insertar). Si falla, la app termina con
código 1 antes de aceptar peticiones.

`migrations/` queda reservada para las migraciones versionadas: cuando el esquema
empiece a evolucionar en producción conviene mover el `AutoMigrate` a un
`cmd/migrate` dedicado.

### Notas de Fiber v3

- Los handlers reciben `fiber.Ctx` **por valor**: en v3 `Ctx` es una interfaz
  (`type Handler = func(Ctx) error`), no un `*fiber.Ctx`.
- Las opciones de arranque (`DisableStartupMessage`, `GracefulContext`, prefork, TLS…)
  van en `fiber.ListenConfig`, que se pasa a `app.Listen(addr, cfg...)`;
  `fiber.Config` mantiene solo la configuración de la app y del servidor.
- Dependencia: `github.com/gofiber/fiber/v3 v3.6.1` (requiere Go 1.26+).

## Estructura del proyecto

Sigue el [Standard Go Project Layout](https://github.com/golang-standards/project-layout),
adaptado para no sobre-ingenierizar: solo `cmd/` e `internal/` tienen contenido real hoy.

```
backend/
├── api/                 # Contrato de la API (OpenAPI/Swagger)
│   └── openapi.yaml
├── cmd/
│   └── api/
│       └── main.go      # Punto de entrada: config, DB, Fiber, rutas y apagado ordenado
├── internal/            # Código privado del módulo: nadie fuera de este repo puede importarlo
│   ├── config/          # Carga de configuración (env, flags, archivos)
│   │   └── config.go
│   ├── handler/         # Handlers HTTP: parsean request y escriben response
│   │   ├── auth.go          # Endpoints de registro, login y perfil (/me)
│   │   ├── auth_dto.go      # Request/response y mapeo a DTOs (sin password_hash)
│   │   ├── contact.go       # CRUD de terceros (rutas protegidas por JWT)
│   │   ├── contact_dto.go   # Request/response de terceros
│   │   └── validate.go      # go-playground/validator + mensajes en español
│   ├── infrastructure/  # Adaptadores de infraestructura (drivers, clients externos)
│   │   ├── auth/
│   │   │   └── jwt.go       # Firma y validación de access tokens (HS256)
│   │   └── database/
│   │       ├── migrate.go   # AutoMigrate de los modelos del dominio
│   │       ├── contact_repository.go  # Implementación GORM de ContactRepository
│   │       ├── postgres.go  # Conexión a PostgreSQL con GORM
│   │       └── user_repository.go  # Implementación GORM de UserRepository
│   ├── middleware/      # Middlewares (logging, recovery, CORS, auth, request ID)
│   │   ├── auth.go          # Authenticate: valida el Bearer token y publica la identidad
│   │   ├── context.go       # Claves de c.Locals y helpers GetUserID/Email/Role
│   │   └── role.go          # RequireRole: autorización por rol (401/403)
│   ├── model/           # Entidades y DTOs del dominio
│   │   ├── contact.go       # Entidad Contact (tercero), soft delete y UUID
│   │   ├── errors.go        # Errores de dominio (email duplicado, tercero inexistente…)
│   │   └── user.go          # Entidad User, roles y default de bcrypt/rol
│   ├── repository/      # Contratos (puertos) de acceso a datos
│   │   ├── contact_repository.go  # ContactRepository y filtros de búsqueda
│   │   └── user_repository.go     # UserRepository (Interfaz)
│   ├── router/          # Registro de grupos de rutas (/api/v1)
│   │   └── router.go
│   └── service/         # Lógica de negocio (orquesta repositorios)
│       ├── auth.go          # Casos de uso Register, Login y Profile
│       └── contact.go       # Casos de uso CRUD de terceros
├── migrations/          # Migraciones de base de datos (versionadas)
├── pkg/                 # Código exportable y reutilizable por otros proyectos
├── scripts/             # Scripts de desarrollo, CI y despliegue
├── test/               # Pruebas de integración y e2e
├── .env.example
├── .gitignore
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### Por qué `main.go` vive en `cmd/api/`

En Go el paquete `main` no es importable, así que la convención es aislarlo en
`cmd/<binario>/`. Esto permite añadir más ejecutables (`cmd/worker`, `cmd/migrate`)
sin mezclar responsabilidades, y mantiene `main.go` como simple *wiring*: leer
configuración (`config.Load`), abrir la base de datos (`database.New`), construir la
app de Fiber (`newApp`), arrancar el servidor y gestionar el apagado ordenado con
`ShutdownWithContext`.

### Flujo de dependencias recomendado

```
cmd/api → router → handler → service → repository → model
                ↖ middleware       config ↗
                       infrastructure/database · infrastructure/auth ↗
```

Las dependencias apuntan siempre hacia adentro (`internal/`); `handler` no conoce
detalles de la base de datos y `repository` no conoce HTTP. Los puertos viven en el
dominio (`repository.UserRepository`, `service.TokenGenerator`) y sus
implementaciones en `infrastructure/`: `database.userRepository` (GORM) y
`auth.JWTManager` (JWT). Eso permite testear los casos de uso con dobles en
memoria, como hace `internal/service/auth_test.go`.

## Comandos útiles

| Comando      | Descripción                                   |
| ------------ | --------------------------------------------- |
| `make deps`  | Descarga y verifica las dependencias          |
| `make run`   | Ejecuta la API en modo desarrollo             |
| `make build` | Compila el binario en `bin/api`               |
| `make test`  | Pruebas con `-race` y cobertura               |
| `make vet`   | Análisis estático (`go vet`)                  |
| `make fmt`   | Formatea el código (`go fmt`)                 |
| `make tidy`  | Sincroniza `go.mod` / `go.sum`                |
| `make ci`    | `tidy` + `vet` + `test` + `build`             |
| `make clean` | Borra artefactos generados                    |

## Siguientes pasos sugeridos

1. Restringir `POST /api/v1/auth/register` a usuarios con rol `admin` (ya existe `middleware.RequireRole`).
2. Refresh tokens, rotación/revocación y rate limiting en login.
3. Mover `AutoMigrate` a un `cmd/migrate` con migraciones versionadas en `migrations/`.
4. Mover los handlers de ejemplo (`hello`, `health`) a `internal/handler` y añadir logging/recovery/CORS en `internal/middleware`.
5. Añadir `golangci-lint`, un `Dockerfile` multi-stage y CI (GitHub Actions).
