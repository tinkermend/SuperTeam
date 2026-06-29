-- name: CreateCaptchaChallenge :one
INSERT INTO auth_captcha_challenges (
    id,
    tenant_id,
    answer_hash,
    expires_at,
    client_ip,
    user_agent
) VALUES (
    sqlc.arg('id')::uuid,
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('answer_hash')::varchar,
    sqlc.arg('expires_at')::timestamptz,
    sqlc.narg('client_ip')::varchar,
    sqlc.narg('user_agent')::text
) RETURNING *;

-- name: GetCaptchaChallengeForUpdate :one
SELECT * FROM auth_captcha_challenges
WHERE id = sqlc.arg('id')::uuid
FOR UPDATE;

-- name: ConsumeCaptchaChallenge :execrows
UPDATE auth_captcha_challenges
SET used_at = sqlc.arg('used_at')::timestamptz,
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND used_at IS NULL;

-- name: DeleteExpiredCaptchaChallenges :exec
DELETE FROM auth_captcha_challenges
WHERE expires_at < sqlc.arg('before')::timestamptz;
