# ZebraGateway

[中文](./README.md) | English

🌐 A lightweight API gateway built with Go / Gin, integrating with ZebraRBAC for permission management. Performs JWT authentication and fine-grained permission validation at the gateway layer, and reverse-proxies legitimate requests to backend microservices. Supports **dynamic management of routes and whitelists** via PostgreSQL with hot-reload without restart. Provides management REST API and CLI tools.

![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)
![Gin](https://img.shields.io/badge/Gin-1.11-00ADD8?logo=gin&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-17-4169E1?logo=postgresql&logoColor=white)
![JWT](https://img.shields.io/badge/JWT-HS256-000000?logo=jsonwebtokens&logoColor=white)
![Swagger](https://img.shields.io/badge/Swagger-OpenAPI-85EA2D?logo=swagger&logoColor=black)
![License](https://img.shields.io/badge/License-MIT-green)

---

## ⚙️ Tech Stack

| Category             | Tech / Version                                                                                                          |
| -------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| **Language**         | Go 1.24                                                                                                                 |
| **Web Framework**    | [Gin](https://github.com/gin-gonic/gin) v1.11                                                                           |
| **CORS**             | [gin-contrib/cors](https://github.com/gin-contrib/cors) v1.7.6                                                          |
| **JWT Verify**       | [golang-jwt/jwt](https://github.com/golang-jwt/jwt) v5.2.1, HS256 algorithm                                             |
| **Permission Cache** | [patrickmn/go-cache](https://github.com/patrickmn/go-cache) v2.1 (in-memory TTL cache)                                  |
| **Config Mgmt**      | [Viper](https://github.com/spf13/viper) v1.21 (YAML + environment variable override)                                    |
| **Logging**          | [Zap](https://github.com/uber-go/zap) v1.27 + [Lumberjack](https://github.com/natefinch/lumberjack) v2.2 (log rotation) |
| **Reverse Proxy**    | Go standard library `net/http/httputil.ReverseProxy`                                                                    |
| **Dynamic Routes**   | [GORM](https://gorm.io) v1.25 + [gorm/driver/postgres](https://github.com/go-gorm/postgres) v1.5 (PostgreSQL)           |
| **CLI Tool**         | [cobra](https://github.com/spf13/cobra) v1.9                                                                            |
| **API Docs**         | [swaggo/swag](https://github.com/swaggo/swag) v1.16.6 + [gin-swagger](https://github.com/swaggo/gin-swagger) v1.6.1     |
| **Auth Service**     | ZebraRBAC (FastAPI, `GET /api/authorization`)                                                                           |

---

## 📐 Architecture Model

```
                          ┌──────────────────────────────────────┐
  Client                  │            ZebraGateway               │
  ─────                   │               :8080                   │
  Browser / App  ─HTTP──► │                                       │
                          │  ┌─────────────────────────────────┐  │
                          │  │       RequestLogger MW          │  │  Log requests
                          │  └──────────────┬──────────────────┘  │
                          │                 ▼                      │
                          │  ┌─────────────────────────────────┐  │
                          │  │           Auth MW               │  │
                          │  │  1. Static whitelist check      │  │
                          │  │  2. Dynamic whitelist check (DB)│  │
                          │  │  3. Extract Bearer Token        │  │
                          │  │  4. Local JWT HS256 verify      │  │◄──► ZebraRBAC :8000
                          │  │  5. Check permission cache      │  │    GET /api/authorization
                          │  │  6. Cache miss → call RBAC      │  │
                          │  │  7. Permission matching         │  │
                          │  │  8. Inject X-User-Id header     │  │
                          │  └──────────────┬──────────────────┘  │
                          │                 ▼                      │
                          │  ┌─────────────────────────────────┐  │
                          │  │       router.Manager            │  │
                          │  │  Longest prefix matching        │  │◄──► PostgreSQL
                          │  │  Path rewrite + proxy           │  │    Route persistence
                          │  │  (Hot reload, 30s DB sync)      │  │
                          │  └──────────────┬──────────────────┘  │
                          └─────────────────┼──────────────────────┘
                                            │
                       ┌────────────────────┼───────────────────────┐
                       ▼                                             ▼
                ZebraRBAC :8000                         Other backend services
               /rbac/* → /api/*                      /<service>/* → /*
```

### Auth Middleware Detailed Flow

```
Request arrives at gateway
    │
    ├─[1] Remove spoofed X-User-Id / X-User-Name headers (prevent injection)
    │
    ├─[2] Path in static whitelist (YAML)? ──── YES ──► Allow directly
    │
    ├─[3] Path in dynamic whitelist (DB)?  ──── YES ──► Allow directly
    │         NO
    ▼
  Extract Authorization: Bearer <token>
    │  No token ──► 401 Missing auth header
    ▼
  Local HS256 verify JWT signature, extract sub field = userID
    │  Invalid/expired signature ──► 401 Invalid Token
    ▼
  Check in-memory cache cache[userID]
    ├── Hit ──► Use cached permission data
    └── Miss ──► Call ZebraRBAC GET /api/authorization (with original token)
                        │  Failed ──► 401 Permission fetch failed
                        └─ Success ──► Write to cache (TTL configurable)
    │
    ├─ permissions.all == true (super admin) ──────────────────────► Allow
    │
    └─ Check request.path ∈ permissions.functions list
           ├── Support exact match："/publish/tasks"
           └── Support prefix wildcard："/publish/tasks/*"
               │
               ├── Matched ──► Allow, inject X-User-Id / X-User-Name headers
               └── Not matched ──► 403 Permission denied
```

---

## 📁 Directory Structure

```
ZebraGateway/
├── main.go                          # Entry point: initialize config, DB, route manager
├── go.mod
├── cmd/
│   └── cli/
│       └── main.go                  # zebra-gw CLI tool (Cobra)
├── config/
│   ├── config.go                    # Viper config loading struct
│   └── configs.yaml                 # Default config (supports env var override)
├── docs/                            # swag auto-generated (swagger.json / swagger.yaml / docs.go)
├── internal/
│   ├── api/
│   │   └── api.go                   # Named handler (for swag doc generation)
│   ├── handler/
│   │   └── route.go                 # Management API: route & whitelist CRUD
│   ├── middleware/
│   │   ├── auth.go                  # JWT parse + RBAC permission check middleware (core)
│   │   ├── proxy.go                 # httputil.ReverseProxy reverse proxy (fallback)
│   │   └── requestLogger.go         # Zap structured request logging middleware
│   ├── model/
│   │   └── route.go                 # GORM models: ServiceRoute, WhitelistRoute
│   ├── rbac/
│   │   └── client.go                # RBAC HTTP client, call /api/authorization
│   ├── router/
│   │   └── manager.go               # Dynamic route manager (hot reload, longest prefix match)
│   ├── store/
│   │   └── store.go                 # PostgreSQL connection + AutoMigrate
│   └── types/
│       └── types.go                 # Common types: Response, RBACAuthData, ContextKey
├── pkg/
│   ├── cache/
│   │   └── cache.go                 # go-cache permission cache wrapper (configurable TTL)
│   └── log/
│       └── logger.go                # Zap + Lumberjack logger initialization
└── logs/
    └── app.log                      # Runtime logs (auto-created)
```

---

## 🔧 Configuration

Config file path: `config/configs.yaml`. All fields can be overridden via environment variables `ZEBRA_GW_APP_<FIELD_NAME>` (auto-bound by Viper).

```yaml
app:
  Port: "8080" # Gateway listen port
  JWTSecret: "..." # Must match ZebraRBAC SECRET_KEY
  RbacURL: "http://..." # ZebraRBAC service URL
  CacheTTL: 300 # Permission cache TTL (seconds), 0 to disable
  DatabaseURL:
    "postgres://user:pass@host:5432/zebra_gateway?sslmode=disable"
    # PostgreSQL DSN for route persistence
  RouteReloadInterval: "30s" # Route auto-reload interval (supports s/m/h)

# Initial static routes (imported to DB on first startup only)
services:
  - prefix: "/rbac"
    target: "http://..."
    rewrite: "/api"

# Initial static whitelist (imported to DB on first startup only)
whitelist:
  - method: "POST"
    path: "/rbac/login/access-token"
  - method: "GET"
    path: "/swagger/*"
```

> **Auto-init on first startup**: If route table is empty in DB, the gateway will auto-import entries from `services` and `whitelist` to database. Subsequent changes should use management API or CLI tool.

### Route Forwarding Rules

| Client Request Path             | Forward To                                | Description                   |
| ------------------------------- | ----------------------------------------- | ----------------------------- |
| `POST /rbac/login/access-token` | `http://rbac:8000/api/login/access-token` | Whitelist, pass through       |
| `GET /rbac/users`               | `http://rbac:8000/api/users`              | Strip `/rbac` → append `/api` |

Routes use **longest prefix first** matching strategy. More precise prefixes take precedence.

---

## 🔄 Dynamic Route Management

Routes and whitelists are persisted in PostgreSQL. Three management methods supported:

### Method 1: REST Management API

All management endpoints prefixed with `/admin` and require JWT authentication.

#### Service Routes

| Method   | Path                        | Description               |
| -------- | --------------------------- | ------------------------- |
| `GET`    | `/admin/routes`             | List all routes           |
| `POST`   | `/admin/routes`             | Create new route          |
| `PUT`    | `/admin/routes/:id`         | Update route              |
| `DELETE` | `/admin/routes/:id`         | Delete route              |
| `POST`   | `/admin/routes/:id/enable`  | Enable route              |
| `POST`   | `/admin/routes/:id/disable` | Disable route             |
| `POST`   | `/admin/routes/reload`      | Trigger manual hot reload |

Create route request body example:

```json
{
  "prefix": "/api-v2",
  "target": "http://192.168.1.100:9000",
  "rewrite": "",
  "description": "v2 service",
  "enabled": true
}
```

#### Whitelist

| Method   | Path                    | Description      |
| -------- | ----------------------- | ---------------- |
| `GET`    | `/admin/whitelists`     | List all entries |
| `POST`   | `/admin/whitelists`     | Create entry     |
| `DELETE` | `/admin/whitelists/:id` | Delete entry     |

Create whitelist entry request body example:

```json
{
  "method": "GET",
  "path": "/api-v2/public/*",
  "description": "Public API, no auth required"
}
```

> Any write operation on routes/whitelists immediately triggers memory snapshot update. **Changes take effect in real-time**, no restart needed.

### Method 2: CLI Tool (zebra-gw)

Build CLI:

```bash
go build -o zebra-gw.exe ./cmd/cli/
```

**Route Management:**

```bash
# List all routes
zebra-gw route list

# Add route
zebra-gw route add -p /api-v2 -t http://192.168.1.100:9000 -r "" -d "v2 service"

# Update route
zebra-gw route update 3 --target http://192.168.1.101:9000

# Enable / disable route
zebra-gw route enable 3
zebra-gw route disable 3

# Delete route
zebra-gw route delete 3
```

**Whitelist Management:**

```bash
# List all whitelists
zebra-gw whitelist list

# Add whitelist (* method matches any HTTP method)
zebra-gw whitelist add -m GET -p /api-v2/health -d "Health check"
zebra-gw whitelist add -p "/api-v2/public/*"      # Default method=*

# Delete whitelist
zebra-gw whitelist delete 5
```

> CLI tool directly reads `config/configs.yaml` to connect to database. Changes **won't** immediately reflect to running gateway instance (wait for next `RouteReloadInterval` auto-refresh, or call `POST /admin/routes/reload` to trigger manually).

### Method 3: Hot-reload Mechanism

- **Write-triggered**: After adding/updating/deleting routes/whitelists via management API, memory snapshot updates **immediately** and atomically
- **Periodic sync**: Every `RouteReloadInterval` (default 30s) fully sync from DB to ensure multi-instance consistency
- **Manual trigger**: `POST /admin/routes/reload`

---

## 📤 Upstream Request Header Injection

After successful authentication, the gateway injects the following headers to upstream services:

| Header        | Description            |
| ------------- | ---------------------- |
| `X-User-Id`   | JWT sub field (userID) |
| `X-User-Name` | Username from RBAC     |

Upstream services can directly read these headers to identify the caller, no need to verify JWT again.

---

## 🚀 Quick Start

### Prerequisites

- Go 1.20+
- PostgreSQL instance (for route persistence), pre-create database `zebra_gateway`
- ZebraRBAC running (default `:8000`)
- `JWTSecret` must match ZebraRBAC `SECRET_KEY`

### Local Development

```bash
cd d:\Z\ZebraOps\ZebraGateway

# Download dependencies
go mod tidy

# Edit config (set DatabaseURL, RbacURL)
vim config/configs.yaml

# Start gateway (first startup auto-creates table and imports initial routes from YAML)
go run main.go
```

### Production Deployment (Environment Variable Override)

```bash
export ZEBRA_GW_APP_PORT=8080
export ZEBRA_GW_APP_JWTSECRET=<prod-secret>
export ZEBRA_GW_APP_RBACURL=http://zebra-rbac:8000
export ZEBRA_GW_APP_DATABASEURL=postgres://user:pass@pg-host:5432/zebra_gateway?sslmode=disable

go build -o zebra-gateway . && ./zebra-gateway
```

---

## 📋 Endpoint List

### Gateway Self

| Method | Path         | Auth | Description                 |
| ------ | ------------ | ---- | --------------------------- |
| `GET`  | `/health`    | None | Gateway health check        |
| `GET`  | `/swagger/*` | None | Swagger UI docs (JSON/YAML) |

### Management API

| Method   | Path                        | Auth | Description               |
| -------- | --------------------------- | ---- | ------------------------- |
| `GET`    | `/admin/routes`             | JWT  | List all service routes   |
| `POST`   | `/admin/routes`             | JWT  | Create service route      |
| `PUT`    | `/admin/routes/:id`         | JWT  | Update service route      |
| `DELETE` | `/admin/routes/:id`         | JWT  | Delete service route      |
| `POST`   | `/admin/routes/:id/enable`  | JWT  | Enable service route      |
| `POST`   | `/admin/routes/:id/disable` | JWT  | Disable service route     |
| `POST`   | `/admin/routes/reload`      | JWT  | Trigger manual hot reload |
| `GET`    | `/admin/whitelists`         | JWT  | List all whitelists       |
| `POST`   | `/admin/whitelists`         | JWT  | Create whitelist entry    |
| `DELETE` | `/admin/whitelists/:id`     | JWT  | Delete whitelist entry    |

### Proxy Routes (Dynamic, DB-configured)

| Method | Path      | Auth                   | Description        |
| ------ | --------- | ---------------------- | ------------------ |
| `ANY`  | `/rbac/*` | JWT (whitelist except) | Proxy to ZebraRBAC |

### Swagger UI Access

After starting the gateway, open in browser:

```
http://localhost:8080/swagger/index.html
```

Swagger UI is fully open and **requires no JWT** for viewing docs. To test protected endpoints in Swagger UI, click **Authorize** in the top-right corner and input `Bearer <token>`.

> `<token>` is obtained via `POST /rbac/login/access-token`.

After modifying endpoint comments, regenerate docs by running in project root:

```bash
swag init --parseDependency --parseInternal
```

---

## 🚨 HTTP Status Codes

| HTTP Status | Code | Description                                   |
| ----------- | ---- | --------------------------------------------- |
| 200         | 200  | Success                                       |
| 400         | 400  | Bad request (invalid parameters)              |
| 401         | 401  | Unauthorized (no token / invalid / auth fail) |
| 403         | 403  | Forbidden (authenticated but no permission)   |
| 404         | 404  | Route not found                               |
| 500         | 500  | Internal server error                         |
| 502         | 502  | Bad gateway (upstream unavailable)            |

---

## 🔗 Relationship with Other Zebra Services

```
ZebraAdmin (React)           ← Frontend management interface
    │  HTTP
    ▼
ZebraGateway (This Project)  ← Unified entry, auth & dynamic routing
    ├──► ZebraRBAC (Python/FastAPI)   ← Permission data source
    ├──► ZebraCICD (Go)               ← CI/CD pipeline
    └──► Other backend services        ← Business services

ZebraDeployment              ← Docker Compose one-click deployment

PostgreSQL                   ← ZebraGateway route persistence (zebra_gateway db)
MySQL                        ← ZebraRBAC permission data persistence (zebra_rbac db)
```

---

## 📄 License

[MIT](./LICENSE)
