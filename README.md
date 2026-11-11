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

| Método   | Ruta                    | Respuesta                                        |
| -------- | ----------------------- | ------------------------------------------------ |
| `GET`    | `/`                     | `200 OK` · `{"message":"Hola mundo con Fiber"}`  |
| `GET`    | `/health`               | `200 OK` · `{"database":"up","status":"ok"}`     |
| `POST`   | `/api/v1/auth/register` | `201 Created` · datos del usuario + token        |
| `POST`   | `/api/v1/auth/login`    | `200 OK` · datos del usuario + token             |
| `GET`    | `/api/v1/auth/me`       | `200 OK` · usuario autenticado (requiere token)  |
| `PUT`    | `/api/v1/auth/me`       | `200 OK` · perfil propio actualizado (requiere token) |
| `PUT`    | `/api/v1/auth/me/password` | `204 No Content` · contraseña cambiada (requiere token) |
| `POST`   | `/api/v1/contacts`      | `201 Created` · tercero creado (requiere token)  |
| `GET`    | `/api/v1/contacts`      | `200 OK` · lista de terceros con filtros         |
| `GET`    | `/api/v1/contacts/:id`  | `200 OK` · tercero (requiere token)              |
| `PUT`    | `/api/v1/contacts/:id`  | `200 OK` · tercero actualizado (requiere token)  |
| `DELETE` | `/api/v1/contacts/:id`  | `204 No Content` · soft delete (requiere token)  |
| `POST`   | `/api/v1/products`      | `201 Created` · producto creado (requiere token) |
| `GET`    | `/api/v1/products`      | `200 OK` · lista de productos con filtros        |
| `GET`    | `/api/v1/products/:id`  | `200 OK` · producto (requiere token)             |
| `PUT`    | `/api/v1/products/:id`  | `200 OK` · producto actualizado (requiere token) |
| `DELETE` | `/api/v1/products/:id`  | `204 No Content` · soft delete (requiere token)  |
| `POST`   | `/api/v1/orders`        | `201 Created` · pedido con líneas (requiere token) |
| `GET`    | `/api/v1/orders`        | `200 OK` · lista de pedidos con filtros          |
| `GET`    | `/api/v1/orders/:id`    | `200 OK` · pedido con líneas (requiere token)    |
| `PUT`    | `/api/v1/orders/:id`    | `200 OK` · pedido actualizado (requiere token)   |
| `DELETE` | `/api/v1/orders/:id`    | `204 No Content` · soft delete (requiere token)  |

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

### Actualizar el perfil propio

`PUT /api/v1/auth/me` modifica el usuario dueño del token. El id sale de
`middleware.GetUserID(c)` (lo publica `Authenticate`), nunca del body, así que la
ruta no permite editar a otra persona.

```bash
curl -X PUT http://localhost:8080/api/v1/auth/me \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"full_name": "María Pérez Gómez", "phone": "1155559999"}'
```

Responde `200 OK` con el usuario actualizado (misma forma que `GET /auth/me`).

- Actualización **parcial**: solo se escriben los campos enviados; el resto conserva
  su valor actual (el DTO usa punteros para distinguir "no enviado" de "vacío").
- `email` se normaliza (minúsculas y sin espacios) y `full_name`/`phone` se recortan.
- `role` e `is_active` **no** son editables por esta vía: si llegan en el JSON se
  ignoran, porque cambiarlos es una operación de administración y aceptarlos acá
  sería una escalada de privilegios.
- El cambio de email es inmediato y no pide verificación. Los tokens ya emitidos
  siguen siendo válidos (el JWT guarda el email del momento de la emisión, pero las
  consultas usan el `user_id`), así que la respuesta de `GET /auth/me` ya muestra el
  email nuevo.

| Situación                                              | Código | Body                                             |
| ------------------------------------------------------ | ------ | ------------------------------------------------ |
| Email con formato inválido o campos fuera de rango     | `400`  | `{"error":"datos inválidos: ..."}`               |
| `full_name` con menos de 2 caracteres                   | `400`  | `{"error":"datos inválidos: full_name debe tener al menos 2 caracteres"}` |
| `full_name` que queda vacío al recortar espacios       | `400`  | `{"error":"el nombre completo es obligatorio"}`  |
| `email` vacío                                          | `400`  | `{"error":"el email es obligatorio"}`            |
| Email nuevo ya registrado por otro usuario             | `409`  | `{"error":"el email ya está registrado"}`        |
| Token ausente, inválido, o usuario inexistente/inactivo | `401`  | ver la tabla anterior                            |

