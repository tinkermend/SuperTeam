-- 为 Web 用户管理相关表补充中文注释，便于后续运维、审计和数据字典生成。

COMMENT ON TABLE auth_users IS 'Web 控制台平台用户表';
COMMENT ON COLUMN auth_users.id IS '用户主键 ID';
COMMENT ON COLUMN auth_users.username IS '登录账号，平台内唯一';
COMMENT ON COLUMN auth_users.display_name IS '用户展示名称，当前 MVP 可为空';
COMMENT ON COLUMN auth_users.email IS '用户邮箱，当前 MVP 可为空';
COMMENT ON COLUMN auth_users.password_hash IS '用户密码哈希，禁止存储明文密码';
COMMENT ON COLUMN auth_users.status IS '用户状态：active 表示启用，disabled 表示禁用';
COMMENT ON COLUMN auth_users.created_at IS '用户创建时间';
COMMENT ON COLUMN auth_users.updated_at IS '用户最后更新时间';

COMMENT ON TABLE web_operation_logs IS 'Web 控制台操作日志表';
COMMENT ON COLUMN web_operation_logs.id IS '操作日志主键 ID';
COMMENT ON COLUMN web_operation_logs.user_id IS '执行操作的用户 ID，用户删除后保留日志';
COMMENT ON COLUMN web_operation_logs.username IS '执行操作的用户账号快照';
COMMENT ON COLUMN web_operation_logs.module IS '操作所属模块';
COMMENT ON COLUMN web_operation_logs.resource_type IS '被操作资源类型';
COMMENT ON COLUMN web_operation_logs.resource_id IS '被操作资源 ID';
COMMENT ON COLUMN web_operation_logs.action IS '操作动作';
COMMENT ON COLUMN web_operation_logs.result IS '操作结果：succeeded 或 failed';
COMMENT ON COLUMN web_operation_logs.request_id IS '请求 ID，便于链路追踪';
COMMENT ON COLUMN web_operation_logs.client_ip IS '客户端 IP';
COMMENT ON COLUMN web_operation_logs.user_agent IS '客户端 User-Agent';
COMMENT ON COLUMN web_operation_logs.details IS '操作上下文扩展信息';
COMMENT ON COLUMN web_operation_logs.created_at IS '操作发生时间';
