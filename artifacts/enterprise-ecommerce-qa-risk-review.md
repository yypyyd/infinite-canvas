# 企业电商增量代码 QA 风险复核

- **复核人**：严过关（QA）
- **复核方式**：严格只读检查；除本报告外未修改任何文件
- **输入**：`AGENTS.md`、企业电商 PRD、架构设计、架构代码审计、当前企业电商相关已跟踪 diff 与新增文件
- **未执行**：未运行构建、测试、类型检查、格式化、迁移或 Git 提交
- **总判定**：**同意当前不可合并。智能路由：Engineer。** 当前不是“测试代码写错”或“只差验证”，而是存在可从源码直接证明的业务、权限、一致性和范围缺口。

## 1. 独立结论与架构报告复核

### 1.1 架构报告 P0 阻断核对

| 架构报告阻断 | QA 核对 | 结论 |
|---|---|---|
| 额度保留只有字段、没有行为 | `model/user.go:31`、`model/commerce.go:26,366-368` 只有字段；全仓业务逻辑无 `ReservedCredits` 使用。`repository/commerce_expanded.go:33,39` 仍把每项和父任务估价硬编码为 1；实际定价与逐项扣费仍发生在 `service/batch_executor.go:89-107` / `service/generation_task.go:44-95` | **确定性 P0，成立** |
| 父任务实时进度不可信 | `repository/commerce.go:1107-1113` Claim 只改 Item 和父状态，不维护 queued/running；取消 `:930-936` 不归零父计数；租约耗尽 `:1151-1158` 把 queued+running 合并数写入 `running_items` | **确定性 P0，成立** |
| ZIP 一定重名 | 新 expanded Job 未设置父 `DeliverySpec`（`service/commerce_image_job.go:29-31`），因此 `service/batch_archive.go:108-115` 对该链路会采用带 `{item}` 的 fallback，静态上不能直接证明新多模板任务必然重名；旧单模板链路通常每 SKU 也只有一个 Item | **不支持“确定性 P0”表述，降为高风险未验证**。但 archive 只取父 Job spec（`:52,60`），完全忽略 selection 自己的 `DeliverySpec`，新任务命名不遵循每模板交付规则，仍是确定性缺陷 |
| 组织切换未清旧缓存 | `organization-switcher.tsx:35-38` 只取消/删除 `['commerce', oldOrg]`；旧页仍使用 `commerce-workspace/products/...` 等多套根键（`commerce/page.tsx:174-189`），之后还全局 invalidate | **确定性 P0，成立** |
| 模板变量只校验不替换 | `service/production_template.go` 有白名单校验；真正执行 `service/batch_executor.go:353-395` 把原 Prompt 作为首段，只追加商品文本，没有替换 `{{product.name}}` 等占位符 | **确定性 P0，成立** |
| AutoMigrate 旧数据/三库风险 | `model/commerce.go:407` 给旧 Item 新增 `TemplateSelectionID not null` 并纳入新唯一索引；`repository/db.go:66-103` 启动即 AutoMigrate；没有兼容回填或迁移验证证据 | **P0 高风险未验证，成立**；是否在每一数据库必然启动失败不能只靠静态代码断言 |
| 视频列表死链接 | `video-projects/page.tsx` 链接动态详情；当前没有 `[id]/page.tsx`；写路由已在 `router/router.go:82-88` 公开 | **确定性范围/导航缺陷，成立**；严重度按发布入口是否可见评为 P1，而不是底层数据安全 P0 |
| 文档称“真实可用” | 实现仍存在上述静态缺陷且没有验证证据 | **确定性发布治理缺陷，成立，P1** |

### 1.2 QA 新发现的重点遗漏