### Cambiar la contraseña

`PUT /api/v1/auth/me/password` pide la contraseña actual como confirmación y
guarda la nueva hasheada: responde `204 No Content`, sin body.

```bash
curl -X PUT http://localhost:8080/api/v1/auth/me/password \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"current_password": "secreto123", "new_password": "nuevaClave123"}'
```

- `new_password` debe tener entre 8 y 72 caracteres (el tope de 72 existe porque
  bcrypt ignora silenciosamente lo que exceda 72 bytes).
- `current_password` se compara con el hash guardado; si no coincide responde `400`
  con `{"error":"la contraseña actual no es correcta"}` y no cambia nada.
- El hash nuevo se genera con bcrypt cost 12 y la contraseña nunca vuelve al cliente.
- Como el JWT es stateless, los access tokens ya emitidos **siguen vigentes**: la
  contraseña nueva aplica a los próximos logins. Revocar sesiones requiere una lista
  de revocación o un `token_version` en el usuario (ver siguientes pasos).

| Situación                                    | Código | Body                                                              |
| -------------------------------------------- | ------ | ----------------------------------------------------------------- |
| `current_password` incorrecta                | `400`  | `{"error":"la contraseña actual no es correcta"}`                 |
| `current_password` o `new_password` faltantes | `400`  | `{"error":"datos inválidos: current_password es obligatorio"}`   |
| `new_password` de menos de 8 caracteres      | `400`  | `{"error":"datos inválidos: new_password debe tener al menos 8 caracteres"}` |
| `new_password` de más de 72 caracteres       | `400`  | `{"error":"datos inválidos: new_password debe tener como máximo 72 caracteres"}` |
| Token ausente, inválido, o usuario inexistente/inactivo | `401`  | ver la tabla de `GET /auth/me`                          |

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
| Contraseña actual incorrecta al cambiarla                   | `400`  | `{"error":"la contraseña actual no es correcta"}`                 |
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

## Productos (catálogo)

Misma estructura de capas: entidad `model.Product`, puerto
`repository.ProductRepository`, adaptador GORM en
`internal/infrastructure/database/product_repository.go`, casos de uso en
`service.ProductService` y handlers en `internal/handler/product*.go`. Todas las
rutas viven en el grupo protegido, así que exigen
`Authorization: Bearer <token>`.

Una sola tabla cubre productos terminados, insumos y empaques: el mismo ítem
puede venderse, comprarse y/o producirse según sus flags (`is_sold`,
`is_purchased`, `is_produced`), que se combinan libremente y por defecto son
`false`. Solo `name` y `unit` son obligatorios; `track_stock` e `is_active`
arrancan en `true` (los resuelve el service, nunca la base). El stock y los
importes usan `github.com/shopspring/decimal` sobre columnas `numeric` de
PostgreSQL y se devuelven como números JSON con los dígitos exactos
(`current_stock`, `min_stock`: `numeric(14,3)`; `sale_price`, `cost`:
`numeric(14,2)`). `PUT` es una actualización parcial: solo cambia los campos
enviados.

### Crear

```bash
curl -X POST http://localhost:8080/api/v1/products \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Pan francés",
    "sku": "PAN-001",
    "description": "Pan francés clásico",
    "unit": "unidad",
    "is_sold": true,
    "is_purchased": false,
    "is_produced": true,
    "track_stock": true,
    "current_stock": 0,
    "min_stock": 20,
    "sale_price": 500,
    "cost": 180
  }'
```

```json
{
  "id": "0f0b6d3e-5b1a-4c8e-9f6a-2d7c1b9e4a10",
  "name": "Pan francés",
  "sku": "PAN-001",
  "description": "Pan francés clásico",
  "unit": "unidad",
  "is_sold": true,
  "is_purchased": false,
  "is_produced": true,
  "track_stock": true,
  "current_stock": 0,
  "min_stock": 20,
  "sale_price": 500,
  "cost": 180,
  "is_active": true,
  "created_at": "2026-11-01T09:00:00Z",
  "updated_at": "2026-11-01T09:00:00Z"
}
```

