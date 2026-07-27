# 企业电商未完成代码架构审计

- **审计人**：高见远（软件架构）
- **审计方式**：严格只读检查业务代码；仅创建本报告
- **审计输入**：`AGENTS.md`、`artifacts/enterprise-ecommerce-architecture.md`、`artifacts/enterprise-ecommerce-prd.md`、当前 `git status`、完整已跟踪差异及全部企业电商相关新增文件
- **未执行事项**：未修改业务文件，未运行构建、测试、格式化、迁移或 Git 提交
- **总判断**：**当前工作区是跨 T01–T05 的未完成纵向切片，不具备合并条件。** 它已经形成“模型—部分仓储—部分服务—Handler/Router—若干页面”的骨架，但计费保留、迁移安全、模板发布/变量渲染、任务进度、ZIP 唯一命名、组织切换缓存、商品/模板/任务完整 UI、视频编辑页与测试均未闭环。

## 1. Git 工作区与审计范围

### 1.1 当前状态摘要

- 分支：`main`，相对 `origin/main` **ahead 7 commits**。
- 已跟踪修改：17 个文件，`241 insertions / 48 deletions`。
- 企业电商相关新增业务文件：后端 8 个、前端 12 个；另有 PRD/架构/规划文档。
- 还存在非本功能或工具状态文件：`.impeccable.md`、`.workbuddy/`、`artifacts/ui-redesign-overview.md` 等，不应与本功能代码混合提交。
- `git diff --check` 未报告空白错误，但仅代表已跟踪差异；未跟踪文件不在该检查范围。

### 1.2 当前实际实现到的能力

有静态证据的能力如下：

1. **数据骨架**
   - 模板增加来源、媒体类型、模板类型、平台、草稿状态和版本 `SpecJSON`。
   - 图片父任务增加 kind、排队/运行计数、估计/保留/实际额度字段；任务项增加模板选择、变体、错误分类字段。
   - 新增 `BatchProductionTemplateSelection`、`VideoProject`、`VideoProjectVersion` 模型并注册 AutoMigrate。
2. **图片预检与多模板展开骨架**
   - 支持商品范围、指定 SKU/全部有效 SKU、多个模板、每模板数量、交付规格。
   - 有 200 商品、5000 项、16MB 快照、企业 10000 待处理项限制。
   - 可检查 SKU 归属、缺参考图、缺卖点，并生成一个 SKU 的提示词预览。
   - 新建任务时可按 `SKU × 模板选择 × quantity` 写入 selection、snapshot、item，并保留 `(organizationId, requestId)` 幂等检查。
3. **Worker 部分接线**
   - 新任务项可读取 selection，并将冻结的 Prompt/交付规格注入现有执行器与归档转换流程。
   - 父任务新增 `pending_review`、`partial_success` 的部分聚合逻辑。
4. **审核与导出部分改造**
   - 单项审核可推进父状态；父任务失败/部分成功可重试。
   - ZIP 目录尝试加入 SPU/SKU/templateType，文件名支持 template/variant 占位符。
5. **视频工程后端基础**
   - 已有企业隔离的工程列表、创建、读取、保存、预检、版本列表和冻结版本路由。
   - 草稿使用乐观版本；草稿/冻结版本通过 `UserFileReference` 保护引用；没有创建虚假 render job。
6. **前端骨架**
   - 新增 commerce 侧栏、商品列表/详情、图片生产向导、模板列表、图片任务列表/详情、视频工程列表。
   - 新页面使用带 `organizationId` 的 Query Key 工厂；运行任务有 5 秒轮询并在静态终态停止。
   - 组织切换开始尝试取消/移除旧组织的 `['commerce', organizationId]` 缓存。

以上只能说明代码骨架存在，**不能说明可编译、可迁移、可运行或满足验收**。

## 2. 闭环审计

### 2.1 图片生产链路

