-- 项目需求生命周期状态扩展
-- project_demands.status 由应用层校验，新增 planned / executing / completed / failed 终态，
-- 使需求状态在协调规划、任务分派与全部任务终态后能真实推进，不再恒为 planning_pending。
-- 列类型保持 VARCHAR(50) 无 CHECK 约束，仅更新注释以保持 schema 自文档化。

COMMENT ON COLUMN project_demands.status IS '需求状态：submitted, recorded, planning_pending, planned, executing, completed, failed, cancelled';
