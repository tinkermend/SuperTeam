-- Seed local development administrator for Web console login smoke tests.
-- Username: admin
-- Password: admin

INSERT INTO tenants (id, slug, name, status)
VALUES (
    '00000000-0000-0000-0000-000000000001'::uuid,
    'default',
    '默认租户',
    'active'
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO tenant_teams (id, tenant_id, slug, name, status)
VALUES (
    '00000000-0000-0000-0000-000000000101'::uuid,
    '00000000-0000-0000-0000-000000000001'::uuid,
    'default',
    '默认团队',
    'active'
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO auth_users (username, display_name, email, password_hash, status)
VALUES (
    'admin',
    '开发管理员',
    'admin@superteam.local',
    '$2b$10$80xO1fy8PgNgH3qmmysLLOYe3RcHh3qVJs17hGbSqltjIJP7lNpfC',
    'active'
)
ON CONFLICT (username) WHERE deleted_at IS NULL DO NOTHING;

INSERT INTO tenant_members (tenant_id, principal_type, principal_id, role, status)
SELECT
    '00000000-0000-0000-0000-000000000001'::uuid,
    'user',
    id,
    'owner',
    'active'
FROM auth_users
WHERE username = 'admin'
ON CONFLICT DO NOTHING;
