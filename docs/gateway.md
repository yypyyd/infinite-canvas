# 内置账号池（原 Nexa）

Nexa 已并入幻图，作为幻图的**内置账号池**随主进程启动：账号池里已启用、且有可用路由的模型会自动以「内置账号池」渠道出现在幻图的模型列表中，幻图的生图、编辑、视频与文本请求直接调度到池中账号，无需在「模型管理」中配置渠道。超级管理员在后台「账号池」菜单管理账号、模型路由、日志与素材、违禁词和设置。

账号池**不对外提供任何接口**：没有 `/v1` 或 `/api/gateway/v1` 对外 API、没有 `sk-*` API Key、也没有单独的端口。幻图通过进程内的内存监听器（`http://nexa.internal/v1`，不经 DNS 和网络）调用它。

## 架构

| 部分 | 位置 |
| --- | --- |
| 账号池后端（路由、账号、并发、额度、故障转移、额度刷新、日志、资产、违禁词、设置、封闭模型目录） | `gateway/internal/**` |
| 模块入口（启动、管理接口挂载、进程内通道、模型列表） | `gateway/gateway.go`、`gateway/models.go` |
| 幻图侧虚拟渠道 | `service/builtin_pool.go` |
| 导入命令 | `cmd/gateway-import`、`gateway/import.go` |
| 管理页面 | `web/src/app/(admin)/admin/account-pool/**` |
| 容器入口（嵌入式 PostgreSQL） | `scripts/docker-entrypoint.sh` |

## 运行要求

| 依赖 | 是否必需 | 说明 |
| --- | --- | --- |
| PostgreSQL | 必需 | 见下文“数据库” |
| Redis | 可选 | 仅多个幻图实例共享同一账号池时需要 |
| RustFS / S3 | 可选 | 未配置时跳过图片缓存、无法回放已存储的视频 |

### 数据库

账号池按以下顺序选择数据库，表放在独立 schema（`POOL_DB_SCHEMA`，默认 `gateway`），启动时自动建 schema 并迁移，不与幻图自身的表冲突：

1. `POOL_POSTGRES_DSN`（显式指定）；
2. 幻图自身使用 PostgreSQL 时（`STORAGE_DRIVER=postgres`），直接用 `DATABASE_DSN`；
3. Docker 镜像中、幻图使用 SQLite 时：入口脚本在容器内启动**嵌入式 PostgreSQL**（只监听 Unix socket，数据在 `/app/data/pool-postgres`，随现有 `/app/data` 数据卷持久化），无需任何配置或额外服务。`POOL_EMBEDDED_POSTGRES=false` 可关闭；
4. 以上都没有（例如本机直接运行且使用 SQLite）：账号池不启动，幻图其余功能照常，后台账号池页面提示缺少 PostgreSQL。

`POOL_ENABLED=false` 可整体关闭账号池。

为什么必须是 PostgreSQL：账号池的数据迁移和运行时查询大量依赖 PostgreSQL 专有能力——`jsonb` 字段与运算（70 余处）、类型转换、`ON CONFLICT` upsert、`FOR UPDATE SKIP LOCKED` 调度、advisory lock 迁移锁、PL/pgSQL 数据迁移等，额度预留与故障转移的正确性依赖这些事务语义。移植到 SQLite 等于重写数据层，风险高于收益，因此改为在单容器部署中内置一个 PostgreSQL。

`POOL_DB_SCHEMA` 不能与 PostgreSQL 登录用户同名：Postgres 默认 `search_path` 为 `"$user", public`，同名时幻图自身的表会被建进账号池 schema，启动时会拒绝这种配置。

### Redis（可选）

未设置 `POOL_REDIS_ADDR` 时，并发闸门（每账号/每调用方并发上限，带过期自愈）、轮询游标与设置缓存都在进程内完成，这对单个幻图进程是正确的。只有多个幻图实例共享同一账号池时才需要 Redis，否则各实例的并发计数互不可见。

## 与幻图渠道的关系

- 账号池以虚拟渠道「内置账号池」（ID `builtin-account-pool`）注入运行时渠道列表，不写入系统设置；账号池停止后它自动消失。
- 账号池模型自动加入模型列表（已有的同名模型定义保持管理员的名称、排序与启用状态；管理员可在「模型管理」中停用）。
- **优先级**：同一模型同时由账号池和第三方渠道提供时，优先用账号池；账号池失败时，幻图现有的“排除失败渠道后重选”逻辑会回退到第三方渠道。
- **计费**：账号池模型与其他模型一样需要在「计费管理」中设置价格，否则请求会提示“该模型或当前规格未设置价格”。
- 旧的手动配置（渠道 Base URL 为 `http://nexa.internal/v1`）仍然可用；存在这样的渠道时不再额外注入虚拟渠道，避免重复。

