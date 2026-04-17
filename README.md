# ZebraGateway

[English](./README.en.md) | 中文

🌐 基于 Go / Gin 实现的轻量级 API 网关，对接 ZebraRBAC 权限系统，在网关层完成 JWT 认证与细粒度权限校验，将合法请求反向代理到后端各微服务。支持通过 PostgreSQL **动态管理路由与白名单**，无需重启即可热更新，并提供管理 REST API 和 CLI 工具。

![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)
![Gin](https://img.shields.io/badge/Gin-1.11-00ADD8?logo=gin&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-17-4169E1?logo=postgresql&logoColor=white)
![JWT](https://img.shields.io/badge/JWT-HS256-000000?logo=jsonwebtokens&logoColor=white)
![Swagger](https://img.shields.io/badge/Swagger-OpenAPI-85EA2D?logo=swagger&logoColor=black)
![License](https://img.shields.io/badge/License-MIT-green)

---

## ⚙️ 技术栈

| 类别         | 技术 / 版本                                                                                                          |
| ------------ | -------------------------------------------------------------------------------------------------------------------- |
| **语言**     | Go 1.24                                                                                                              |
| **Web 框架** | [Gin](https://github.com/gin-gonic/gin) v1.11                                                                        |
| **跨域**     | [gin-contrib/cors](https://github.com/gin-contrib/cors) v1.7.6                                                       |
| **JWT 验证** | [golang-jwt/jwt](https://github.com/golang-jwt/jwt) v5.2.1，算法 HS256                                               |
| **权限缓存** | [patrickmn/go-cache](https://github.com/patrickmn/go-cache) v2.1（内存 TTL 缓存）                                    |
| **配置管理** | [Viper](https://github.com/spf13/viper) v1.21（YAML + 环境变量覆盖）                                                 |
| **日志**     | [Zap](https://github.com/uber-go/zap) v1.27 + [Lumberjack](https://github.com/natefinch/lumberjack) v2.2（日志轮转） |
| **反向代理** | Go 标准库 `net/http/httputil.ReverseProxy`                                                                           |
| **动态路由** | [GORM](https://gorm.io) v1.25 + [gorm/driver/postgres](https://github.com/go-gorm/postgres) v1.5（PostgreSQL）       |
| **CLI 工具** | [cobra](https://github.com/spf13/cobra) v1.9                                                                         |
| **API 文档** | [swaggo/swag](https://github.com/swaggo/swag) v1.16.6 + [gin-swagger](https://github.com/swaggo/gin-swagger) v1.6.1  |
| **权限服务** | ZebraRBAC（FastAPI，`GET /api/authorization`）                                                                       |

---

## 📐 架构模型

```
                          ┌──────────────────────────────────────┐
  Client                  │            ZebraGateway               │
  ─────                   │               :8080                   │
  Browser / App  ─HTTP──► │                                       │
                          │  ┌─────────────────────────────────┐  │
                          │  │       RequestLogger MW          │  │  记录请求日志
                          │  └──────────────┬──────────────────┘  │
                          │                 ▼                      │
                          │  ┌─────────────────────────────────┐  │
                          │  │           Auth MW               │  │
                          │  │  1. 静态白名单检查（YAML）      │  │
                          │  │  2. 动态白名单检查（DB）        │  │
                          │  │  3. 提取 Bearer Token           │  │
                          │  │  4. 本地 JWT HS256 验证         │  │◄──► ZebraRBAC :8000
                          │  │  5. 查权限缓存(TTL 5m)          │  │    GET /api/authorization
                          │  │  6. 未命中→调用RBAC             │  │
                          │  │  7. 权限列表匹配                │  │
                          │  │  8. 注入 X-User-Id 头           │  │
                          │  └──────────────┬──────────────────┘  │
                          │                 ▼                      │
                          │  ┌─────────────────────────────────┐  │
                          │  │       router.Manager            │  │
                          │  │  最长前缀匹配 + 路径重写 + 转发  │  │◄──► PostgreSQL
                          │  │  （热更新，30s 自动同步 DB）     │  │    路由配置持久化
                          │  └──────────────┬──────────────────┘  │
                          └─────────────────┼──────────────────────┘
                                            │
                       ┌────────────────────┼───────────────────────┐
                       ▼                                             ▼
                ZebraRBAC :8000                         其他后端服务
               /rbac/* → /api/*                      /<service>/* → /*
```

### 鉴权中间件详细流程

```
请求到达网关
    │
    ├─[1] 删除客户端伪造的 X-User-Id / X-User-Name 请求头（防注入）
    │
    ├─[2] 路径在静态白名单（YAML）？ ──── YES ──► 直接放行
    │
    ├─[3] 路径在动态白名单（DB）？   ──── YES ──► 直接放行
    │         NO
    ▼
  提取 Authorization: Bearer <token>
    │  无 token ──► 401 缺少认证头
    ▼
  本地 HS256 验证 JWT 签名，取 sub 字段 = userID
    │  签名非法/过期 ──► 401 无效 Token
    ▼
  查内存缓存 cache[userID]
    ├── 命中 ──► 使用缓存的权限数据
    └── 未命中 ──► 调用 ZebraRBAC GET /api/authorization（携带原始 token）
                        │  失败 ──► 401 权限获取失败
                        └─ 成功 ──► 写入缓存（TTL 由配置决定）
    │
    ├─ permissions.all == true（超级管理员）────────────────────► 放行
    │
    └─ 检查 request.path ∈ permissions.functions 列表
           ├── 支持精确匹配："/publish/tasks"
           └── 支持前缀通配："/publish/tasks/*"
               │
               ├── 匹配 ──► 放行，注入 X-User-Id / X-User-Name 请求头
               └── 不匹配 ──► 403 权限不足
```

---

## 📁 目录结构

```
ZebraGateway/
├── main.go                          # 入口：初始化配置、DB、路由管理器
├── go.mod
├── cmd/
│   └── cli/
│       └── main.go                  # zebra-gw CLI 工具（Cobra）
├── config/
│   ├── config.go                    # Viper 配置加载结构体
│   └── configs.yaml                 # 默认配置（支持环境变量覆盖）
├── docs/                            # swag 自动生成（swagger.json / swagger.yaml / docs.go）
├── internal/
│   ├── api/
│   │   └── api.go                   # 具名 handler（供 swag 生成文档）
│   ├── handler/
│   │   └── route.go                 # 管理 API：路由 & 白名单 CRUD
│   ├── middleware/
│   │   ├── auth.go                  # JWT 解析 + RBAC 权限校验中间件（核心）
│   │   ├── proxy.go                 # httputil.ReverseProxy 反向代理（静态路由备用）
│   │   └── requestLogger.go         # Zap 结构化请求日志中间件
│   ├── model/
│   │   └── route.go                 # GORM 模型：ServiceRoute、WhitelistRoute
│   ├── rbac/
│   │   └── client.go                # RBAC HTTP 客户端，调用 /api/authorization
│   ├── router/
│   │   └── manager.go               # 动态路由管理器（热更新、最长前缀匹配）
│   ├── store/
│   │   └── store.go                 # PostgreSQL 连接 + AutoMigrate
│   └── types/
│       └── types.go                 # 公共类型：Response、RBACAuthData、ContextKey
├── pkg/
│   ├── cache/
│   │   └── cache.go                 # go-cache 权限缓存封装（TTL 可配置）
│   └── log/
│       └── logger.go                # Zap + Lumberjack 日志初始化
└── logs/
    └── app.log                      # 运行时日志（自动创建）
```

---

## 🔧 配置说明

配置文件路径：`config/configs.yaml`，所有字段均可通过环境变量 `ZEBRA_GW_APP_<字段名>` 覆盖（Viper 自动绑定）。

```yaml
app:
  Port: "8080" # 网关监听端口
  JWTSecret: "..." # 与 ZebraRBAC SECRET_KEY 保持一致
  RbacURL: "http://..." # ZebraRBAC 服务地址
  CacheTTL: 300 # 权限缓存有效期（秒），设为 0 禁用缓存
  DatabaseURL:
    "postgres://user:pass@host:5432/zebra_gateway?sslmode=disable"
    # PostgreSQL DSN，用于动态路由持久化
  RouteReloadInterval: "30s" # 路由自动热更新间隔（支持 s/m/h）

# 初始静态路由（仅首次启动时导入 DB，之后以 DB 为准）
services:
  - prefix: "/rbac"
    target: "http://..."
    rewrite: "/api"

# 初始静态白名单（仅首次启动时导入 DB，之后以 DB 为准）
whitelist:
  - method: "POST"
    path: "/rbac/login/access-token"
  - method: "GET"
    path: "/swagger/*"
```

> **首次启动自动初始化**：若数据库中路由表为空，网关会将 `services` 和 `whitelist` 中的条目自动导入数据库，后续修改请使用管理 API 或 CLI 工具。

### 路径转发规则

| 客户端请求路径                  | 转发到                                    | 说明                        |
| ------------------------------- | ----------------------------------------- | --------------------------- |
| `POST /rbac/login/access-token` | `http://rbac:8000/api/login/access-token` | 白名单，直接透传            |
| `GET /rbac/users`               | `http://rbac:8000/api/users`              | strip `/rbac` → 拼接 `/api` |

路由采用**最长前缀优先**匹配策略，更精确的前缀优先生效。

---

## 🔄 动态路由管理

路由和白名单数据持久化在 PostgreSQL 中，支持三种管理方式：

### 方式一：REST 管理 API

所有管理接口以 `/admin` 为前缀，需要 JWT 鉴权。

#### 服务路由

| 方法     | 路径                        | 说明                   |
| -------- | --------------------------- | ---------------------- |
| `GET`    | `/admin/routes`             | 列出所有路由           |
| `POST`   | `/admin/routes`             | 新增路由               |
| `PUT`    | `/admin/routes/:id`         | 更新路由               |
| `DELETE` | `/admin/routes/:id`         | 删除路由               |
| `POST`   | `/admin/routes/:id/enable`  | 启用路由               |
| `POST`   | `/admin/routes/:id/disable` | 禁用路由               |
| `POST`   | `/admin/routes/reload`      | 手动触发内存快照热更新 |

新增路由请求体示例：

```json
{
  "prefix": "/api-v2",
  "target": "http://192.168.1.100:9000",
  "rewrite": "",
  "description": "v2 服务",
  "enabled": true
}
```

#### 白名单

| 方法     | 路径                    | 说明           |
| -------- | ----------------------- | -------------- |
| `GET`    | `/admin/whitelists`     | 列出所有白名单 |
| `POST`   | `/admin/whitelists`     | 新增白名单条目 |
| `DELETE` | `/admin/whitelists/:id` | 删除白名单条目 |

新增白名单请求体示例：

```json
{
  "method": "GET",
  "path": "/api-v2/public/*",
  "description": "公开接口，无需鉴权"
}
```

> 路由和白名单的任何写操作都会立即触发内存快照更新，**变更实时生效**，无需重启。

### 方式二：CLI 工具（zebra-gw）

编译 CLI：

```bash
go build -o zebra-gw.exe ./cmd/cli/
```

**路由管理：**

```bash
# 列出所有路由
zebra-gw route list

# 新增路由
zebra-gw route add -p /api-v2 -t http://192.168.1.100:9000 -r "" -d "v2 服务"

# 更新路由
zebra-gw route update 3 --target http://192.168.1.101:9000

# 启用 / 禁用路由
zebra-gw route enable 3
zebra-gw route disable 3

# 删除路由
zebra-gw route delete 3
```

**白名单管理：**

```bash
# 列出所有白名单
zebra-gw whitelist list

# 新增白名单（* 方法匹配任意 HTTP 方法）
zebra-gw whitelist add -m GET -p /api-v2/health -d "健康检查"
zebra-gw whitelist add -p "/api-v2/public/*"      # 默认 method=*

# 删除白名单
zebra-gw whitelist delete 5
```

> CLI 工具直接读取 `config/configs.yaml` 连接数据库，修改**不会**立即反映到运行中的网关实例（等待下一次 `RouteReloadInterval` 自动刷新，或调用 `POST /admin/routes/reload` 主动触发）。

### 方式三：热更新机制

- **写操作触发**：通过管理 API 增删改路由 / 白名单后，内存快照**立即**原子更新
- **定时同步**：每隔 `RouteReloadInterval`（默认 30s）从 DB 全量同步一次，保证多实例一致性
- **手动触发**：`POST /admin/routes/reload`

---

## 📤 上游请求头注入

鉴权通过后，网关会向上游服务注入以下请求头：

| 请求头        | 说明                    |
| ------------- | ----------------------- |
| `X-User-Id`   | JWT sub 字段，即用户 ID |
| `X-User-Name` | RBAC 返回的用户名       |

上游服务可直接读取这两个请求头获取调用方身份，无需再验证 JWT。

---

## 🚀 快速启动

### 前置条件

- Go 1.20+
- PostgreSQL 实例（用于路由配置持久化），提前创建数据库 `zebra_gateway`
- ZebraRBAC 已启动（默认 `:8000`）
- `JWTSecret` 与 ZebraRBAC `SECRET_KEY` 保持一致

### 本地开发

```bash
cd d:\Z\ZebraOps\ZebraGateway

# 下载依赖
go mod tidy

# 修改配置（重点设置 DatabaseURL、RbacURL）
vim config/configs.yaml

# 启动网关（首次启动自动建表并导入 YAML 中的初始路由）
go run main.go
```

### 生产部署（环境变量覆盖）

```bash
export ZEBRA_GW_APP_PORT=8080
export ZEBRA_GW_APP_JWTSECRET=<生产密钥>
export ZEBRA_GW_APP_RBACURL=http://zebra-rbac:8000
export ZEBRA_GW_APP_DATABASEURL=postgres://user:pass@pg-host:5432/zebra_gateway?sslmode=disable

go build -o zebra-gateway . && ./zebra-gateway
```

---

## 📋 接口列表

### 网关自身

| 方法  | 路径         | 鉴权 | 说明                            |
| ----- | ------------ | ---- | ------------------------------- |
| `GET` | `/health`    | 无   | 网关自身健康检查                |
| `GET` | `/swagger/*` | 无   | Swagger UI 文档（含 JSON/YAML） |

### 管理 API

| 方法     | 路径                        | 鉴权 | 说明               |
| -------- | --------------------------- | ---- | ------------------ |
| `GET`    | `/admin/routes`             | JWT  | 列出所有服务路由   |
| `POST`   | `/admin/routes`             | JWT  | 新增服务路由       |
| `PUT`    | `/admin/routes/:id`         | JWT  | 更新服务路由       |
| `DELETE` | `/admin/routes/:id`         | JWT  | 删除服务路由       |
| `POST`   | `/admin/routes/:id/enable`  | JWT  | 启用服务路由       |
| `POST`   | `/admin/routes/:id/disable` | JWT  | 禁用服务路由       |
| `POST`   | `/admin/routes/reload`      | JWT  | 手动触发路由热更新 |
| `GET`    | `/admin/whitelists`         | JWT  | 列出所有白名单     |
| `POST`   | `/admin/whitelists`         | JWT  | 新增白名单条目     |
| `DELETE` | `/admin/whitelists/:id`     | JWT  | 删除白名单条目     |

### 代理路由（动态，DB 配置）

| 方法  | 路径      | 鉴权              | 说明             |
| ----- | --------- | ----------------- | ---------------- |
| `ANY` | `/rbac/*` | JWT（白名单除外） | 代理到 ZebraRBAC |

### Swagger UI 访问

启动网关后，浏览器打开：

```
http://localhost:8080/swagger/index.html
```

Swagger UI 完全开放，**无需 JWT 认证**即可查看接口文档。若需在 Swagger UI 中测试受保护接口，点击右上角 **Authorize**，输入 `Bearer <token>` 即可。

> `<token>` 通过 `POST /rbac/login/access-token` 获取。

每次修改接口注释后，在项目根目录运行以下命令重新生成文档：

```bash
swag init --parseDependency --parseInternal
```

---

## 🚨 错误码

| HTTP 状态码 | code | 说明                                           |
| ----------- | ---- | ---------------------------------------------- |
| 200         | 200  | 成功                                           |
| 400         | 400  | 请求参数错误                                   |
| 401         | 401  | 未认证（无 token / token 非法 / 权限获取失败） |
| 403         | 403  | 已认证但无权限                                 |
| 404         | 404  | 路由不存在                                     |
| 500         | 500  | 服务内部错误                                   |
| 502         | 502  | 上游服务不可用                                 |

---

## 🔗 与其他 Zebra 服务的关系

```
ZebraAdmin (React)           ← 前端管理界面
    │  HTTP
    ▼
ZebraGateway (本项目)        ← 统一入口，认证 & 动态路由
    ├──► ZebraRBAC (Python/FastAPI)   ← 权限数据来源
    ├──► ZebraCICD (Go)               ← CI/CD 流水线
    └──► 其他后端服务                  ← 业务服务

ZebraDeployment              ← Docker Compose 一键拉起上述所有服务

PostgreSQL                   ← ZebraGateway 路由配置持久化（zebra_gateway 库）
MySQL                        ← ZebraRBAC 权限数据持久化（zebra_rbac 库）
```

---

## 📄 License

[MIT](./LICENSE)
