# 项目 approval_policy / evidence_policy 字段退役

**状态**: 已落地  
**日期**: 2026-07-22  

## 背景

`projects` 表的 `approval_policy` 和 `evidence_policy` JSON 列（迁移早期加入）在 runtime 侧从未有消费者读取；create/update 路径虽然写入，但写入值对执行流程无任何影响。继续维护这两列只会造成 API surface 虚胖和测试噪音。

存活的协调策略字段为 `coordination_policy.require_human_review_for_new_demands`，是项目创建与配置页的唯一活跃 toggle。

## 变更内容

1. **数据库**：`ALTER TABLE projects DROP COLUMN approval_policy, DROP COLUMN evidence_policy`（迁移 `20260722004800`）。
2. **Control Plane**：`internal/project` 包删除 `ApprovalPolicy`/`EvidencePolicy` 字段及相关读写逻辑。
3. **OpenAPI**：从 `Project`、`ProjectConfig`、Create/Update 请求体删除这两个字段；保留 `DigitalEmployeePolicyDefaults.approval_policy`（员工侧，不受影响）。
4. **Web**：
   - `create-project-draft.ts`：`buildProjectCreateInput` 不再发送 `approval_policy`/`evidence_policy`；保留 `coordination_policy.require_human_review_for_new_demands` toggle 及 `newDemandNeedsHumanConfirmation` 字段；步骤标签改为「协调策略」。
   - `project-config-page.tsx`：`ConfigDraft` 移除 `approvalPolicy`/`evidencePolicy` 字段，`saveConfig` 不再包含这两个字段。
   - `project-config-revision-history.tsx`：只保留 `coordination_policy` section。
   - `apps/web/src/lib/api/projects.ts`：从 `Project`/`ProjectConfig`/Create/Update 类型中删除这两个字段。