## 管理接口

`/api/admin/account-pool/*`，需幻图超级管理员登录。账号池未运行时返回 503 和原因。

## 从独立 Nexa 导入数据

导入目标就是账号池当前使用的数据库（规则同上）。

```bash
# 单容器（嵌入式 PostgreSQL）：在运行中的容器里执行，会自动找到嵌入式库
docker exec -i infinite-canvas /app/gateway-import -source 'postgres://user:pass@host:5432/nexa?sslmode=disable' -dry-run
docker exec -i infinite-canvas /app/gateway-import -source 'postgres://user:pass@host:5432/nexa?sslmode=disable'

# 本机：使用 .env 中的 STORAGE_DRIVER/DATABASE_DSN 或 POOL_POSTGRES_DSN
go run ./cmd/gateway-import -source '...' -dry-run
```

也可用 `NEXA_SOURCE_DSN` 代替 `-source`；`-source-schema` 默认 `public`，`-batch-size` 默认 500。源库需要能从执行导入的位置访问（例如先把备份 `pg_restore` 到一个临时库）。

行为说明：

- 幂等：按主键 upsert（源数据覆盖），无主键的表 `DO NOTHING`，可重复执行；`-dry-run` 只统计、不写入。
- 按外键依赖排序复制两边共有的列，结束后重置自增序列；配置了 Redis 时清除其设置缓存。
- 跳过 `schema_migrations`、`admins`（管理员改由幻图负责）和 `api_credentials`（外部 API Key 已移除）；不导入 `public.base_url` 设置。
- 源库迁移版本不得高于本账号池版本，否则拒绝导入。
- **不迁移** RustFS 对象与本地生成文件，如需保留请单独拷贝 `data/generated` / 存储桶。
- **不导入 Dola 数据**：跳过 `pool = 'dola'` 的账号、`provider = 'dola'` 的路由、只有 Dola 路由的逻辑模型，以及引用它们的账号路由绑定、额度桶、额度预留、调度记录、事件日志、`provider.dola.*` / `proxy.dola.*` 设置和 Dola 刷新配置，因此不会留下悬空引用。

## 部署

```bash
docker compose up -d                                   # 单容器：SQLite + 嵌入式 PostgreSQL，无需额外服务
docker compose --profile postgres up -d                # 幻图与账号池共用独立 PostgreSQL（需设置 STORAGE_DRIVER/DATABASE_DSN/POSTGRES_PASSWORD）
docker compose --profile redis up -d                   # 可选：多实例共享账号池时的 Redis（POOL_REDIS_ADDR=redis:6379）
docker compose --profile pool-storage up -d            # 可选：RustFS 与建桶任务
```

备份：嵌入式 PostgreSQL 的数据目录在 `data/pool-postgres`。在线备份请用 `docker exec infinite-canvas runuser -u postgres -- pg_dump -h /run/postgresql huantu_pool > pool.sql`；直接拷贝数据目录只在容器停止时可靠。

## 外部接口与 API Key 移除

- 对外 `/v1/*`、`/api/gateway/v1/*`、`sk-*` 认证和 API Key 管理（后端与后台页面）已移除。
- 迁移 `000032_remove_external_api` 清空 `api_credentials` 表中的 Key 哈希，并删除 `public.base_url` 设置。表本身保留（历史迁移引用它）；事件日志中的旧 Key ID 作为历史文本保留。
- 所有进程内请求使用内置身份 `cred-huantu-builtin`（只存在于内存中）。

## Dola 移除

Dola 渠道（代码、签名脚本、模型目录中的 Dola 模型/路由、管理界面入口、相关环境变量）已整体移除。

- 已有数据库：迁移 `000031_remove_dola` 在启动时自动删除 Dola 账号（含其 Cookie）、路由、只有 Dola 路由的逻辑模型、Dola 设置，以及引用它们的绑定、额度桶、预留、调度记录与事件日志。该迁移不可逆，升级前请先备份。
- 运行时 `dola` 被视为已退役的提供方，即使库中残留相关路由也不会被调度。
- 旧 Dola 生成的视频记录随事件日志一并删除。

## 许可证

账号池代码源自 Nexa（MIT），许可证保留在 `gateway/LICENSE`，另见 `THIRD_PARTY_NOTICES.md`。