| 层 | 静态证据 | 结论 |
|---|---|---|
| Model | selection、item 变体、父状态和预检 DTO 已定义 | 部分闭环 |
| AutoMigrate | selection、视频工程表已注册；新增字段随现有模型迁移 | 已接线但迁移风险高 |
| Repository | 新增展开事务、selection/job 读取；现有领取/完成/审核被部分扩展 | 部分闭环，事务语义与计数不完整 |
| Service | 预检、创建、job detail、模板 Spec 校验存在 | 部分闭环，估价/额度/变量/发布缺失 |
| Handler | 预检、job detail 已接 | 已接线 |
| Router | `/production/image/preflight`、`/batch-jobs/:id` 已挂企业鉴权组 | 已接线 |
| Frontend API | 预检、创建、详情、items 已定义 | 已接线 |
| Frontend Page | 有四步向导、任务列表和逐项审核/重试 | 仅主干骨架，缺完整 P0 UI |

**结论**：图片链路能看出设计意图，但不是完整闭环。尤其“真实估价 → 创建时额度冻结 → Worker 按项结算 → 终态释放”完全没有实现；模板变量也只校验、不渲染。不能把当前实现描述为“企业图片生产闭环完成”。

### 2.2 视频工程链路

| 层 | 静态证据 | 结论 |
|---|---|---|
| Model / AutoMigrate | 工程与不可变版本模型已定义并注册 | 已接线 |
| Repository / Service | CRUD、乐观锁、素材引用、预检、冻结版本存在 | 后端骨架较完整但校验/并发需修 |
| Handler / Router | 7 个工程接口均注册在企业鉴权组 | 已暴露 |
| Frontend API | CRUD/预检/冻结 API 已定义 | 已定义 |
| Frontend Page | 只有列表；无 `[id]/page.tsx`、无新建/编辑器 | **未闭环** |

**结论**：后端路由已公开，但 UI 中每个工程名称链接到不存在的 `/commerce/video-projects/[id]` 页面，新建按钮明确禁用。因此当前是“路由暴露但产品功能不完整”的典型状态。没有 render 接口是正确的 Release 1 边界。

### 2.3 商品、模板和任务中心

- **商品**：没有 `GET /products/:id`；前端详情通过拉取前 200 个商品后查找，超过范围会误报不存在。没有品牌/类目/状态组合筛选、服务端分页联动、批量状态、商品/SKU/参考图编辑。
- **模板**：数据库模板 Spec 可保存，但没有内置+企业合并列表，没有独立 publish/copy/detail API；草稿“保存”仍会直接增加不可变版本。模板页面只读且不能筛选/编辑/发布。
- **任务中心**：只列批量图片任务，没有单次图片/视频任务聚合；缺创建人/商品/时间筛选、成本真实值、批量审核/重试/取消、2–4 图对比、输入快照详情。

## 3. 主要静态风险

### 3.1 P0 / 合并阻断风险

1. **额度保留是假字段，不是实现**
   - `User.ReservedCredits`、`Organization.ReservedCredits`、Job 的 estimated/reserved/actual 字段仅建模。
   - 全仓静态搜索没有额度冻结、结算或释放逻辑。
   - `EstimatedCredits` 被硬编码为任务项数，每项也是 `1`，未调用模型/渠道真实定价，也没有余额/预算不足阻止创建。
   - **后果**：直接违反 PRD “余额不足不得创建部分任务或重复扣费”，也违反架构 T02/T03 核心事务边界。

2. **任务实时进度字段不可信**
   - Claim 只更新父任务 status，没有减少 `queued_items` / 增加 `running_items`。
   - 单项重试、取消也未完整维护新计数。
   - `failExhaustedBatchProductionItems` 把 queued+running 的 `pending` 写入 `running_items`。
   - **后果**：`GET /batch-jobs/:id` 和前端 5 秒轮询展示的排队/运行进度会陈旧或错误。

