-- 071_feishu_integration.sql
-- 飞书集成 P1:租户级飞书应用配置、用户身份绑定、外部服务凭据、出站消息 outbox。
-- 设计依据 docs/superpowers/specs/2026-07-17-feishu-integration-design.md §9。

-- 租户级飞书应用配置:secret 经 AES-GCM sealer 加密存储,connector 经 bootstrap 端点拉取。
CREATE TABLE IF NOT EXISTS feishu_app_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    app_id VARCHAR(64) NOT NULL,
    app_secret_sealed TEXT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_feishu_app_configs_tenant_app UNIQUE (tenant_id, app_id)
);

COMMENT ON TABLE feishu_app_configs IS '租户级飞书应用配置(企业自建应用凭据)';
COMMENT ON COLUMN feishu_app_configs.id IS '配置记录ID';
COMMENT ON COLUMN feishu_app_configs.tenant_id IS '所属租户ID';
COMMENT ON COLUMN feishu_app_configs.app_id IS '飞书应用 App ID(公开标识,明文)';
COMMENT ON COLUMN feishu_app_configs.app_secret_sealed IS '飞书应用 App Secret(AES-GCM sealer 加密后的密文,禁止明文入库)';
COMMENT ON COLUMN feishu_app_configs.status IS '配置状态:active / disabled,应用层校验';
COMMENT ON COLUMN feishu_app_configs.created_at IS '创建时间';
COMMENT ON COLUMN feishu_app_configs.updated_at IS '更新时间';

-- 用户飞书身份绑定:open_id 只经 bot 事件/OAuth/通讯录反查写入,禁止用户手填。
CREATE TABLE IF NOT EXISTS user_feishu_identities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    auth_user_id UUID NOT NULL,
    feishu_app_config_id UUID NOT NULL,
    open_id VARCHAR(128) NOT NULL,
    union_id VARCHAR(128),
    bound_via VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_feishu_identity_open UNIQUE (feishu_app_config_id, open_id),
    CONSTRAINT uq_feishu_identity_user UNIQUE (feishu_app_config_id, auth_user_id)
);

CREATE INDEX IF NOT EXISTS idx_user_feishu_identities_tenant_user
    ON user_feishu_identities(tenant_id, auth_user_id);

COMMENT ON TABLE user_feishu_identities IS '平台用户与飞书身份绑定(open_id 一人一绑,双向唯一)';
COMMENT ON COLUMN user_feishu_identities.id IS '绑定记录ID';
COMMENT ON COLUMN user_feishu_identities.tenant_id IS '所属租户ID';
COMMENT ON COLUMN user_feishu_identities.auth_user_id IS '平台用户ID(auth_users.id)';
COMMENT ON COLUMN user_feishu_identities.feishu_app_config_id IS '归属飞书应用配置ID(feishu_app_configs.id)';
COMMENT ON COLUMN user_feishu_identities.open_id IS '飞书 open_id(按应用隔离的用户标识,来源为飞书平台事件/OAuth,不可伪造)';
COMMENT ON COLUMN user_feishu_identities.union_id IS '飞书 union_id(跨应用标识,多飞书企业兼容预留,可空)';
COMMENT ON COLUMN user_feishu_identities.bound_via IS '绑定方式:contact_sync(通讯录反查) / oauth(用户授权),应用层注册校验';
COMMENT ON COLUMN user_feishu_identities.created_at IS '绑定时间';

-- 外部服务凭据:仿 auth_runtime_tokens,token 只存哈希,可吊销;第一消费者 feishu-connector。
CREATE TABLE IF NOT EXISTS auth_service_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    service_name VARCHAR(64) NOT NULL,
    token_hash VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_auth_service_tokens_service
    ON auth_service_tokens(service_name, status);

COMMENT ON TABLE auth_service_tokens IS '外部服务凭据(服务身份认证,on-behalf-of 判权仍以绑定用户为行为人)';
COMMENT ON COLUMN auth_service_tokens.id IS '凭据记录ID';
COMMENT ON COLUMN auth_service_tokens.tenant_id IS '所属租户ID';
COMMENT ON COLUMN auth_service_tokens.service_name IS '服务名(如 feishu-connector),与请求头 X-Service-Name 匹配';
COMMENT ON COLUMN auth_service_tokens.token_hash IS '凭据哈希(明文只在签发时返回一次,不落库)';
COMMENT ON COLUMN auth_service_tokens.status IS '凭据状态:active / revoked,应用层校验';
COMMENT ON COLUMN auth_service_tokens.last_used_at IS '最近使用时间(审计用)';
COMMENT ON COLUMN auth_service_tokens.created_at IS '签发时间';
COMMENT ON COLUMN auth_service_tokens.revoked_at IS '吊销时间';

-- 飞书出站 outbox:决策卡/结果通知与业务写同事务入队,connector 轮询消费+ack,三层幂等之一。
CREATE TABLE IF NOT EXISTS feishu_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID,
    kind VARCHAR(64) NOT NULL,
    resource_type VARCHAR(64) NOT NULL,
    resource_id UUID NOT NULL,
    recipient_user_id UUID NOT NULL,
    recipient_open_id VARCHAR(128) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT,
    feishu_message_id VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_feishu_outbox_pending
    ON feishu_outbox(tenant_id, status, created_at)
    WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_feishu_outbox_resource
    ON feishu_outbox(tenant_id, resource_type, resource_id);

COMMENT ON TABLE feishu_outbox IS '飞书出站消息队列(与业务写同事务入队,投影不阻塞业务;connector 轮询+ack)';
COMMENT ON COLUMN feishu_outbox.id IS 'outbox 行ID';
COMMENT ON COLUMN feishu_outbox.tenant_id IS '所属租户ID';
COMMENT ON COLUMN feishu_outbox.project_id IS '关联项目ID(可空)';
COMMENT ON COLUMN feishu_outbox.kind IS '消息种类:decision_card(可操作审批卡) / card_update(决策终态卡片更新) / result_notice(只读结果通知),应用层注册';
COMMENT ON COLUMN feishu_outbox.resource_type IS '来源资源类型(如 decision_request / project_demand)';
COMMENT ON COLUMN feishu_outbox.resource_id IS '来源资源ID';
COMMENT ON COLUMN feishu_outbox.recipient_user_id IS '收件人平台用户ID(收件人展开在写入时按合格处理人集合×绑定表完成)';
COMMENT ON COLUMN feishu_outbox.recipient_open_id IS '收件人飞书 open_id(写入时冗余快照,消费侧免反查)';
COMMENT ON COLUMN feishu_outbox.payload IS '卡片渲染所需业务快照(标题/摘要/判据/深链等)';
COMMENT ON COLUMN feishu_outbox.status IS '状态:pending / sent / failed / skipped_unbound / superseded,应用层状态机';
COMMENT ON COLUMN feishu_outbox.attempts IS '投递尝试次数(3 次失败标 failed)';
COMMENT ON COLUMN feishu_outbox.last_error IS '最近一次投递失败原因';
COMMENT ON COLUMN feishu_outbox.feishu_message_id IS '飞书消息ID(发送成功后回填,用于后续卡片更新)';
COMMENT ON COLUMN feishu_outbox.created_at IS '入队时间';
COMMENT ON COLUMN feishu_outbox.updated_at IS '更新时间';