### Listar con filtros

```bash
# Todos los productos (ordenados por nombre)
curl http://localhost:8080/api/v1/products -H "Authorization: Bearer $TOKEN"

# Solo lo que se produce y coincide con "pan"
curl "http://localhost:8080/api/v1/products?is_produced=true&search=pan" \
  -H "Authorization: Bearer $TOKEN"

# Insumos que se compran y no se venden
curl "http://localhost:8080/api/v1/products?is_purchased=true&is_sold=false" \
  -H "Authorization: Bearer $TOKEN"
```

`search` ignora mayúsculas y busca en nombre, SKU y descripción. Los filtros
`is_sold`, `is_purchased`, `is_produced`, `is_active` y `track_stock` aceptan
`true`/`false`; cualquier otro valor responde `400`. Sin coincidencias la
respuesta es `[]` (nunca `null`).

### Obtener, actualizar y eliminar

```bash
ID=0f0b6d3e-5b1a-4c8e-9f6a-2d7c1b9e4a10

curl http://localhost:8080/api/v1/products/$ID -H "Authorization: Bearer $TOKEN"

# Actualización parcial: solo cambia el precio de venta y el stock mínimo
curl -X PUT http://localhost:8080/api/v1/products/$ID \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"sale_price": "550.75", "min_stock": "25"}'

# Soft delete: responde 204 y el producto deja de aparecer en la API
curl -X DELETE http://localhost:8080/api/v1/products/$ID \
  -H "Authorization: Bearer $TOKEN"
```

Los decimales se aceptan tanto como número (`500`) como cadena (`"500.00"`) y se
devuelven siempre con los dígitos guardados en la base.

| Situación                                         | Código | Body                                                  |
| ------------------------------------------------- | ------ | ----------------------------------------------------- |
| Body inválido o JSON malformado                    | `400`  | `{"error":"datos inválidos: name es obligatorio"}`     |
| `name` o `unit` vacíos en un `PUT`                 | `400`  | `{"error":"la unidad del producto es obligatoria"}`    |
| `:id` que no es UUID o filtro booleano inválido    | `400`  | `{"error":"el id debe ser un UUID válido"}`            |
| Producto inexistente o eliminado                   | `404`  | `{"error":"producto no encontrado"}`                   |
| Sin token o token inválido                         | `401`  | `{"error":"falta el header Authorization"}`            |

## Pedidos (ventas)

Misma estructura de capas: entidades `model.Order` y `model.OrderItem`, puerto
`repository.OrderRepository`, adaptador GORM en
`internal/infrastructure/database/order_repository.go`, casos de uso en
`service.OrderService` y handlers en `internal/handler/order*.go`. Todas las
rutas viven en el grupo protegido, así que exigen
`Authorization: Bearer <token>`.

Un pedido es una **cabecera** (`orders`) más sus **líneas** (`order_items`). El
único campo obligatorio es `contact_id` (el tercero tiene que existir) y, cuando
el pedido no es borrador, al menos una línea. Los importes usan
`github.com/shopspring/decimal` sobre `numeric(14,2)` y las cantidades sobre
`numeric(14,3)`; se devuelven como números JSON con los dígitos exactos.

Reglas que aplica el service:

- **Código interno:** `PED-YYYYMMDD-XXXX` (secuencia diaria en UTC con ceros a la
  izquierda). Se asigna dentro de la transacción y se reintenta hasta 5 veces si
  choca con el índice único; si no lo logra responde `409`.
- **Totales calculados:** `line_total = quantity × unit_price` redondeado a 2
  decimales, `subtotal` es la suma de las líneas y
  `final_amount = subtotal − discount_amount`. El descuento no puede ser negativo
  ni mayor que el subtotal.
- **Precio y snapshot:** el precio **no** se recibe del cliente: sale del
  `sale_price` del catálogo (tiene que ser mayor que cero) y, junto con
  `product_name` y `unit`, queda congelado en la línea. Editar el producto
  después no cambia el pedido.