3. **ZIP 文件仍可能重名**
   - 平台交付规格的现有 `FilenamePattern` 多数为 `{spu}_{sku}_{role}`，没有 `{template}`、`{variant}`、`{item}`。
   - 新代码只替换占位符，不强制追加 variant/item 后缀；同 SKU、同模板多个变体会产生相同 ZIP entry 名。
   - **后果**：不满足“不覆盖/可唯一识别”；不同解压工具可能覆盖同名文件。该改动绝不能按现状合并。

4. **组织切换只清理新命名空间，旧 `/commerce` 缓存仍存留**
   - 新页面 key 为 `['commerce', organizationId, ...]`；旧巨型页面仍大量使用 `['commerce-products', organizationId, ...]`、`['commerce-workspace', ...]` 等。
   - `organization-switcher.tsx` 只 cancel/remove `['commerce', oldOrg]`，未清旧 key。
   - 随后执行全局 `invalidateQueries()`，存在旧页面活动查询以新 Header 回填旧 key、切回后显示错组织缓存或成员移除后仍保留私有数据显示的风险。
   - **后果**：多租户 UI 一致性与隐私风险，合并阻断。

5. **模板变量只校验，不替换**
   - `validateProductionTemplateVariables` 能拒绝未知变量；但 `batchProductionPrompt` 直接使用原 Prompt 并追加商品文本，没有渲染 `{{product.name}}` 等变量。
   - **后果**：合法变量会原样发送给模型；模板契约与实际执行不一致。

6. **AutoMigrate 注册了未完成 schema，且未证明旧表安全**
   - `BatchProductionItem.TemplateSelectionID` 等字段被设为 `not null`，但旧任务没有 selection；新唯一索引同时覆盖旧记录。
   - 虽然部分字段有默认值，`TemplateSelectionID` 无安全默认声明；三数据库添加非空字段/唯一索引行为未验证。
   - `ReservedCredits` 字段被迁移但业务不使用，容易造成“数据库已支持额度冻结”的假象。
   - **后果**：启动即迁移可能失败或产生不一致 schema。未做数据库验证前不可合并。

7. **前端视频列表存在死链接**
   - `video-projects/page.tsx` 链接 `/commerce/video-projects/${id}`，但目录中不存在 `[id]/page.tsx`。
   - Router 已暴露写接口，前端又声明“后端已落地”，但用户无法创建、保存、预检或冻结。

8. **文档把未验证骨架描述为“真实可用”**
   - `api-response.mdx` 使用“新增真实可用接口”。
   - `pending-test.mdx` 把未收口实现移入已实现待测试；但当前仍有明确静态缺陷，不只是缺测试。
   - **后果**：发布状态和实现证据不一致。

### 3.2 高风险接口/状态不匹配

1. **模板发布模型不一致**：架构要求草稿元数据与 publish 分离；当前 `POST /production-templates` 每次保存（包括 draft）都新增版本，没有 `/publish`。
2. **模板列表不含内置模板**：`ListProductionTemplates` 只查企业表；新图片生产页也只使用该列表，因此内置 `product-main` 等模板在新向导中不可选。
3. **模板筛选不匹配**：Repository 只把 `q.Type` 当 status；没有 source/mediaType/templateType/platform 多维筛选。
4. **模板前端类型不完整**：`ProductionTemplateVersion` 缺 `specJson`；保存 API 类型也未明确 Spec。
5. **商品详情接口缺失**：架构定义 `GET /products/:id`，Router 未注册；前端用第一页最多 200 项模拟详情。
6. **canonical hash 未实现**：重复 scopes 合并后没有再次去重 SKU；模板选择未排序，hash 对语义等价请求不稳定。创建事务没有重做完整模板/定价预检。
7. **selection 去重键错误**：提示称“同一模板版本和交付规格”不可重复，但 key 未包含 version。
8. **错误分类字段未使用**：`ErrorCode`、`Retryable`、`NextAttemptAt` 没有 Worker/Repository 行为，仍按旧租约尝试，未实现分类与退避。
9. **父状态/审核规则局部实现**：状态聚合散落在 Finish、超时失败、审核和重试路径，更新字段不一致，尚未形成单一聚合函数。
10. **旧 UI 不认识新父状态**：旧 `/commerce` 的 `statusLabels/statusColors` 没有 `pending_review`、`partial_success`，会显示空文案/无色状态。
11. **旧导出路径被全局改变**：所有旧任务也会从旧 `{SPU}/file` 改为 `{SPU}/{SKU}/custom/file`；是否允许改变历史交付目录没有迁移/兼容策略。
12. **侧栏破坏旧页面宽度**：新 `commerce/layout.tsx` 在 `xl`（1280px）就固定占 240px，旧巨型 `/commerce` 页面只剩约 1024px；未见响应式回归证据，也没有架构要求的折叠/移动 Drawer。

