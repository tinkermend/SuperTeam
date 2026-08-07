# Chat 会话持久化:切页恢复与会话边界 设计提纲
> 复核状态：已实施，现状与本设计一致：B2方案chat_thread_id（迁移067）+两查恢复最新链+新对话断链+retry孤儿链修复，真实E2E六轮全过。

- 日期:2026-07-16
- 状态:已评审定稿,待实现(§6 开放点已全部收口)
- 前置:`2026-07-13-task-hub-tri-mode-design.md`(§3 显式取舍"线程视图为页面本地状态",本 spec 接续偿还;§5 定义 chat run 架构与四条不变量,本 spec 全部沿用不动)

## 1. 背景与问题

任务中枢 chat 模式的对话线程是 `ChatPanel` 组件本地 `useState`(`apps/web/src/features/task-launches/components/chat-panel.tsx`),切菜单卸载即丢,回来是空白面板,用户体感"对话丢了"。

事实盘点(2026-07-16):**数据一轮都没丢,丢的只是前端视图。**

- 每轮对话 = 一条持久化 `DigitalEmployeeRun`(`run_kind=chat`),追问经 `resume_of_run_id` 串成链,provider 会话随链 resume(`run_types.go` `ResumeOfRunID` 语义:必须指向同员工的前序 terminal chat run)。
- 问题文本以 `Title: objective` 全文存在关联 task 上(`run_service.go:480`,无截断),列表接口回 `task_title`;回答在 `run.result`;运行状态在服务器。
- runs 列表契约已支持 `project_id` + `run_kind` + `status` 过滤(`contracts/control-plane/openapi.yaml` `/digital-employees/{id}/runs` GET)。

缺的两件事:**前端视图重建**、**会话边界语义**(什么时候算新会话)。

### 1.1 连带缺陷(本 spec 纳入修复)

`handleRetry` 重发失败条目时不带 `resume_of_run_id`(`chat-panel.tsx:245`),每次重试都从头开新链——上下文静默丢失,且在链模型下产生孤儿分叉。应与 `handleSend` 一致,携带 `lastCompleted` 的 runId。

## 2. 目标(Claude Code 式会话语义)

1. 回到 chat 页(切菜单返回、浏览器刷新)自动恢复同 (员工, 项目) 锚点下的**当前会话**(最新链),含历史问答与进行中 run 的轮询续接。
2. 显式"新对话"按钮是断链的唯一主动入口;不点则一直续在当前会话上。
3. 换员工/换项目 = 切换锚点,展示**该锚点自己的**最新链(不再是简单清空丢弃)。
4. Runtime 与 Provider 层零改动;tri-mode spec §5 四条不变量(chat 不进协调链路、产出隔离、转任务唯一出口、审计照常)不动。

## 3. 非目标

- 不做会话列表 UI、会话命名、多会话切换、历史会话回溯(`chat_thread_id` 为其铺路,届时另立 spec)。
- 不引入独立 Conversation 实体/表;会话 = resume 链,以 thread 根 id 归组。
- 不做前端本地持久化(store/sessionStorage)——服务端是唯一事实源,符合"Console 不承载长期业务状态"。
- 不升级流式呈现(维持轮询)。
- 存量 chat runs 不回填 thread id(见 §4.2 取舍)。

## 4. 方案:服务端冠 `chat_thread_id`(评审中定为 B2)

评审过的备选:A. 前端 store/sessionStorage(刷新丢、平行副本违背架构口径,弃);B1. 零后端改动、客户端沿 `resume_of_run_id` 拼链(分页边界与 retry 分叉使拼链长期脆弱,弃);**B2. 服务端 thread 根 id,一查得全链(采纳)**;C. 完整 Conversation 实体(超出 chat 轻量定位,推迟)。

### 4.1 数据库(迁移 067,编号以落地时目录为准)

- `digital_employee_runs` 新增可空列 `chat_thread_id uuid`,仅 chat run 填写。
- 索引:`(tenant_id, digital_employee_id, chat_thread_id)` 局部索引(`WHERE chat_thread_id IS NOT NULL`),支撑按链拉取。
- 遵循 `DATABASE_DESIGN.md`;更新 `atlas.sum` + `make -C apps/control-plane migrate-validate`。

