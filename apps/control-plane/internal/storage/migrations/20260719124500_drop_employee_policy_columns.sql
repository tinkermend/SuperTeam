-- 数字员工个体策略字段下线（第二步 teardown，用户拍板 2026-07-19）。
-- 立项与拆除清单见 docs/superpowers/specs/2026-07-19-employee-policy-columns-teardown.md。
--
-- 两列是无消费方的死字段：全库恒 '{}'，创建 UI 只读展示已于第一步移除，
-- 前端不再提交。员工个体 approval_policy 零运行期消费（真实审批策略在团队
-- 配置与项目层，勿混淆）；context_policy 仅剩规划画像透传
-- max_context_classification 一处（实践恒空），随本次一并拆除。
-- 无视图/索引依赖（pg_views 全库核对过）。风险等级默认推导
-- （min_risk_for_human）经核实早已死路：模板 ToDefinition 恒置空 map，
-- 推导结果恒为 medium，代码侧同步收敛为常量。

ALTER TABLE digital_employees
    DROP COLUMN context_policy,
    DROP COLUMN approval_policy;