### 3.3 视频工程静态风险

1. 更新路由没有强制 `expectedVersion > 0`；若对已有 ID 传 0，Repository 会走 create 分支，结果是数据库重复键错误而非业务冲突。
2. 冻结版本不递增草稿 `Version`；同一个 expectedVersion 可以连续冻结多个相同版本快照，缺少幂等/意图说明。
3. shot 只校验文件存在，不校验 `source.kind` 与 MIME 是否匹配；`sourceType`、字幕 style、负 start/trim、BGM trim/fade 等约束不完整。
4. 转场只比较当前镜头时长，没有比较相邻镜头；严格预检没有验证建议总时长 15–60 秒。
5. 工程列表不反序列化 DraftTimeline（当前列表不使用，但 DTO 返回零值）；详情会反序列化。
6. 新建/编辑/时间线/字幕/BGM/安全区页面均缺失；T05 不能视为前端完成。

### 3.4 未定义符号、重复实现与静态可编译性

- 通过源码搜索，新增代码引用的 `GetProductSKU`、`GetUserFile`、`GetBatchProductionJobByRequest`、`resolveProductionPreset`、`batchProductionPrompt`、`replaceUserFileReferences` 均存在；**未发现明确的 Go 未定义符号证据**。
- `CreateBatchProductionJob` 通过把旧函数改名为 `legacyCreateBatchProductionJob` 保留单一公开入口，不构成同包重复定义。
- 但未运行编译/类型检查，不能据此宣称无编译错误。
- 存在明显的**职责重复/分叉**：旧 `CreateBatchProductionJob` 仓储与新 `CreateExpandedBatchProductionJob` 两套创建事务；旧页面 Query Key 与新 Query Key 两套缓存体系；旧单页与新独立页两套产品入口。兼容期可以存在，但必须共享聚合、鉴权、计费和缓存失效规则，当前尚未做到。

## 4. 文件级结论

> “可保留”表示设计方向和局部实现可作为后续修复基础，不表示可以单独合并；所有依赖未闭环的文件仍须随最终切片验证。

### 4.1 已跟踪修改文件

