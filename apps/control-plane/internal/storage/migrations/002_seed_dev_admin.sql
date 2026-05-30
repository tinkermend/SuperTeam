-- Seed local development administrator for Web console login smoke tests.
-- Username: admin
-- Password: admin
INSERT INTO auth_users (
    username,
    display_name,
    password_hash,
    status
) VALUES (
    'admin',
    '开发管理员',
    '$2b$10$80xO1fy8PgNgH3qmmysLLOYe3RcHh3qVJs17hGbSqltjIJP7lNpfC',
    'active'
)
ON CONFLICT (username) DO NOTHING;
