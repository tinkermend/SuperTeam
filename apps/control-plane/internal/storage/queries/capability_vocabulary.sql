-- name: ListCapabilityVocabulary :many
SELECT * FROM capability_vocabulary
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
  AND status = 'active'
ORDER BY vocab_key ASC;

-- name: GetCapabilityVocabularyByKeys :many
SELECT * FROM capability_vocabulary
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND vocab_key = ANY(sqlc.arg('vocab_keys')::text[])
  AND deleted_at IS NULL
  AND status = 'active';
