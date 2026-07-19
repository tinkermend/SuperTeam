-- 收件箱 SSE 脏通知探测(PeekInboxChange)按 (tenant_id, updated_at, id) 游标增量扫描;
-- 既有索引均以 status/last_activity_at 为序,无法服务 updated_at 游标谓词。
CREATE INDEX idx_inbox_items_tenant_updated_id
    ON inbox_items(tenant_id, updated_at, id);

COMMENT ON INDEX idx_inbox_items_tenant_updated_id IS '收件箱变更流游标探测索引';
