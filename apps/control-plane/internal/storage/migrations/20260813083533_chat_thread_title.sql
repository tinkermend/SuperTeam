-- 会话级标题：仅根 chat task（chat_thread_id IS NULL）有意义；追问轮不写。
-- 缺省由服务端自首问截断写入；用户改名仅发起人可 PATCH。
ALTER TABLE tasks
  ADD COLUMN IF NOT EXISTS thread_title TEXT;

COMMENT ON COLUMN tasks.thread_title IS 'chat 会话标题（仅根轮）；缺省自首问截断，用户可改名；追问轮为 NULL';
