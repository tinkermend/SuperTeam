-- 085: Drop tables of four user-retired features (07-19 拍板).
--
-- 共同点：功能代码接线完整但历史上零数据，产品方向上已明确不再需要；
-- 代码侧同一提交内全链下线（模块/路由/契约/前端/协调器接线）。
--
-- ① 团队借调（026）：team_lending_policy / team_lending_request。
--    协调器的借调门禁降级为纯团队边界门禁（跨团队员工一律不可用，
--    与授权表恒空时的现行行为一致）；planning_gap 出路选项移除 lending。
-- ② 团队成员角色申请（004）：tenant_team_member_role_requests。
-- ③ 员工工作区文件下发（017）：digital_employee_workspace_file_syncs /
--    _revisions / _files 三表。创建入口从未建成，CP 从不下发
--    sync_workspace_files 命令；runtime 侧命令标签落 Unsupported 兜底。
--
-- 保留（用户拍板）：project_budget_ledger（项目 Token/成本流水）、
-- project_report_refs、project_transfer_requests（任务转派协议，待补完）。

DROP TABLE IF EXISTS team_lending_request;
DROP TABLE IF EXISTS team_lending_policy;
DROP TABLE IF EXISTS tenant_team_member_role_requests;
DROP TABLE IF EXISTS digital_employee_workspace_file_syncs;
DROP TABLE IF EXISTS digital_employee_workspace_file_revisions;
DROP TABLE IF EXISTS digital_employee_workspace_files;