### 4.2 控制平面(`internal/employee/`)

CreateRun 的 chat 分支:

- 带 `resume_of_run_id` → 继承 prior 的 `chat_thread_id`;prior 无(存量数据)则取 `prior.ID` 作根,链从此归组。
- 不带 → `chat_thread_id = 自身 run ID`(新会话即新根)。
- task run 恒为 NULL,不参与。

存量取舍:不做迁移回填。老链在新代码下续问一次即归组;从不再续的死链不会被"当前会话"恢复——dev 阶段可接受,声明即可。

### 4.3 契约(`contracts/control-plane/openapi.yaml`,改后 `generate:control-plane` + 契约验证)

- `DigitalEmployeeRun` schema 暴露 `chat_thread_id`(可空)。
- runs 列表 GET 新增查询参数 `chat_thread_id`(uuid,可选);命中时链内按 `created_at` 升序返回。
- 不新增端点。前端"恢复当前会话"= 两次查询:① `run_kind=chat&project_id=…&limit=1` 取最新 run 的 `chat_thread_id`;② 按该 id 拉全链。

### 4.4 前端(`ChatPanel`)

- **恢复**:挂载及 (employeeId, projectId) 变化时执行上述两查,把链重建为 `ChatEntry[]`(question=`task_title`,answer=`extractAnswerText(run.result)`,status 原样);链尾若是活跃状态,现有 `runQuery` 轮询自动续接直至 terminal。
- **新对话**:头部新增按钮,清空本地 thread;下一条消息不带 `resume_of_run_id` → 服务端冠新根 → 最新链自然切换。边界取舍:点了新对话但未发消息就切走,回来仍恢复旧链(无新会话事实产生,与 Claude Code /clear 后未输入即退出的行为一致),声明不修。
- **retry 修复**(§1.1):`handleRetry` 改为携带 `lastCompleted?.runId`,复用既有 400 降级重发路径。
- **已知损失**:`contextNotContinued`("上下文未延续"提示)是发送时的瞬时信息,重建不恢复;若要持久可后续写入 run metadata,本版不做。
- 链长兜底:首版单页拉取(limit 100),超长截断为最近 100 条并在顶部提示;不做翻页加载。

## 5. E2E 验证清单(真实链路,完成条件)

1. 同一锚点发两轮对话(第二轮为追问)→ 切任意菜单页 → 返回:两轮问答完整恢复,追问上下文在 provider 侧仍延续(第三问验证)。
2. 浏览器整页刷新 → 恢复同上。
3. 发出问题后立即切走,run 仍在跑 → 返回:进行中条目恢复且轮询续接至完成出答案。
4. 点"新对话"→ 发新问题 → 切走返回:只见新会话;老会话不出现(会话列表属后续 spec)。
5. 换项目/换员工再换回:各锚点恢复各自最新链。
6. 失败条目重试:携带 resume,成功后仍在原链上(切走返回可见完整链)。
7. 存量老链(无 thread id):续问一次后可被恢复。

## 6. 评审决议(2026-07-16 收口)

1. **恢复时效上限:不设**(人类负责人拍板)。恢复只是读取服务端既有事实,无额外损耗,与 Claude Code 恢复会话的行为对齐。备注:若未来确需时效,负责人预设值为 1 天。
2. **链内 run 内容被清理/过期时:条目保留,答案区降级显示"内容已过期"**。链结构与问题文本(task title)长期在库,过期的只是 result/transcript 内容;保留条目维持会话完整性。若 provider 会话同时失效,续问自然走既有 400 降级重发路径("上下文未延续"提示),无需新逻辑。
3. **`chat_thread_id` 不进审计导出,本版只在 run 对象与列表过滤暴露**。审计记录已含 `resume_of_run_id`,链可离线重建,无审计信息损失;thread 归组是视图层便利,不是新的业务事实。