| 文件 | 结论 | 理由 |
|---|---|---|
| `model/commerce.go` | **需修复** | 核心模型方向正确；但非空字段/唯一索引旧数据安全、计数/额度假字段、默认兼容均未收口。 |
| `model/user.go` | **建议撤销** | 只有 `ReservedCredits` schema，没有任何冻结/结算逻辑；应随完整额度事务一次引入，而非提前迁移。 |
| `repository/db.go` | **需修复** | 注册了 selection/video 表是必要接线，但当前模型未达到可迁移状态；必须在数据库验证后保留。 |
| `repository/commerce.go` | **需修复** | selection/spec 读取、父状态方向可留；进度计数错误、状态聚合分散、重试/审核字段不一致、旧路径兼容不足。 |
| `service/commerce.go` | **需修复** | legacy facade 可留；模板保存与发布未分离，变量只校验不渲染。 |
| `service/batch_worker.go` | **需修复** | selection 快照接线方向正确；未接错误分类/退避/额度结算，依赖错误的父进度。 |
| `service/batch_executor.go` | **可保留** | 按 selection 使用交付规格是必要且局部自洽的最小改造；仍需随 Worker 全链验证。 |
| `service/batch_archive.go` | **需修复** | 目录层级方向正确，但文件名不强制 item/variant 唯一，当前会产生同名 ZIP entry。 |
| `handler/commerce.go` | **可保留** | Handler 保持薄层，调用已存在 service；应在服务修复后保留。 |
| `router/router.go` | **需修复** | 路由均在正确企业鉴权组；但视频写接口和不完整功能已公开，需发布开关或完成前不暴露。 |
| `web/src/services/api/commerce.ts` | **需修复** | 新状态/字段已补；版本 Spec 类型缺失，旧页面状态文案与新状态不匹配。 |
| `web/src/components/layout/organization-switcher.tsx` | **需修复** | 顺序方向正确；仅清新 key，无法清兼容页面旧 key，是租户缓存阻断风险。 |
| `web/src/app/(user)/commerce/page.tsx` | **需修复** | 给 delivery specs 加 organizationId 是正确局部修复；但整个旧页仍不在统一 key 根下且不认识新状态。 |
| `docs/content/docs/backend/api-response.mdx` | **需修复** | API 说明方向可留，但“真实可用”表述超出证据。 |
| `docs/content/docs/backend/backend-database.mdx` | **需修复** | 新表说明必要；存在 `。### video_projects` 排版断裂，并把未验证 schema 描述为既成事实。 |
| `docs/content/docs/progress/pending-test.mdx` | **需修复** | 可作为未来验收清单；当前静态缺陷未修，不应全部归类为“已实现待测试”。 |
| `docs/content/docs/progress/todo.mdx` | **可保留** | 明确记录额度、商品/模板/任务 UI、视频编辑器仍缺失，基本忠于现状；最终需与修复结果再同步。 |

### 4.2 新增后端文件

| 文件 | 结论 | 理由 |
|---|---|---|
| `model/video_project.go` | **需修复** | 模型骨架可留；企业字段约束、枚举、时间线约束和版本语义需补。 |
| `repository/commerce_expanded.go` | **需修复** | 多模板原子写入是核心；但定价硬编码、事务内未完整重检、canonical/计数/额度不完整。 |
| `service/commerce_image_job.go` | **需修复** | 新旧入口兼容方式合理；创建前外部预检与事务内真实校验/额度冻结之间仍断裂。 |
| `service/production_preflight.go` | **需修复** | 预检问题定位可留；真实估价、变量渲染、内置模板 DTO、canonical、能力/文件元数据检查均缺。 |
| `service/production_template.go` | **需修复** | 白名单与 Spec 规范化可留；缺变量渲染、完整 Spec 契约和发布边界。 |
| `repository/video_project.go` | **需修复** | 企业隔离、乐观锁、文件引用方向正确；expectedVersion=0 更新、冻结版本并发/幂等需修。 |
| `service/video_project.go` | **需修复** | 后端草稿/预检/冻结主干可留；素材 MIME、时间线完整约束和版本语义不完整。 |
| `handler/video_project.go` | **可保留** | 薄 Handler，无虚假 render 接口；应等 UI/服务闭环或受开关保护后暴露。 |

### 4.3 新增前端文件

