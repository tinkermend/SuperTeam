CREATE TABLE IF NOT EXISTS auth_captcha_challenges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    answer_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    client_ip VARCHAR(255),
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_auth_captcha_challenges_expires_at
    ON auth_captcha_challenges(expires_at);

CREATE INDEX IF NOT EXISTS idx_auth_captcha_challenges_used_at
    ON auth_captcha_challenges(used_at);

CREATE INDEX IF NOT EXISTS idx_auth_captcha_challenges_created_at
    ON auth_captcha_challenges(created_at DESC);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_trigger
        WHERE tgname = 'update_auth_captcha_challenges_updated_at'
          AND tgrelid = 'auth_captcha_challenges'::regclass
    ) THEN
        CREATE TRIGGER update_auth_captcha_challenges_updated_at
            BEFORE UPDATE ON auth_captcha_challenges
            FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

COMMENT ON TABLE auth_captcha_challenges IS 'Web 登录图形验证码挑战表';
COMMENT ON COLUMN auth_captcha_challenges.id IS '验证码挑战主键 UUID';
COMMENT ON COLUMN auth_captcha_challenges.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN auth_captcha_challenges.answer_hash IS '验证码答案哈希，不保存明文';
COMMENT ON COLUMN auth_captcha_challenges.expires_at IS '验证码过期时间';
COMMENT ON COLUMN auth_captcha_challenges.used_at IS '验证码消费时间；非空表示已使用';
COMMENT ON COLUMN auth_captcha_challenges.client_ip IS '创建验证码的客户端 IP';
COMMENT ON COLUMN auth_captcha_challenges.user_agent IS '创建验证码的 User-Agent';
COMMENT ON COLUMN auth_captcha_challenges.created_at IS '创建时间';
COMMENT ON COLUMN auth_captcha_challenges.updated_at IS '更新时间';
