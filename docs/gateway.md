# 内置 API 网关（原 Nexa）

Nexa 网关已并入幻图，作为 `gateway/` 模块随主进程启动，由幻图**超级管理员**在后台「API 网关」菜单管理。Nexa 自带的管理员登录与 bootstrap token 已移除，管理接口统一走幻图 `AdminAuth`。

## 架构

| 部分 | 位置 |
| --- | --- |
| 网关后端（路由、账号池、并发、额度、故障转移、额度刷新、密钥、日志、资产、违禁词、设置、封闭模型目录） | `gateway/internal/**` |
| 模块入口（启动、挂载、进程内通道） | `gateway/gateway.go` |
| 导入命令 | `cmd/gateway-import`、`gateway/import.go` |
| 管理页面 | `web/src/app/(admin)/admin/gateway/**` |

- 数据库：PostgreSQL，表放在独立 schema（`GATEWAY_DB_SCHEMA`，默认 `gateway`），启动时自动建 schema 并迁移，不与幻图表冲突。
- Redis：必需。建议单独的 `GATEWAY_REDIS_DB`（设置缓存键为 `setting:<key>`）。
- RustFS/S3：可选。未配置时跳过图片缓存、无法回放已存储的视频；配置了 `GATEWAY_RUSTFS_ENDPOINT` 时生产环境会校验 bucket 与访问密钥（ACCESS_KEY ≥16 位、SECRET_KEY ≥32 位）。
- 支持的上游：ChatGPT、Grok、Adobe Firefly、BytePlus。Dola 已移除（见下文“Dola 移除”）。
- `GATEWAY_ENABLED=true` 时启用，配置只读取 `GATEWAY_*` 环境变量（见 `.env.example`）。
- `GATEWAY_APP_ENV=production` 时必须设置 `GATEWAY_PUBLIC_BASE_URL` 为站点 HTTPS 源地址（不带路径），否则后端启动失败、容器会退出重启。

## 路由

| 用途 | 路径 | 认证 |
| --- | --- | --- |
| 对外 OpenAI 兼容接口（文本、图片、编辑、异步任务、视频） | `/api/gateway/v1/*`（经 Next 代理），容器内 Go 端口 `/v1/*` | `Authorization: Bearer sk-*` |
| 管理接口 | `/api/admin/gateway/*` | 幻图超级管理员登录 |

幻图自身接口在 `/api/v1`，与上述路径不冲突。

## 内置渠道（进程内调用）

在幻图后台新建渠道，Base URL 填 `http://nexa.internal/v1`，API Key 任填。请求不走网络，通过内存监听器直达网关，并使用内置凭证 `cred-huantu-builtin` 记账。外部调用方继续使用后台「API 密钥」页签发的 `sk-*`。

## 从独立 Nexa 导入数据

1. 备份目标库；确认目标幻图已启动过一次（或命令会自动迁移网关 schema）。
2. 在能访问源库的机器上配置好 `.env` 中的 `GATEWAY_POSTGRES_DSN`、`GATEWAY_REDIS_*`。
3. 先演练：

   ```bash
   /app/gateway-import -source 'postgres://user:pass@host:5432/nexa?sslmode=disable' -dry-run
   # 或 go run ./cmd/gateway-import -source ... -dry-run
   ```

4. 确认输出的表与行数后正式执行（去掉 `-dry-run`）。也可用 `NEXA_SOURCE_DSN` 代替 `-source`；`-source-schema` 默认 `public`，`-batch-size` 默认 500。

行为说明：

- 幂等：按主键 upsert（源数据覆盖），无主键的表 `DO NOTHING`，可重复执行。
- 按外键依赖排序复制两边共有的列，结束后重置自增序列，并清除 Redis 设置缓存。
- 跳过 `schema_migrations` 和 `admins`（管理员改由幻图负责）。
- 源库迁移版本不得高于本网关版本，否则拒绝导入。
- **不迁移** RustFS 对象与本地生成文件，如需保留请单独拷贝 `data/generated` / 存储桶。
- **不导入 Dola 数据**：跳过 `pool = 'dola'` 的账号、`provider = 'dola'` 的路由、只有 Dola 路由的逻辑模型，以及引用它们的账号路由绑定、额度桶、额度预留、调度记录、事件日志、`provider.dola.*` / `proxy.dola.*` 设置和 Dola 刷新配置，因此不会留下悬空引用。`-dry-run` 显示的就是过滤后的行数。

## 部署

```bash
# .env 中设置 GATEWAY_ENABLED=true、GATEWAY_POSTGRES_DSN、GATEWAY_PG_PASSWORD、GATEWAY_REDIS_PASSWORD 等
docker compose --profile gateway up -d                       # 含 Postgres + Redis
docker compose --profile gateway --profile gateway-storage up -d   # 另含 RustFS 与建桶任务
```

镜像已包含 `/app/gateway-import`、`/app/gateway-init-bucket` 和 `/app/data/generated` 目录。

## Dola 移除

Dola 渠道（代码、签名脚本、模型目录中的 Dola 模型/路由、管理界面入口、相关环境变量）已整体移除。

- 已有数据库：迁移 `000031_remove_dola` 在启动时自动删除 Dola 账号（含其 Cookie）、路由、只有 Dola 路由的逻辑模型、Dola 设置，以及引用它们的绑定、额度桶、预留、调度记录与事件日志。该迁移不可逆，升级前请先备份。
- 运行时 `dola` 被视为已退役的提供方，即使库中残留相关路由也不会被调度。
- 旧 Dola 生成的视频记录随事件日志一并删除。

## 许可证

网关代码源自 Nexa（MIT），许可证保留在 `gateway/LICENSE`，另见 `THIRD_PARTY_NOTICES.md`。
