# NovaNova 式 Agent 创作能力实施方案

## 结论

当前项目已经具备多渠道模型配置、图片/视频生成、持久化无限画布、生成记录、资产能力以及批处理 Worker，约有 55%–65% 的基础设施。真正缺少的不是再做一个聊天框，而是服务端统一的 **Agent Run → Tool Call → Job → Event → Observation → 下一轮** 执行闭环。

建议吸收其能力设计，不直接照搬 Java/AgentScope 实现；继续沿用本项目 Go + Gin + GORM、Next.js + React Flow + Zustand 架构。

## 目标体验

1. 用户在画布助手中输入自然语言，也可引用图片、视频和画布节点。
2. 服务端根据场景选择主 Agent、图片 Agent、视频 Agent或画布 Agent。
3. Agent 产生结构化工具调用，服务端校验权限、模型能力、参数和费用。
4. 长任务写入数据库并由通用 Worker 异步执行；SSE 推送计划、工具调用、进度、成功、失败和取消事件。
5. 前端执行画布工具并回传真实结果；服务端将结果作为 observation 继续 Agent 循环。
6. 生成结果关联会话、任务、画布节点和来源节点，可复用、再次编辑或加入个人资产库。

## 架构决策

- **项目结构**：保持现有 layer-first，不为引入 Agent 重构全仓；新增代码仍遵循 handler → service → repository。
- **API 客户端**：沿用 `web/src/services/api/` 的 typed fetch，不新增 React Query。
- **鉴权**：沿用当前同源后端鉴权；Agent、任务、资产均按当前用户隔离。
- **实时方式**：选择 SSE。主要是服务端向前端推送 Agent 与生成进度，不需要 WebSocket 的双向长连接复杂度。
- **错误处理**：继续使用 `{ code, data, msg }`；SSE 事件内部使用稳定的 `type/code/message/retryable`。
- **队列**：首版复用数据库任务抢占和租约模式，抽成通用 Job Worker；不要为了模仿目标项目立即引入 Redis Stream。规模增长后再换 Redis，不改变上层协议。

## 目标模块

### 1. Agent 会话与运行

建议新增：

- `agent_sessions`：项目、画布、profile、标题、状态、上下文摘要。
- `agent_messages`：role、content、attachments、tool_call_id、sequence。
- `agent_runs`：一次用户请求对应的运行，记录 profile、model、status、usage、error、cancel_at。
- `agent_steps`：message / plan / tool_call / tool_result / final，保存输入输出快照与耗时。

Agent profile 首期仅四类：`main`、`image`、`video`、`canvas`。提示词使用服务端文件管理，不硬编码进 Go；每个 profile 明确允许的工具白名单、最大循环次数和最大工具次数。

### 2. 工具注册与安全边界

统一工具协议：

```json
{
  "name": "image.generate",
  "arguments": {},
  "call_id": "call_xxx"
}
```

首期工具：

- `image.generate`、`image.edit`、`image.history.search`
- `video.generate`、`video.history.search`
- `canvas.get_state`
- `canvas.create_nodes`、`canvas.update_nodes`、`canvas.move_nodes`
- `canvas.delete_nodes`、`canvas.connect_nodes`
- `asset.search`、`asset.add`

工具分两类：

- **服务端工具**：生成、历史、资产，由后端直接执行。
- **前端画布工具**：节点增删改连，由 SSE 下发；前端执行后调用结果回传接口。涉及删除、批量覆盖等破坏性动作必须要求用户确认。

所有工具参数先做 JSON Schema/Go struct 校验，再进入 service；模型永远不能直接访问 repository、URL、API Key 或任意 HTTP。

### 3. 通用异步任务

将现有批处理 Worker 的数据库抢占、租约、重试经验提取为普通生成可用的通用执行器：

- `jobs` 或扩展现有 generation task：kind、status、payload snapshot、attempt、lease_owner、lease_until、progress、result、error、cancel_requested。
- 创建任务与扣费/冻结额度必须在事务内完成。
- Worker 幂等领取，成功写生成记录，失败统一结算或退回额度。
- Agent tool call 只保存 `job_id`，由任务事件唤醒对应 run 继续执行。

首版不要让 HTTP handler 等待图片/视频生成完成。

### 4. SSE 事件协议

建议端点：

- `POST /api/v1/agent/sessions`
- `POST /api/v1/agent/sessions/:id/messages`
- `GET /api/v1/agent/runs/:id/events`
- `POST /api/v1/agent/runs/:id/tool-results`
- `POST /api/v1/agent/runs/:id/cancel`

事件类型：

- `run.started`
- `message.delta`
- `plan.created`
- `tool.requested`
- `tool.completed`
- `job.progress`
- `job.completed`
- `run.waiting_tool`
- `run.completed`
- `run.failed`
- `run.cancelled`

每个事件带递增 `sequence` 并持久化最近事件；前端断线可用 `Last-Event-ID` 恢复，避免刷新后丢进度。

