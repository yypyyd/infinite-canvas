# 企业电商改造审计总览

## TL;DR

企业级电商图片/视频生成改造已完成增量 PRD、架构设计、代码架构审计和 QA 风险复核；工程实现因连续三次执行轮次超限只留下约 39% 的未完成纵向骨架，当前不可合并、不可视为可用版本。

## 已完成

- 增量 PRD：明确商品/SKU、多模板图片生产、视频工程、任务中心、存储、导出和企业后台边界。
- 增量架构：确定 Release 1 图片闭环与视频工程基础，给出 T01–T05 实施切片。
- 代码架构审计：逐文件标记可保留、需修复、建议撤销，并给出 R01–R07 恢复顺序。
- QA 风险复核：独立核对 P0/P1/P2 风险，智能路由判定为 Engineer。

## 当前关键结论

- 合并状态：不可合并。
- 静态完成度：约 39%，不代表可编译、可迁移或可运行。
- QA 路由：Engineer；问题来自业务实现缺失和源码确定性缺陷，不是测试代码问题。
- 本次审计未修改或回滚业务代码，未运行构建、测试、类型检查或数据库迁移。

## 主要 P0 阻断

1. 额度保留只有字段，无冻结、结算、退款和释放；估价硬编码。
2. Reviewer 可越权重试生成并触发扣费。
3. 父任务 queued/running 计数错误，任务详情可能显示虚假进度。
4. 取消或租约失效后，GenerationTask 可能永久停留在 running，账本不闭合。
5. 企业切换没有清除旧 `commerce-*` Query Key，存在私有数据残留和错键回填风险。
6. 模板变量仅校验不渲染，占位符会原样进入预览和上游。
7. 创建事务未重做模板、素材、定价和额度决定性校验，存在 TOCTOU。
8. 品牌 Logo 快照没有文件引用保护，历史任务依赖文件可能被 GC。
9. 新非空字段和唯一索引对 legacy 数据及三数据库 AutoMigrate 尚未验证。

## QA 对架构审计的校正

架构审计将 ZIP 重名列为确定性 P0；QA 复核认为当前 expanded Job 会落入带 `{item}` 的 fallback，静态上不能证明必然重名。但归档逻辑确定忽略 selection 自己的 DeliverySpec/filename pattern，仍是 P1 确定性缺陷，并必须覆盖多模板、多变体、截断碰撞等验证矩阵。

## 建议恢复顺序

1. R01：封住迁移和未完成路由暴露面。
2. R02：统一模板契约、变量渲染和 canonical 预检。
3. R03：完成任务创建与额度原子事务。
4. R04：统一 Worker 状态聚合、结算、重试和 ZIP 唯一命名。
5. R05：修复租户缓存后，再补图片前端最小闭环。
6. R06：补齐视频工程编辑页后恢复视频入口；仍不实现渲染。
7. R07：最后修正文档状态，并进入验证矩阵。

## 最新恢复进展：R03-B 已静态通过

- `repository/commerce.go` 已建立父任务单一聚合函数，从 Item 事实统一重算 total/queued/running/completed/failed 和父状态。
- Claim、Finish、Cancel、单项/整单 Retry、Review、租约耗尽七条路径已统一接入，不再各自维护易漂移的父计数。
- QA 首轮识别出 Claim/Finish 锁序反转、failed+pending 状态误判、整单重试契约扩张、cancelled Item 聚合缺口和空 Job 静默失败。
- 工程修复后，Claim 锁序统一为 `Job → Organization → Item CAS`；整单 Retry 恢复为只处理执行失败 Item；状态矩阵和空 Job 防御已修正。
- 第二轮静态 QA：`PASS`，路由 `NoOne`，上述 P0/P1 阻断均关闭。
- 已知模型限制：父任务没有 `cancelled_items` 字段，因此取消任务四类公开计数之和可能小于 `total_items`。
- 本结论仍不表示整个 Release 1 可合并；后续 R03/R04–R07 和最终验证尚未完成。本轮遵循项目规范未执行构建或测试。

## 相关文档

- `artifacts/enterprise-ecommerce-prd.md`
- `artifacts/enterprise-ecommerce-architecture.md`
- `artifacts/enterprise-ecommerce-code-audit.md`
- `artifacts/enterprise-ecommerce-qa-risk-review.md`