| 文件 | 结论 | 理由 |
|---|---|---|
| `web/src/services/api/commerce-query-keys.ts` | **可保留** | 统一 `['commerce', organizationId, ...]` 是正确基线；必须同步迁移/清理旧 key。 |
| `web/src/services/api/commerce-products.ts` | **需修复** | 列表/SKU API 可留；用前 200 项模拟详情不可接受，应接真实详情 API。 |
| `web/src/services/api/commerce-production.ts` | **需修复** | 请求/响应骨架匹配现有 Handler；需随真实估价、selection/filter/批量动作 DTO 收口。 |
| `web/src/services/api/video-projects.ts` | **可保留** | 与后端 CRUD/预检/冻结契约基本对应且无 render 伪接口；当前没有页面消费完整能力。 |
| `web/src/app/(user)/commerce/layout.tsx` | **需修复** | 侧栏方向正确；不折叠、无移动 Drawer，且在 1280px 压缩旧巨型页。 |
| `web/src/app/(user)/commerce/products/page.tsx` | **需修复** | 仅前 100 条客户端分页和关键词；缺服务端分页、多筛选、勾选、批量状态与 CRUD。 |
| `web/src/app/(user)/commerce/products/[id]/page.tsx` | **需修复** | 只读骨架可留；详情来源错误，SKU pageSize 固定 500，缺编辑/参考图/历史任务。 |
| `web/src/app/(user)/commerce/production/images/page.tsx` | **需修复** | 多模板主流程骨架存在；内置模板不可选、SKU 范围不可编辑、异步错误/防双击/预览信息和真实额度缺失。 |
| `web/src/app/(user)/commerce/templates/page.tsx` | **需修复** | 只有只读列表；不满足来源/类型/平台/状态筛选及创建/复制/编辑/发布。 |
| `web/src/app/(user)/commerce/tasks/page.tsx` | **需修复** | 图片列表和条件轮询可留；不是统一任务中心，缺筛选/成本/批量动作。 |
| `web/src/app/(user)/commerce/tasks/[id]/page.tsx` | **需修复** | 单项查看/审核/重试骨架存在；缺取消/父重试/主图/批量动作/2–4 图对比/快照，动作错误处理不足。 |
| `web/src/app/(user)/commerce/video-projects/page.tsx` | **建议撤销** | 当前页面会生成指向不存在详情页的死链接，新建按钮禁用；在详情编辑器出现前不应作为可用导航入口。 |

### 4.4 新增文档与非业务文件

| 文件 | 结论 | 理由 |
|---|---|---|
| `artifacts/enterprise-ecommerce-prd.md` | **可保留** | 本轮需求来源；建议作为独立文档提交。 |
| `artifacts/enterprise-ecommerce-architecture.md` | **可保留** | 本轮架构基线；建议与代码实现状态分离提交。 |
| `docs/ecommerce-content-platform-mvp.md` | **建议撤销** | 旧规划声明“仅规划”，且视频 P0/导出范围与当前 Release 1 架构边界冲突，容易形成双重真相。 |
| `.workbuddy/` | **建议撤销** | 工具运行状态/记忆文件，不是产品源码，绝不能进入业务提交。 |
| `.impeccable.md`、`artifacts/ui-redesign-overview.md` | **建议撤销出本功能提交** | 属于既有 UI 改造上下文，不应与企业电商未完成切片混合合并。 |

## 5. 对照 T01–T05 的证据完成度

> 百分比仅用于表达静态证据覆盖度，不代表质量、测试或可发布性。

| 任务 | 完成度 | 已有证据 | 缺失/阻断证据 |
|---|---:|---|---|
| **T01 基础设施、配置与数据骨架** | **55%** | 模型字段、selection/video 模型、AutoMigrate、路由骨架、数据库/API 文档已改；未新增依赖 | 无 config 开关；额度字段无行为；非空字段/唯一索引迁移未验证；旧任务安全默认不完整；路由提前公开 |
| **T02 模板、商品查询与多模板事务核心** | **40%** | Spec/变量白名单、SKU/模板选择预检、24 项公式型展开、requestId/hash、事务写 selection/snapshot/item | 无真实定价/额度冻结；无 publish；无内置+企业模板列表；无商品详情/批量状态/筛选；hash 非 canonical；事务内未完整重检；无测试 |
| **T03 Worker、状态机、计费、审核与导出** | **30%** | Worker 读取 selection Prompt/规格；部分父状态；单项审核/重试；ZIP 模板目录 | 无保留额度结算/释放；无错误分类/退避；进度计数错误；ZIP 重名；无批量动作/operations 更新/测试 |
| **T04 企业工作台前端闭环** | **30%** | 新布局、商品/模板/图片生产/任务页面骨架、Query Key 工厂、5 秒轮询 | 缺完整 CRUD/筛选/分页/批量/对比/统一任务；组织切换未清旧 key；详情假 API；侧栏不折叠；错误态不足；旧 `/commerce` 兼容风险 |
| **T05 视频工程基础与阻断验证** | **40%** | 后端模型/AutoMigrate/CRUD/引用/预检/冻结/路由/API；无 render 假任务 | 无详情/编辑器/新建页面；列表死链；验证规则与版本并发不完整；无测试/发布阻断验证；无路由开关 |