- **Estados:** `status` nace `confirmed` (o `draft` si se pide) y
  `payment_status` nace `pending`. `delivery_type`, `payment_method` y
  `payment_status` son opcionales y se validan contra las constantes del dominio.
- **Entregas parciales:** `quantity_delivered` (en las líneas) y
  `parent_order_id` (en la cabecera) ya existen en el esquema, pero la API todavía
  no los escribe: quedan preparados para registrar entregas parciales sin migrar
  tablas.

### Crear

```bash
curl -X POST http://localhost:8080/api/v1/orders \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "contact_id": "3d2b1a54-8f0e-4c1a-9b7d-6a5c4e3d2b10",
    "delivery_type": "delivery",
    "delivery_at": "2026-11-02",
    "payment_method": "cash",
    "discount_amount": "50.00",
    "notes": "entregar antes de las 9",
    "items": [
      {"product_id": "0f0b6d3e-5b1a-4c8e-9f6a-2d7c1b9e4a10", "quantity": 12},
      {"product_id": "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d", "quantity": 2.5, "notes": "cortado fino"}
    ]
  }'
```

```json
{
  "id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "code": "PED-20261101-0001",
  "contact_id": "3d2b1a54-8f0e-4c1a-9b7d-6a5c4e3d2b10",
  "status": "confirmed",
  "ordered_at": "2026-11-01T09:00:00Z",
  "delivery_at": "2026-11-02T00:00:00Z",
  "delivery_type": "delivery",
  "subtotal": 8500,
  "discount_amount": 50,
  "final_amount": 8450,
  "payment_method": "cash",
  "payment_status": "pending",
  "amount_paid": 0,
  "notes": "entregar antes de las 9",
  "is_active": true,
  "created_at": "2026-11-01T09:00:00Z",
  "updated_at": "2026-11-01T09:00:00Z",
  "items": [
    {
      "id": "5b1f2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d",
      "product_id": "0f0b6d3e-5b1a-4c8e-9f6a-2d7c1b9e4a10",
      "product_name": "Pan francés",
      "unit": "unidad",
      "quantity": 12,
      "quantity_delivered": 0,
      "unit_price": 500,
      "line_total": 6000,
      "created_at": "2026-11-01T09:00:00Z",
      "updated_at": "2026-11-01T09:00:00Z"
    },
    {
      "id": "b2c3d4e5-f6a1-4b2c-9d3e-4f5a6b7c8d9e",
      "product_id": "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d",
      "product_name": "Pan de campo",
      "unit": "kg",
      "quantity": 2.5,
      "quantity_delivered": 0,
      "unit_price": 1000,
      "line_total": 2500,
      "notes": "cortado fino",
      "created_at": "2026-11-01T09:00:00Z",
      "updated_at": "2026-11-01T09:00:00Z"
    }
  ]
}
```

Un pedido en `draft` puede crearse sin líneas. `delivery_at` acepta
`2026-11-02` (medianoche UTC) o un RFC3339 completo.

### Listar con filtros

```bash
# Todos los pedidos (más recientes primero, sin líneas)
curl http://localhost:8080/api/v1/orders -H "Authorization: Bearer $TOKEN"

# Los de un cliente en un rango de fechas
curl "http://localhost:8080/api/v1/orders?contact_id=$CONTACT_ID&from=2026-11-01&to=2026-11-30" \
  -H "Authorization: Bearer $TOKEN"

# Pendientes de entrega que mencionan "mostrador"
curl "http://localhost:8080/api/v1/orders?status=confirmed&search=mostrador" \
  -H "Authorization: Bearer $TOKEN"
```

`from` y `to` acotan `ordered_at` y son inclusivos: un `to` sin hora cubre el día
completo (`23:59:59.999999999`). `search` ignora mayúsculas y busca en el código,
las notas y el nombre del tercero. `status` y `payment_status` se validan contra
las constantes del dominio. Sin coincidencias la respuesta es `[]` (nunca
`null`), y el listado devuelve solo cabeceras, sin `items`.

### Obtener, actualizar y eliminar

