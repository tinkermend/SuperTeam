ALTER TABLE auth_users
    ADD COLUMN IF NOT EXISTS avatar_provider VARCHAR(50) NOT NULL DEFAULT 'dicebear',
    ADD COLUMN IF NOT EXISTS avatar_style VARCHAR(100) NOT NULL DEFAULT 'adventurer',
    ADD COLUMN IF NOT EXISTS avatar_seed VARCHAR(255),
    ADD COLUMN IF NOT EXISTS avatar_options JSONB NOT NULL DEFAULT '{}'::jsonb;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_auth_users_avatar_provider'
    ) THEN
        ALTER TABLE auth_users
            ADD CONSTRAINT chk_auth_users_avatar_provider CHECK (avatar_provider IN ('dicebear'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_auth_users_avatar_style'
    ) THEN
        ALTER TABLE auth_users
            ADD CONSTRAINT chk_auth_users_avatar_style CHECK (avatar_style <> '');
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_auth_users_avatar_options_object'
    ) THEN
        ALTER TABLE auth_users
            ADD CONSTRAINT chk_auth_users_avatar_options_object CHECK (jsonb_typeof(avatar_options) = 'object');
    END IF;
END $$;

COMMENT ON COLUMN auth_users.avatar_provider IS '用户头像来源，MVP 使用 DiceBear 生成稳定卡通头像';
COMMENT ON COLUMN auth_users.avatar_style IS '用户头像样式标识，MVP 默认为 DiceBear adventurer';
COMMENT ON COLUMN auth_users.avatar_seed IS '用户头像生成种子；为空时由服务端使用 username 生成稳定种子';
COMMENT ON COLUMN auth_users.avatar_options IS '用户头像生成选项 JSON，保留颜色、配件等后续扩展配置';
