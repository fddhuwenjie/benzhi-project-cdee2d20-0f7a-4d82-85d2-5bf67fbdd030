# 洞潜任务安全放行服务

本项目面向科研洞穴潜水团队，将任务建档、分洞段风险评估、人员与呼吸气体方案审核、装备证据核验、应急演练整改、独立监督签发和不可变归档组织为一条可追溯的 HTTP JSON 工作流。

服务使用本地 SQLite 保存任务聚合、风险与核验明细、幂等命令结果和只追加审计事件。每个写请求都必须在 JSON 顶层提供 `request_id`、`expected_revision` 和 `actor_id`。成功写入会推进 `revision`；同一载荷重复提交会返回带 `idempotent_replay=true` 的首次响应，同一 `request_id` 改变载荷会返回 `idempotency_key_reused`。过期修订号的 HTTP `409` 响应会给出当前修订号、最近事件类型和重试动作。

## 构建与运行

构建环境需要 Go 1.25 或更高版本、启用 CGO，并提供系统 SQLite 运行库 `libsqlite3.so.0`。项目不需要下载第三方 Go 模块。

```bash
go build ./cmd/server
go run ./cmd/server -addr=127.0.0.1:19081 -db=dive_missions.db
```

默认监听 `127.0.0.1:19081`，不会绑定公网地址。也可以通过 `PORT` 指定端口，服务会绑定到 `127.0.0.1:<PORT>`；显式 `-addr` 优先于 `PORT`。端口必须位于 `1024` 到 `65535`，地址必须是回环 IP。

## 测试与自检

```bash
go test ./...
go run ./cmd/server -self-check -addr=127.0.0.1:19081
```

自检会创建临时 SQLite 数据库并启动真实回环 HTTP 服务，完整执行包含演练偏差、整改、定向复验、签发和归档的业务流程，校验详情、状态历史与审计查询后主动退出。

## API 流程

所有业务 API 使用 `/api/v1` 前缀，写请求使用 `Content-Type: application/json`，请求体上限为 1 MiB。

1. `POST /api/v1/dive-missions/schedule-preflight` 只读预检规范站点时间窗并返回 `source_digest`；`POST /api/v1/dive-missions` 创建任务草案时必须在 `team_members` 旁提交一一对应的 `member_qualifications`。服务按职责、目标深度和 `window_end` 固化资格结论；`PATCH` 或 `PUT /api/v1/dive-missions/{id}` 修订成员、深度或时间窗时会重新核算全部资格。
2. `POST /api/v1/dive-missions/{id}/risks` 一次提交全部洞段风险；高风险和极高风险洞段的每个危险项必须带结构化 `mitigation_actions`。负责人可通过 `POST .../risks/mitigations/{action_code}/complete` 逐项提交，或通过 `POST .../risks/mitigations/complete-batch` 在同一事务批量提交结果和唯一证据；存在 open 或逾期行动时不能提交生命支持方案。`PUT .../risks` 或 `POST .../risks/reassess` 会为变化行动建立新版本并保留历史关联。
3. `POST /api/v1/dive-missions/{id}/life-support-plan` 提交人员、气体、转向压力、`member_gas_assignments` 和完整的 `segment_gas_budgets`。服务逐成员逐洞段模拟主用、冗余气源单点失效，保存 `scenario_margins`，任一场景为负时整笔拒绝；独立审核时会使用同一规则重新计算。
4. `POST /api/v1/dive-missions/{id}/life-support-review` 由独立审核员批准或退回方案。
5. `GET /api/v1/dive-missions/{id}/life-support-plan?plan_id=...&compare_to=...` 返回退回版本、指定版本差异与问题覆盖率。
6. `POST /api/v1/dive-missions/{id}/equipment-verifications` 逐项或批量提交装备、证据和按 `check_code` 限定的 `measurements`。服务依据深度、最长撤离时间、路线长度和最高风险计算阈值及结论；请求声明不能覆盖自动结论，批次中出现未知读数会整笔拒绝。
7. `POST /api/v1/dive-missions/{id}/drills` 登记失联和气体共享演练；偏差会固化 `remediation_due_at`。任务详情实时返回 `remediation_deadlines` 的 open、due_soon、overdue、awaiting_retest 或 closed 状态，不因此推进修订号。
8. `POST /api/v1/dive-missions/{id}/remediations` 与 `POST /api/v1/dive-missions/{id}/retests` 均兼容原子批次。逾期整改必须提交 `delay_reason` 和角色独立的 `delay_reviewed_by`；实际迟延秒数及复核信息永久进入签发与归档复盘摘要。
9. `GET /api/v1/dive-missions/{id}/release-preview` 只读返回四类门禁及其 `source_records`、`lineage`、审计 sequence、`source_revision`、`preview_digest` 和可选 `supervisor_id` 的隔离规则诊断；拒绝后重新签发必须确认拒绝修订、最新预览差异和确认理由。
10. `POST /api/v1/dive-missions/{id}/archive` 固化不可变档案；`GET /api/v1/dive-missions/{id}/archive-export` 分段导出规范快照与审计链。`GET /api/v1/dive-missions/{id}/archive/evidence` 支持按 `gate_code`、`record_id`、`actor_id` 和 `event_type` 组合定位证据，返回对应审计序号、事件哈希及到链尾的 `proof_nodes`；查询前会同时验证完整链、`archive_digest` 和冻结的 `release_digest`。

查询端点包括任务台账 `GET /api/v1/dive-missions`、任务详情 `GET /api/v1/dive-missions/{id}`、状态历史 `GET /api/v1/dive-missions/{id}/history` 和审计事件 `GET /api/v1/dive-missions/{id}/audit-events`。台账在既有条件上支持 `risk_level`、`min_total_score` 和 `mitigation_state=complete|incomplete|none`，返回完整筛选集的状态、风险等级、总分区间与缓解门禁统计以及稳定游标页。审计查询支持 `after`、`limit`、`event_type` 与 `status_after`，按序返回 `next_cursor` 以及本页链段的起止摘要；返回前会校验完整事件哈希链。

归档巡检入口 `GET /api/v1/dive-missions/archive-integrity` 支持按 `cave_site`、`archived_from`、`archived_to` 和稳定 `cursor` 分页，返回每份档案的首个完整性异常位置及全量筛选汇总。

新增只读复盘入口：`POST|GET /api/v1/dive-missions/template-preview` 用于归档模板派生草案预览（仅返回预览摘要，不写入任务）；`GET /api/v1/dive-missions/{id}/risks/mitigations` 返回可按洞段、负责人、状态和截止时间筛选的缓解行动；`GET /api/v1/dive-missions/{id}/equipment-evidence` 返回装备证据有效期分层及替换链；`GET /api/v1/dive-missions/{id}/remediation-review` 返回演练整改各轮次状态、耗时和逾期统计。上述查询均在响应前校验任务审计链，并支持稳定游标分页。

## 数据与安全约束

装备证据摘要必须是 16 到 128 位十六进制字符串，同一任务内不能跨检查项复用，同一实体装备不能承担两个必检代码；替换记录会保留在历史证据中。方案审核员必须独立于任务团队；装备核验员不能兼任领队或方案审核员；演练、整改和复验操作者相互隔离；最终签发人必须独立于任务成员、方案审核和现场核验人员。签发摘要覆盖规范排序后的风险、方案、核验记录和差异清单。归档任务的所有写端点统一返回 `immutable_archive`，详情与台账读取同时校验归档摘要和连续审计链。