1. **Reviewer 可触发单项重新生成并产生费用**：`service/commerce.go:573-581` 使用 `!canWriteCommerce && !canReviewCommerce`，因此 Reviewer 被允许 retry；但 PRD 角色明确 Reviewer 不能创建任务。该动作会把 Item 重新入队，随后进入模型扣费链路。这是服务端鉴权问题，不是按钮隐藏问题。
2. **取消/租约丢失时 GenerationTask 可能永久 running**：成功路径先归档、再 Finish Item，最后才结算 GenerationTask（`service/batch_worker.go:165-178`）。若用户取消或 lease token 已失效，`FinishBatchProductionItem` 返回失败，代码直接 return，`settleBatchProductionGeneration` 不执行；而扣费已在执行器开始时发生。会留下运行中生成任务和未明确结算的费用。
3. **品牌 Logo 没有被任务文件引用保护**：Job 快照保存 Brand，Worker 又从 Brand 快照读取 Logo（`service/batch_worker.go:304-323`），但创建任务时 `batch_input` 只引用 SKU 图片（`repository/commerce_expanded.go:47-58`），没有加入 `brand.LogoStorageKey`。品牌 Logo 替换后旧文件可能进入 GC，历史任务不可复现。
4. **设主图权限/作用域不符合多 SKU 语义**：`repository/commerce.go:1008-1010` 允许任何“非 rejected”结果设主图，即 pending 也可；清理旧主图时只按 `product_id`，不同 SKU 会互相清除主图。PRD 验收要求审核通过结果可设主图，并按商品/SKU 查看候选。
5. **创建事务没有重做决定性预检**：服务层先在事务外预检（`service/commerce_image_job.go:21-23`），事务只重读 Product/SKU 并直接采用外部解析好的 selection（`repository/commerce_expanded.go:22-39`），没有重查模板状态/版本、素材要求、真实定价、余额或预算。预检到落库之间商品参考图、卖点、模板状态变化时仍可创建不合格 Job。

## 2. 风险清单

> 分类含义：**确定性缺陷**＝由当前控制流/查询条件可直接推出；**高风险未验证**＝需要并发、数据库或真实存储环境才能确认具体表现；**范围未完成**＝PRD/架构切片要求的入口或行为尚不存在。

## 2.1 P0 — 合并阻断

### P0-01 任务创建不冻结额度，余额不足会形成部分任务/运行时失败

- **分类**：确定性缺陷
- **证据**：
  - `model/user.go:31`、`model/commerce.go:26,366-368`：仅声明 Reserved/Estimated/Actual 字段。
  - `repository/commerce_expanded.go:33,39`：Item `EstimatedCredits: 1`，父 `EstimatedCredits=len(items)`。
  - `service/batch_executor.go:89-107`：Worker 真正执行时才按模型定价计算并调用生成任务扣费。
  - `service/generation_task.go:44-95`：逐项 Begin 时才检查个人/企业余额和月预算。
- **具体行为**：API 可在余额不足时先创建完整父任务和全部 Item；Worker 执行到余额耗尽后，后续项失败，无法满足“余额不足不得创建部分任务”“创建幂等只保留/扣费一次”。Job 的 reserved/actual 字段也不会形成可巡检真相。

### P0-02 Reviewer 越权重新生成，可能触发企业/个人扣费

- **分类**：确定性缺陷
- **证据**：`service/commerce.go:573-581`；角色函数 `service/commerce.go:637-638`。
- **具体行为**：Reviewer 满足 `canReviewCommerce`，可调用单项 retry，把失败或驳回 Item 重新排队；这相当于创建新生成轮次，与 PRD “Reviewer 不能创建任务”冲突，并可能消费预算。Job 级 retry 又只允许 canWrite，两个入口授权不一致。

### P0-03 父任务 queued/running 计数在运行和取消阶段错误

- **分类**：确定性缺陷
- **证据**：
  - `repository/commerce.go:1107-1113`：Claim 未从 queued 减 1、未给 running 加 1。
  - `repository/commerce.go:930-936`：Cancel 更新 Item，却不更新父计数。
  - `repository/commerce.go:1151-1158`：租约耗尽后把 queued+running 总数写入 `running_items`，并强制 `queued_items=0`。
- **具体行为**：任务正在执行时详情仍可能显示“全部排队、0 运行”；取消后仍显示排队/运行；租约失败时排队项被错误显示为运行。5 秒轮询只是重复读取错误字段。

### P0-04 取消或 lease fencing 失败可留下已扣费但永久 running 的 GenerationTask

- **分类**：确定性缺陷（触发需要取消/租约并发）
- **证据**：
  - `service/batch_worker.go:165-178`：成功执行后，Finish Item 成功才调用 `settleBatchProductionGeneration`。
  - `repository/commerce.go:1171-1177`：父 Job cancelled/terminal 或 lease token 不匹配时 Finish 返回 lease lost。
  - `service/generation_task.go:44-95`：执行前已创建 GenerationTask 并扣费。