```bash
ID=7c9e6679-7425-40de-944b-e07fc1f90ae7

# Detalle con líneas
curl http://localhost:8080/api/v1/orders/$ID -H "Authorization: Bearer $TOKEN"

# Actualización parcial de la cabecera: solo cambia lo enviado
curl -X PUT http://localhost:8080/api/v1/orders/$ID \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"payment_status": "partial", "amount_paid": "4000.00", "delivery_at": "2026-11-03T09:00:00Z"}'

# Reemplazo de líneas: se conserva la que lleva id y se agrega la que no lo lleva
curl -X PUT http://localhost:8080/api/v1/orders/$ID \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "items": [
      {"id": "5b1f2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d", "quantity": 15},
      {"product_id": "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d", "quantity": 4}
    ]
  }'

# Soft delete de la cabecera y sus líneas: responde 204
curl -X DELETE http://localhost:8080/api/v1/orders/$ID \
  -H "Authorization: Bearer $TOKEN"
```

En un `PUT`, `items` **ausente** deja las líneas como están; `items` **presente**
(aunque sea `[]`) define el conjunto final: las líneas que no aparecen se borran
lógicamente. Una línea con `id` modifica la existente (`quantity`, `product_id`,
`notes`) y otra sin `id` agrega una nueva. Las líneas con `quantity_delivered > 0`
no se pueden quitar y su cantidad no puede quedar por debajo de lo entregado;
además, un pedido que no es borrador no puede quedarse sin líneas. Enviar
`delivery_at` o `notes` vacíos limpia el campo.