### 5. 画布闭环

前端画布暴露一个很小的工具执行适配层，将 Zustand/React Flow 操作映射为受控命令。工具结果至少返回：成功与否、受影响节点 ID、最终节点摘要、错误信息。

关键原则：服务端 Agent 不猜测操作是否完成。只有收到前端真实 tool result 后，才写入 `agent_steps` 并继续下一轮。

生成节点应记录 provenance：`run_id`、`step_id`、`job_id`、`source_node_ids`、prompt、model、channel、generation_record_id。这样才能实现“基于上一张继续改”和完整追溯。

### 6. 提示词优化

图片和视频分别设置策略与接口，优化前保留原文，成功后由用户确认回填；失败不覆盖输入。优化模型使用服务端渠道配置，并记录原提示词、优化结果和采用状态。

## 与当前项目的主要差距

| 能力 | 当前状态 | 需要补齐 |
|---|---|---|
| 模型渠道与能力 | 已有渠道、映射、计费、路由 | Agent 专用模型、工具调用兼容与能力校验 |
| 图片/视频生成 | 已较完整 | 统一 Job 协议、取消/恢复、视频编辑 |
| 生成记录 | 已有任务与历史 | 来源谱系、中间步骤、Agent run 关联 |
| 异步执行 | 批处理专用 Worker | 通用 Job Worker 与普通生成异步化 |
| 实时反馈 | 前端可解析流，但后端会完整缓冲 | 真正 SSE、事件持久化和断线恢复 |
| 画布 | 节点/连线/撤销/自动保存成熟 | 服务端工具协议、前端执行回传、确认机制 |
| 对话 | 画布 JSON 内存储会话 | 独立 session/message/run/step 数据模型 |
| 资产 | 公共与个人资产已有基础 | 统一来源、加入资产工具和引用关系 |
| 提示词 | 提示词库与反推已有 | 图片/视频独立优化与安全回填 |

## 分阶段交付

### P0：可用闭环

- Agent session/message/run/step 表与 API。
- OpenAI-compatible tool call 解析，四个 profile 和工具白名单。
- 通用 Job Worker：图片生成、图片编辑、视频生成。
- SSE 事件流、取消、失败反馈。
- 画布 `get_state/create/update/move/connect` 工具及结果回传。
- 结果自动插入画布并关联生成记录。

验收：用户说“参考这张图生成 3 个电影感版本并排放到右侧”，系统可规划、生成、实时显示状态并自动落到画布。

### P1：连续创作

- 历史/资产检索工具。
- 来源节点与版本链追溯。
- 图片/视频提示词独立优化。
- SSE 断线恢复、任务重试、运行恢复。
- 删除和批量修改的用户确认。

验收：用户可说“用刚才第二张继续改成夜景”，Agent 能准确定位来源并生成新版本。

### P2：高级编排

- 多步骤 DAG、并行任务和依赖调度。
- 失败步骤局部重跑。
- 会话摘要与长上下文压缩。
- 多渠道自动故障转移和成本/质量路由。
- 计划可视化、单步批准与可观测性面板。

## 首批建议修改位置

后端：

- `model/`：新增 agent session/run/step/event 数据结构，扩展 generation task 关联字段。
- `repository/`：新增 Agent 与通用 Job 数据访问。
- `service/`：新增 orchestrator、tool registry、run loop、event service、generic job worker。
- `handler/` 与 `router/`：Agent 会话、消息、SSE、tool result、cancel。
- `config/`：Agent profile、提示词文件路径、循环与并发限制。
- `docs/content/docs/backend/backend-database.mdx`：同步新增表结构。

前端：

- `web/src/services/api/`：Agent 与 SSE 客户端。
- `web/src/app/(user)/canvas/`：画布工具执行适配层、助手面板、运行状态展示。
- `web/src/stores/` 或画布内部 store：只保存当前页面生命周期的运行态，不使用 localStorage/IndexedDB。

## 风险控制

- 不允许模型任意拼接 URL、SQL、文件路径或渠道参数。
- 工具白名单按 profile 固定；参数严格校验；循环和费用都设硬上限。
- 删除、覆盖、大批量画布操作需显式确认。
- 任务、事件和 tool result 必须幂等，避免重连或重试导致重复扣费、重复节点。
- 前端 Agent 请求只访问同源 `/api/v1/*`，上游地址和密钥继续只放服务端。
- 首版避免照搬 Redis Stream 和 AgentScope；先用项目已有、团队能维护的基础设施完成闭环。

## 建议下一步

直接从 P0 开始，但分成两个开发批次：

1. **后端骨架**：数据表、会话/运行接口、工具协议、SSE、通用 Worker。
2. **画布接入**：助手面板、工具执行回传、节点 provenance、生成结果自动落板。

先打通一个垂直切片：`自然语言 → image.generate → 异步任务 → SSE → 图片节点`，验证闭环后再扩视频和画布编辑工具。