- **具体行为**：上游成功期间用户取消任务，旧 Worker 归档后无法 Finish，于是直接 return；GenerationTask 不 success、不 failed/refund，状态与账本长期不闭合。即使这是“尽力取消”，也不能留下未知终态。

### P0-05 企业切换未清旧 Query 命名空间，存在旧私有数据残留/错键回填风险

- **分类**：确定性缺陷 + 高风险时序
- **证据**：
  - `web/src/components/layout/organization-switcher.tsx:35-38`：只处理新 `['commerce', oldOrg]` 根，随后 `invalidateQueries()`。
  - `web/src/app/(user)/commerce/page.tsx:174-189`：旧页面仍有十余个 `commerce-*` 根键。
  - `web/src/services/api/request.ts:85-98`：请求使用全局 activeOrganizationId；多数 GET API 不接 Query 的 AbortSignal。
- **具体行为**：旧企业缓存不会被 remove；活动旧查询也没有统一被取消。切换/成员移除后，浏览器内仍可能保留旧企业商品、成员、任务、审计数据；全局失效与全局 Header 组合还可能把新组织响应写进旧 key。

### P0-06 模板合法变量不会渲染，预览与执行都会把占位符原样发送

- **分类**：确定性缺陷
- **证据**：`service/batch_executor.go:353-395`；变量只在 `service/production_template.go` 被校验。
- **具体行为**：包含 `{{product.name}}`、`{{sku.attributes.color}}` 的已验证模板仍把字面占位符发给模型；模板预览也复用同一未渲染函数，无法满足最终 Prompt 预览和历史可复现性。

### P0-07 创建事务没有重做模板/素材要求/定价/额度决定性校验

- **分类**：高风险未验证（TOCTOU），其中“事务内无校验”是确定事实
- **证据**：`service/commerce_image_job.go:21-31` 与 `repository/commerce_expanded.go:17-39`。
- **具体行为**：事务接收预检阶段形成的 selection，未重查模板是否仍 active、版本是否仍允许、RequireReference/RequireSellingPoints 是否仍满足，也未查真实定价和余额。预检后到事务开始前若 SKU 图片/商品卖点/模板状态变化，仍可能创建不能执行的 Job。

### P0-08 新 schema 启动即迁移，但旧 Item 兼容和三数据库行为无证据

- **分类**：高风险未验证
- **证据**：`model/commerce.go:401-415`、`repository/db.go:66-103`。
- **具体行为**：旧 Item 没有 selection，新字段却为 not null 且进入业务唯一索引；ReservedCredits 同时被迁移但业务不使用。SQLite/MySQL/PostgreSQL 对已有表添加非空列/复合唯一索引的实际结果未验证，可能启动失败或形成与代码假设不一致的 schema。

### P0-09 品牌 Logo 快照缺少文件引用保护，历史输入可能被 GC

- **分类**：确定性缺陷
- **证据**：
  - `repository/commerce_expanded.go:47-58`：Brand 有 JSON 快照，但 `batch_input` keys 只收集 SKU 图片。
  - `service/batch_worker.go:304-323`：执行时从 Brand 快照取 `LogoStorageKey` 并解析文件。
  - `repository/user_workspace.go:152-187`：文件是否进入未引用状态完全取决于 UserFileReference。
- **具体行为**：任务创建后更换品牌 Logo 会删除旧 brand 引用；由于 Job 没有 batch_input 引用，旧 Logo 可被标记未引用并被 GC，重试时输入不可用，违反快照不可变和引用文件不被 GC 的验收要求。

## 2.2 P1 — 高严重问题

### P1-01 设主图允许未审核结果，且不同 SKU 互相覆盖

- **分类**：确定性缺陷
- **证据**：`repository/commerce.go:1008-1010`。
- **具体行为**：条件只排除 rejected，因此 pending 结果也能设主图；清旧主图只限定 job+product，没有限定 SKU，给 SKU-A 设主图会清除同商品 SKU-B 的主图。

### P1-02 ZIP 对新多模板任务忽略 selection 的交付命名规则

- **分类**：确定性缺陷；重名本身为高风险未验证
- **证据**：`service/batch_archive.go:52,60,98-115`；`BatchProductionArchiveItem` 不包含 selection DeliverySpec。
- **具体行为**：写包始终传父 `job.DeliverySpec`；新 expanded job 父 spec 为空，最终统一 fallback 命名，模板各自 `FilenamePattern` 不生效。当前静态证据不足以认定新链路必然重名，但必须验证同 SKU×同模板多变体、历史 legacy、自定义 pattern 截断碰撞和不同 MIME 场景。

