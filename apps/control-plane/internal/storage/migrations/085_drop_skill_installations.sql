-- 085_drop_skill_installations.sql
-- 能力绑定统一收尾(见 079):市场"安装"已改纯逻辑绑定,物理安装事实由派发时
-- runtime 懒收敛 + attestation(capability_manifest_version/capability_convergence)
-- 承载。本表 079 起冻结(停写停读,端点已删),用户拍板正式下线。
DROP TABLE IF EXISTS skill_installations;