**整体静态完成度约 39%，且关键 P0 一致性部分低于页面/模型可见度。** 不能以文件数量或路由数量提高完成度判断。

## 6. 最小恢复顺序（极小切片）

以下顺序优先恢复“可迁移、可计费、可执行、可展示”，每一步都限定具体文件；前一步未完成时不要继续扩大页面。

### R01：先封住迁移与暴露面

文件：
- `model/commerce.go`
- `model/user.go`
- `repository/db.go`
- `router/router.go`
- `model/video_project.go`

动作：
1. 暂不迁移无业务行为的 ReservedCredits，或在同一切片实现完整额度事务；禁止只留字段。
2. 为旧任务设计安全默认/可空兼容，确认 `TemplateSelectionID` 与新唯一索引不会阻断三数据库启动。
3. 未完成视频详情 UI 前，用明确配置开关隐藏视频工作台入口/写路由，或完成最小编辑页后再公开。
4. 只做 schema/route 收口，不碰 Worker/UI 美化。

### R02：收口模板契约和预检纯逻辑

文件：
- `service/production_template.go`
- `service/production_preflight.go`
- `service/commerce.go`
- `repository/commerce.go`
- `model/commerce.go`

动作：
1. 分离 draft save 与 publish；补内置/企业统一只读 DTO。
2. 实现白名单变量的确定性渲染，并让 preview 与 Worker 共用同一函数。
3. 规范化并排序 scopes/selections，重复 SKU 再去重；hash 必须基于 canonical input。
4. 增加真实模型定价估算和余额主体判断；若本轮不做额度保留，必须显式降级验收并移除“满足 PRD”表述。

### R03：只完成创建事务与额度原子性

文件：
- `service/commerce_image_job.go`
- `repository/commerce_expanded.go`
- `model/user.go`
- `model/commerce.go`
- `service/generation_task.go`（当前未改，但额度闭环必须落到这里/既有账本）

动作：
1. 在同一 DB 事务内重读商品/SKU/模板版本/额度主体并重做决定性校验。
2. 同 requestId 同 hash 返回旧任务，不重复 selection/item/reference/额度保留；不同 hash 冲突。
3. 写入真实 estimated/reserved，形成可巡检账本；取消与终态释放剩余。
4. 不在此步骤加前端功能。

### R04：统一 Worker 状态聚合、重试与导出唯一性

文件：
- `repository/commerce.go`
- `service/batch_worker.go`
- `service/batch_executor.go`
- `service/batch_archive.go`
- `service/generation_task.go`

动作：
1. 提取单一父状态/计数重算函数，Claim/Finish/Cancel/Retry/Review/租约耗尽全部调用；queued/running/completed/failed 必须真实。
2. 接入 `ErrorCode/Retryable/NextAttemptAt` 和退避，仍保留 lease token fencing。
3. Worker 从保留额度按项结算，失败退款与释放只一次。
4. ZIP 无条件把 templateType、variant、item 后缀纳入最终唯一文件名，不依赖可省略占位符。

### R05：先修租户缓存，再补最小图片前端闭环

文件：
- `web/src/services/api/commerce-query-keys.ts`
- `web/src/components/layout/organization-switcher.tsx`
- `web/src/app/(user)/commerce/page.tsx`
- `web/src/services/api/commerce-products.ts`
- `web/src/services/api/commerce-production.ts`
- `web/src/app/(user)/commerce/production/images/page.tsx`
- `web/src/app/(user)/commerce/tasks/[id]/page.tsx`