### P1-03 图片预检只检查 storageKey 数量，不定位文件失效/MIME 问题

- **分类**：确定性缺陷
- **证据**：`service/production_preflight.go:76` 只看 `len(sku.ImageStorageKeys)`；直到 `repository/commerce_expanded.go:58` 才统一 replace 引用。
- **具体行为**：SKU 有非空但已失效的 key 时，预检可能显示可提交；创建事务才以通用错误失败，不能定位到具体 SKU+模板。也没有按模板/模型能力校验参考图数量、大小和 MIME。

### P1-04 canonical 幂等不完整

- **分类**：确定性缺陷
- **证据**：
  - `service/production_preflight.go:41-50`：重复 product scope 合并后追加的 SKU 没有再次去重。
  - `service/production_preflight.go:63-78`：模板选择不排序；selection 去重 key 缺 templateVersion。
  - `service/commerce_image_job.go:24-28`：hash 直接基于上述数组顺序。
- **具体行为**：语义相同但 scopes/selections 顺序不同的请求配合同一 requestId 会冲突；错误提示称“同一模板版本”不可重复，但 v1/v2+同 delivery 被当作重复。

### P1-05 视频时间线输入校验明显不完整

- **分类**：确定性缺陷
- **证据**：`service/video_project.go:97-116`。
- **缺口**：
  - Shot `StartMs`、`TrimStartMs` 可为负；`SourceType` 未枚举校验。
  - 只检查 kind 为 image/video，不检查 shot kind 与实际文件 MIME 匹配；仅 BGM 检查 audio MIME。
  - transition 只比较当前镜头时长，不比较相邻镜头。
  - Subtitle ID 唯一性与 style 白名单未验证。
  - BGM `TrimStartMs/FadeInMs/FadeOutMs` 未验证非负及不超过时长。
  - strict 模式未检查建议的总时长 15–60 秒。
- **具体行为**：服务端可以冻结浏览器无法可靠渲染、未来 Renderer 也无法安全解释的版本快照。

### P1-06 视频冻结缺少重放幂等；更新 expectedVersion=0 错误语义

- **分类**：确定性缺陷
- **证据**：
  - `repository/video_project.go:66-81`：冻结只增加 LatestVersion，不增加草稿 Version；同一 expectedVersion 可重复冻结多个相同快照。
  - `service/video_project.go:53-56` + `repository/video_project.go:40-49`：对已有 ID 传 expectedVersion=0 会进入 create，返回数据库重复键而非业务冲突。
- **具体行为**：网络重放可产生重复工程版本；错误客户端输入不能得到稳定的乐观锁冲突语义。

### P1-07 视频工程表缺强制租户 schema 约束

- **分类**：高风险未验证
- **证据**：`model/video_project.go:67-94` 的 `OrganizationID` 只有 index，没有 `not null`；架构要求所有新表有 organization_id 且优先作为隔离键。
- **具体行为**：服务层当前会填组织 ID，但数据库层不能阻止空租户脏数据，后台脚本/未来仓储调用可破坏隔离假设。

### P1-08 文档与可用性状态超前

- **分类**：确定性发布治理缺陷
- **证据**：`docs/content/docs/backend/api-response.mdx` 的“真实可用”表述、`pending-test.mdx` 将仍有静态缺陷的功能归为“已实现待测试”。
- **具体行为**：验收方可能把明确未实现误认为只差回归验证，掩盖计费与租户阻断。

## 2.3 P2 — 次要但应修复

1. **错误字段形状不利于前端定位**（确定性缺陷）：视频文件缺失时把 storageKey 放进 `ProductionPreflightIssue.Field`（`service/video_project.go:115`），而不是明确的 shot/subtitle/BGM ID；批量预检问题也缺 selectionIndex/variant 上下文。
2. **视频工程列表 DTO 不解析 DraftTimeline**（确定性缺陷）：`repository/video_project.go:12-22` 列表直接返回，`DraftTimeline` 为零值；若列表未来展示时长/比例会与详情不一致。
3. **同步 ZIP 仍是长请求资源风险**（高风险未验证）：`service/batch_archive.go:49-88` 在请求期间落临时文件并串行下载，2GB/10 分钟只是上限；需要验证取消、源端慢响应、磁盘耗尽与临时文件清理。
4. **新 commerce layout 在 1280px 固定占宽且不可折叠**（范围未完成）：不满足架构规定的折叠/Drawer 和 PRD 最低视口验收。

