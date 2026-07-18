-- 系统配置中心(spec 2026-07-19):配置定义与默认值在服务端注册表(internal/systemconfig),
-- 本表只存管理员显式修改的覆盖值;"恢复默认" = 删除覆盖行。
CREATE TABLE system_config_overrides (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    config_key VARCHAR(128) NOT NULL,
    value JSONB NOT NULL,
    updated_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_system_config_overrides_tenant_key
    ON system_config_overrides (tenant_id, config_key);

COMMENT ON TABLE system_config_overrides IS '平台级系统配置覆盖值;配置项定义(key/类型/默认值/边界/文案)在服务端注册表,本表只存管理员显式修改的覆盖,删除行即恢复默认';
COMMENT ON COLUMN system_config_overrides.tenant_id IS '租户 ID;平台级参数按租户隔离,无租户上下文的读取方使用平台默认租户';
COMMENT ON COLUMN system_config_overrides.config_key IS '配置项 key,点分命名(如 artifact.max_file_size_bytes),必须存在于服务端注册表';
COMMENT ON COLUMN system_config_overrides.value IS '覆盖值,JSON 标量;类型与边界由服务端注册表校验后写入';
COMMENT ON COLUMN system_config_overrides.updated_by IS '最后修改人用户 ID,展示与审计冗余,应用层维护不加外键';