动作：
1. 统一所有企业电商 key 到一个根，或在切换时用 predicate 同时取消/移除所有旧 `commerce-*` key；禁止全局 invalidate 把新组织响应写入旧 key。
2. 增加真实 `GET /products/:id` 后再启用详情页。
3. 图片向导只接已经完成的模板/预检/额度契约，提交按钮防重复并保留 requestId。
4. 任务详情先补取消、失败项重试、主图、错误处理和真实进度；批量/对比可作为下一小片。

### R06：最后恢复视频工程入口

文件：
- `service/video_project.go`
- `repository/video_project.go`
- `handler/video_project.go`
- `router/router.go`
- `web/src/services/api/video-projects.ts`
- `web/src/app/(user)/commerce/video-projects/page.tsx`
- 新增架构中已规划但当前缺失的 `web/src/app/(user)/commerce/video-projects/[id]/page.tsx`
- 新增对应 `timeline-editor.tsx`

动作：
1. 先补 expectedVersion、MIME、时间线完整验证与冻结并发语义。
2. 再做可保存、刷新恢复、预检、冻结的最小编辑页。
3. 明确禁用 render；没有 `[id]` 页时不要展示可点击工程链接。

### R07：修正文档状态（仅在代码静态缺陷清零后）

文件：
- `docs/content/docs/backend/api-response.mdx`
- `docs/content/docs/backend/backend-database.mdx`
- `docs/content/docs/progress/todo.mdx`
- `docs/content/docs/progress/pending-test.mdx`

动作：
1. 修复数据库文档标题断裂。
2. 只有达到“已实现、待验证”时才移入 pending-test。
3. 明确额度保留、视频渲染和统一任务中心的真实边界，禁止“真实可用”过度表述。

## 7. 当前绝不能合并的改动

1. **`model/user.go` 与 `model/commerce.go` 的 ReservedCredits 字段**，除非同一合并切片包含完整冻结、结算、退款、释放和巡检。
2. **`repository/db.go` 当前 AutoMigrate 集合**，在三数据库迁移和旧任务兼容未验证前不可合并。
3. **`service/batch_archive.go` 当前命名逻辑**，因同模板多变体仍会重名。
4. **`web/src/components/layout/organization-switcher.tsx` 当前缓存清理逻辑**，因旧 key 未清理，存在租户缓存错配风险。
5. **`web/src/app/(user)/commerce/video-projects/page.tsx` 当前入口**，因存在确定性死链接且没有编辑能力。
6. **`router/router.go` 当前视频写路由公开状态**，除非受开关保护或前端/后端最小工程闭环同时完成。
7. **`repository/commerce_expanded.go` + `service/commerce_image_job.go` 当前任务创建**，因预估硬编码、无额度冻结、canonical/事务内重检不满足架构。
8. **`repository/commerce.go` 当前父进度与状态更新**，因 queued/running 字段可错误，前端详情会展示虚假进度。
9. **`docs/content/docs/backend/api-response.mdx` 的“真实可用”声明及当前 pending-test 状态**，在上述阻断项清零前不可合并。
10. **`.workbuddy/`、旧冲突规划文档和无关 UI 改造文件**，绝不能混入企业电商业务提交。

## 8. 最终结论

当前修改不是应当整体丢弃的废稿：Query Key 工厂、薄 Handler、selection DTO、视频工程文件引用、Worker 读取 selection、模板变量白名单等都有可保留价值。但它把五个架构任务同时铺开，导致最关键的一致性边界没有完成。

建议不要继续“补页面”或“再扩路由”，而是按 R01–R04 先完成：

> **迁移安全 → 模板/预检单一真相 → 创建+额度原子事务 → Worker 状态/计费/ZIP 唯一性**

之后再恢复图片前端和视频编辑器。以当前证据，任何“Release 1 图片闭环已完成”“视频工程已可用”“额度已保留”的表述都不成立。