-- 070_drop_project_leader_acceptance.sql
-- 剃除项目 leader/验收人残留:产品层已放弃角色划分,项目人类成员同等身份,
-- human_owner 保留为必绑锚点与兜底路由目标。两列产品入口已无写路径,休眠数据直接删除。

ALTER TABLE projects DROP COLUMN IF EXISTS leader_user_id;
ALTER TABLE projects DROP COLUMN IF EXISTS acceptance_user_id;

COMMENT ON COLUMN project_members.project_role IS '项目内角色:owner / executor / reviewer / observer 等,应用层注册校验;人类成员同等身份,不再划分 leader/acceptance';