## 3. 范围未完成（不能用“骨架已落地”替代验收）

### 3.1 Release 1 图片/商品/模板/任务

- 商品缺真实 `GET /products/:id`；`commerce-products.ts:5-9` 只拉前 200 项查找，超过范围会误报不存在。
- 商品列表/详情缺品牌、类目组合筛选、服务端完整分页、批量状态、完整 CRUD、参考图和历史任务闭环。
- 图片向导商品和模板均固定只取前 200 项（`production/images/page.tsx:19-23`），SKU 自动全选，用户不能按 PRD 编辑指定 SKU；没有真实额度门禁、防双击 loading/disabled 状态。
- 模板列表只查企业表（`repository/commerce.go:745-761`），未合并内置模板；无独立 publish/copy/detail，保存草稿仍每次创建不可变版本（`:784-807`）。
- 任务中心没有聚合单次图片/视频；缺批量审核/重试/导出、2–4 图对比、输入快照详情、完整筛选和真实费用。
- 错误分类字段 `ErrorCode/Retryable/NextAttemptAt` 已建模但 Worker 未使用；没有明确 transient/permanent 退避状态机。

### 3.2 视频工程基础

- 前端只有列表，无新建、`[id]` 编辑页、镜头排序、字幕、BGM、安全区预览、保存/预检/冻结入口。
- 当前列表链接到不存在详情页，新建按钮禁用；后端写路由却已公开且没有配置开关。
- 没有视频渲染/MP4 是架构明确的 Release 2 边界，本身不是缺陷；但 UI/文档必须明确“仅工程草稿/冻结”，不能暗示已可成片。

### 3.3 验证与文档

- 当前企业电商新增/修改范围没有相应新增测试证据。
- 未完成 MySQL/PostgreSQL 双 Worker、三数据库迁移、七牛过期 URL/GC、真实上游、备份恢复和 Docker 路径的发布阻断验证。
- 数据库/API/todo/pending-test 文档尚未与真实完成边界一致。

## 4. 正向核对（避免只报风险）

以下方向静态上正确，但不能抵消上述阻断：

- 新 commerce/video 路由均挂在 `UserAuth + OrganizationAuth` group（`router/router.go:41,71-88`）。
- 新增商品、模板、Job、Item、视频工程读取大多在 repository 查询中带 `organization_id`；未发现确定性的后端跨租户直接读取路径。
- Worker Renew/Finish 使用 Item + Organization + Job + running + lease token 条件（`repository/commerce.go:1175,1204-1208`），fencing 基础仍在。
- expanded Job 的 `(organization_id, request_id)` 幂等检查和组织行锁方向正确（`repository/commerce_expanded.go:17-21`）；问题是 canonical、额度和事务内重检尚未闭合。
- 结果持久化为企业 `UserFile`/storageKey，成功 Finish 才建立 `batch_result` 引用；没有把上游临时 URL 作为最终交付字段。
- 视频工程草稿/版本文件引用在同一事务替换（`repository/video_project.go:34-53,66-81`），且没有伪造 render job。

## 5. 后续验证矩阵（本轮不编写、不运行）