| Situación                                                     | Código | Body                                                                                    |
| ------------------------------------------------------------- | ------ | --------------------------------------------------------------------------------------- |
| Body inválido, JSON malformado o `contact_id` sin UUID         | `400`  | `{"error":"datos inválidos: contact_id es obligatorio"}`                                  |
| `id` de la ruta sin UUID (GET/PUT/DELETE)                      | `400`  | `{"error":"datos inválidos: id debe ser un UUID válido"}`                                 |
| Tercero inexistente                                            | `400`  | `{"error":"el tercero indicado no existe"}`                                              |
| Producto inexistente                                           | `400`  | `{"error":"el producto indicado no existe"}`                                             |
| Pedido sin líneas y sin ser borrador                           | `400`  | `{"error":"el pedido debe tener al menos un ítem"}`                                      |
| Descuento negativo o mayor que el subtotal                     | `400`  | `{"error":"el descuento no puede ser negativo ni superar el subtotal del pedido"}`       |
| Importe cobrado negativo                                       | `400`  | `{"error":"el importe cobrado no puede ser negativo"}`                                   |
| Estado, tipo de entrega, medio o estado de pago inválido        | `400`  | `{"error":"el estado del pedido no es válido"}`                                          |
| Cantidad nula, negativa o menor a lo ya entregado              | `400`  | `{"error":"la cantidad del ítem debe ser mayor que cero y no menor a lo ya entregado"}`  |
| Producto sin precio de venta                                   | `400`  | `{"error":"el producto no tiene un precio de venta válido"}`                             |
| Línea con `id` que no pertenece al pedido                      | `400`  | `{"error":"el ítem indicado no pertenece al pedido"}`                                    |
| Línea repetida en el body                                      | `400`  | `{"error":"el ítem aparece más de una vez en el pedido"}`                                |
| Línea entregada que se intenta quitar                          | `400`  | `{"error":"no se puede quitar un ítem con cantidad entregada"}`                          |
| Filtro `from`/`to` con formato inválido                        | `400`  | `{"error":"from debe ser una fecha ISO 8601 (por ejemplo 2025-03-01 o 2025-03-01T15:04:05Z)"}` |
| Pedido inexistente o eliminado                                 | `404`  | `{"error":"pedido no encontrado"}`                                                       |
| No se pudo generar el código en 5 intentos                     | `409`  | `{"error":"no se pudo generar el código del pedido, intente de nuevo"}`                   |
| Sin header `Authorization`                                     | `401`  | `{"error":"falta el header Authorization"}`                                              |
| Token inválido o expirado                                      | `401`  | `{"error":"token inválido o expirado"}`                                                  |

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
de email, índice de `deleted_at` y defaults `role = 'vendedor'`, `is_active = true`),
`contacts` y `products` (ambas con índice de `deleted_at`; los booleanos no llevan
default en la base a propósito, los resuelve el service antes de insertar). En
`products` el stock se guarda en `numeric(14,3)` y los importes en
`numeric(14,2)`, mapeados a `decimal.Decimal`. En `orders`/`order_items` la
cabecera tiene índice único en `code` y las líneas guardan `order_id` y
`product_id`; la clave foránea contra `products` usa `ON DELETE RESTRICT`, así que
no se puede borrar un producto que ya está en un pedido. Si falla, la app termina
con código 1 antes de aceptar peticiones.

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
│   │   ├── auth.go          # Endpoints de registro, login y perfil propio (GET/PUT /me)
│   │   ├── auth_dto.go      # Request/response y mapeo a DTOs (sin password_hash)
│   │   ├── contact.go       # CRUD de terceros (rutas protegidas por JWT)
│   │   ├── contact_dto.go   # Request/response de terceros
│   │   ├── product.go       # CRUD del catálogo de productos (rutas protegidas por JWT)
│   │   ├── product_dto.go   # Request/response de productos (decimales como números JSON)
│   │   ├── order.go         # CRUD de pedidos (cabecera + líneas, rutas protegidas por JWT)
│   │   ├── order_dto.go     # Request/response de pedidos y parseo de fechas/importes
│   │   └── validate.go      # go-playground/validator + mensajes en español
│   ├── infrastructure/  # Adaptadores de infraestructura (drivers, clients externos)
│   │   ├── auth/
│   │   │   └── jwt.go       # Firma y validación de access tokens (HS256)
│   │   └── database/
│   │       ├── migrate.go   # AutoMigrate de los modelos del dominio
│   │       ├── contact_repository.go  # Implementación GORM de ContactRepository
│   │       ├── order_repository.go  # Implementación GORM de OrderRepository (transacciones)
│   │       ├── postgres.go  # Conexión a PostgreSQL con GORM
│   │       ├── product_repository.go  # Implementación GORM de ProductRepository
│   │       └── user_repository.go  # Implementación GORM de UserRepository
│   ├── middleware/      # Middlewares (logging, recovery, CORS, auth, request ID)
│   │   ├── auth.go          # Authenticate: valida el Bearer token y publica la identidad
│   │   ├── context.go       # Claves de c.Locals y helpers GetUserID/Email/Role
│   │   └── role.go          # RequireRole: autorización por rol (401/403)
│   ├── model/           # Entidades y DTOs del dominio
│   │   ├── contact.go       # Entidad Contact (tercero), soft delete y UUID
│   │   ├── errors.go        # Errores de dominio (email duplicado, tercero inexistente…)
│   │   ├── order.go         # Entidades Order/OrderItem, estados y código PED-YYYYMMDD-XXXX
│   │   ├── product.go       # Entidad Product (flags, decimales, soft delete y UUID)
│   │   └── user.go          # Entidad User, roles y default de bcrypt/rol
│   ├── repository/      # Contratos (puertos) de acceso a datos
│   │   ├── contact_repository.go  # ContactRepository y filtros de búsqueda
│   │   ├── order_repository.go    # OrderRepository (cabecera, líneas y transacciones)
│   │   ├── product_repository.go  # ProductRepository y filtros del catálogo
│   │   └── user_repository.go     # UserRepository (Interfaz)
│   ├── router/          # Registro de grupos de rutas (/api/v1)
│   │   └── router.go
│   └── service/         # Lógica de negocio (orquesta repositorios)
│       ├── auth.go          # Casos de uso Register, Login, Profile, UpdateProfile y ChangePassword
│       ├── contact.go       # Casos de uso CRUD de terceros
│       ├── order.go         # Casos de uso de pedidos: totales, códigos y líneas
│       └── product.go       # Casos de uso CRUD del catálogo y reglas de stock
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
6. Registrar entregas parciales sobre `order_items.quantity_delivered` (el esquema ya lo soporta, ver más arriba) y descontar stock al confirmar el pedido.
