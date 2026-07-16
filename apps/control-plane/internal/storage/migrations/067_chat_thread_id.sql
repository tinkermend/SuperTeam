-- 067_chat_thread_id.sql
-- Chat 会话持久化:chat run 归组的 thread 根 id。
-- 见 docs/superpowers/specs/2026-07-16-chat-session-persistence-design.md
--
-- 存储约定:首轮(不带 resume_of_run_id)存 NULL,有效 thread id = 自身 run id;
-- 追问轮继承前序 run 的有效 thread id(= 根 run 的 task_runs.id)。
-- 读侧统一以 COALESCE(tasks.chat_thread_id, task_runs.id) 投影,仅 run_kind='chat' 有值。

ALTER TABLE tasks
    ADD COLUMN chat_thread_id UUID;

CREATE INDEX idx_tasks_tenant_chat_thread
    ON tasks (tenant_id, chat_thread_id)
    WHERE chat_thread_id IS NOT NULL;

COMMENT ON COLUMN tasks.chat_thread_id IS 'chat 会话根 id(根 run 的 task_runs.id);首轮为 NULL(有效值=自身 run id),追问轮继承前序有效值。仅 chat run 使用,无 FK。';