| 领域 | 优先验证场景 | 关键断言 | 建议层级/环境 |
|---|---|---|---|
| 租户隔离 | 两企业伪造 Header + product/template/job/item/storageKey/videoProject ID；成员移除后继续轮询/下载 | 全部服务端拒绝；缓存和正在执行请求不再展示旧企业数据 | Router 集成 + 浏览器 E2E |
| 角色鉴权 | Owner/Admin/Member/Reviewer 对商品编辑、模板发布、建任务、单项/整单 retry、审核、主图、导出逐矩阵验证 | Reviewer 不得创建/重试生成；Member 不得发布/审核；不能只靠前端隐藏 | Service/Router 表驱动 |
| 幂等创建 | 同 requestId 同内容串行/并发；字段顺序、scope/selection 顺序变化；同 ID 不同内容 | 仅 1 Job、精确 Item 数、1 次额度保留/审计；语义等价请求返回同 Job；内容变化冲突 | Repository 并发，MySQL/PostgreSQL |
| 额度/预算 | 个人/企业共享模式；余额不足；月预算边界；取消；部分成功；Worker 崩溃；失败退款 | 创建要么全接收要么全拒绝；reserved/actual/余额/账本守恒；释放/退款至多一次 | Repository+Service，并发数据库 |
| 展开与快照 | 2×2×3×2=24；指定 SKU；重复 scope；模板 v1/v2；预检后立即变更素材/模板 | 精确 24；无重复 Item；事务内拒绝过期输入；重试仍用旧快照 | Service/Repository |
| 任务租约 | 双 Worker Claim；续租；过期接管；旧 Worker 晚回写；取消时上游成功 | 同 Item 单有效结果；旧 token 影响 0 行；GenerationTask 必达终态；无重复收费/退款 | MySQL/PostgreSQL 双 Worker |
| 状态机 | Claim/Finish/fail/cancel/retry/review 每条路径 | 父 queued/running/success/failed 总数始终等于 total；终态不再轮询 | Repository 状态机属性测试 |
| 错误重试 | 429、超时、连接中断、参数错误、内容违规、归档失败 | transient 有限退避；permanent 直接失败；NextAttemptAt 生效；成功项不重跑 | Worker 集成 + fake upstream |
| 文件引用/GC | SKU 图片、Brand Logo、模板参考、结果、视频草稿/版本被替换/删除 | 所有历史快照需要的文件仍有引用；旧 Worker 无引用孤儿最终 GC | Repository + 对象存储集成 |
| ZIP | 同 SKU×同模板 10 变体；多模板同 pattern；超长/非法/同名编码；缺文件；>2GB；取消下载 | Entry 唯一、无 `../`、遵循 selection 命名、只含 approved 有效企业文件；失败无空包且临时文件清理 | Service 集成 + ZIP 内容检查 |
| 前端组织切换 | 旧 commerce 页与新独立页分别在请求中切组织，随后切回；移除成员 | 取消所有旧企业请求；移除所有旧 key；响应不串 key；无旧私有数据闪现 | React Query 单测 + Playwright |
| 前端提交 | 双击、响应丢失重试、预检后修改输入、路由跳转 | 同一次内容复用 requestId且按钮禁用；内容变化产生新 ID；错误可见 | 组件/E2E |
| 视频校验 | 负 start/trim/fade、kind/MIME 不匹配、重复 ID、错误 style/sourceType、相邻转场超限、总时长边界 | 草稿可按策略保存但严格冻结必须逐项拒绝并定位；跨企业素材拒绝 | Service/Router 表驱动 |
| 视频并发版本 | 两客户端同 expectedVersion 保存；重复冻结请求；冻结后继续编辑 | 仅一个保存成功；重复请求幂等或明确冲突；旧版本不可变 | Repository 并发 |
| 迁移 | 含 legacy Job/Item 数据的 SQLite/MySQL/PostgreSQL 启动升级 | 不丢历史数据；新非空列/索引可用；旧任务仍可读/执行/导出 | 三数据库迁移夹具 |
| 响应与文档 | 所有新增接口 success/error；文档示例与路由核对 | 始终 `{code,data,msg}`；不泄露内部错误/签名 URL；状态说明与实现一致 | Router 契约检查 |

## 6. 智能路由与合并意见

- **Routing Decision：Engineer**
- **理由**：失败来源是业务实现缺失和源码确定性缺陷，不是 QA 断言错误；当前也没有可进入“只补测试”的稳定契约。
- **合并意见**：**不可合并**。至少先完成：
  1. 收回/保护未闭环 schema 与视频入口；
  2. 创建事务内真实预检、canonical 幂等和额度保留；
  3. 统一父状态计数、GenerationTask 结算和 lease/cancel 语义；
  4. 修复 Reviewer retry 鉴权、主图审核/作用域、品牌 Logo 引用；
  5. 统一清理所有旧/新 commerce Query key；
  6. 实现模板变量渲染并统一 preview/worker；
  7. 修复 selection 级 ZIP 命名，再进入验证矩阵。

**最终结论**：架构报告“当前不可合并”的主结论正确；QA 对“ZIP 当前必然重名”这一条不认定为静态确定性 P0，但发现了 Reviewer 越权扣费、取消/租约导致 GenerationTask 悬挂、品牌 Logo 引用缺失、主图跨 SKU 覆盖等额外阻断/高风险问题，整体不可合并结论反而更强。
