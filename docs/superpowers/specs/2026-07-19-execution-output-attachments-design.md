# 执行输出附件自动采集与预览 Spec：任务产出文件回传平台可看可下载

> 日期：2026-07-19
> 复核状态：v1+§1+v2全量完结（c5733467/23c551c2/8359a52f裹挟）
> 状态：**v1 已实施，GATE 五项真实 E2E 全 PASS（2026-07-19 01:50，未提交，详见 CHANGELOG）**。实施中修订两处：①噪音排除补"任何隐藏路径组件"（§1.3，E2E 实证 .superteam 误采）；②HTML 预览由 iframe src 直连 302 改为 fetch+srcDoc（§3，TOS 强制 Content-Disposition: attachment 致 sandbox 内空白）+ 桶 CORS 需含 `null` origin（fetch 跨域重定向 Origin taint）。遗留：JSON presign 变体待立项（见 §6 后注）。
> 性质：Runtime 采集面扩展 + Web 预览。v1 = 无需 agent 配合的自动兜底捕获；声明式交付物契约闭环（produces → artifact 强关联）为 v2，另行立项。
> 关联：证据地基 spec（07-17，presign/内容寻址/302 管道即其产物，本 spec 全量复用）；intent/acceptance 判据线（deliverable 与 artifact 松关联的残债由 v2 偿还，非本 spec 范围）。

---

## 0. 动因与现状

**问题**：Runtime 是远端的。任务（如"分析系统并生成 HTML 报告"）在远端工作目录产出 report.html / report.md 后，文件躺在远端机器上，平台上既看不到也下不到。

**现状链路（2026-07-19 实测摸底）**：

- 存储与取回管道**已全部建成**：runtime 经 CP presign 直传对象存储（零凭证）、内容寻址 `artifacts/{tenant}/sha256/{hex}` 幂等去重、`GET /api/v1/artifacts/{id}/content` 302 presigned 下载、`project_artifact_refs` 落库（attempt 血缘 + retention）、项目治理 tab 工件面板列表+下载。
- 缺口一：**采集侧不看工作目录**。`collect_artifacts()`（`apps/runtime-agent/src/artifacts.rs:57`）只固定采 transcript / `git diff HEAD` / conclusion 三个 curated 工件，agent 写盘的报告文件任何机制都不会碰。
- 缺口二：**Web 无预览**。`project-artifact-report-panel.tsx` 只有 302 下载链接，HTML/Markdown 不渲染。

**用户拍板的采集哲学**：不从任务发起的自然语言里解析猜测哪些是交付物，不做全量扫描+黑名单过滤。用**白名单反选**：只捕获满足显式规则的文件，其余一切默认是中间文件，无需任何"中间文件界定"机制。

## 1. 采集规则（Runtime 侧）

一句话：**本次执行新写入（untracked）∩ 类型白名单 ∖ 噪音目录，加数量/大小熔断**。

### 1.1 范围 = 只收 untracked 新文件（用户拍板）

- 以任务开始时工作目录的 git 状态为基线，执行结束后 `git ls-files --others --exclude-standard` 的 **untracked 文件**即"本次任务新产生的文件"，精确集合，零启发式。
- **已知局限（拍板接受）**：`--exclude-standard` respect .gitignore——若 agent 把报告写进被 ignore 的路径（如被 ignore 的 `dist/` 或 `*.html`），文件会**静默不被采集且 runtime 无从留痕**（git 不报告 ignored 文件）。换取的是免费的噪音过滤（构建产物天然被 gitignore 覆盖，§1.3 排除表退为第二道防线）。缓解：dispatch 注入的执行约定里提示 agent 勿把交付物写进 ignored 路径；根治靠 v2 声明式。
- **modified（改动原有文件）不收**：该信息已被 transcript（会话记录）+ diff 证据工件完整覆盖，整文件回传是重复。
- 项目原有文件（tracked 且未动）绝不触碰。
- 非 git 工作目录兜底（个别 chat/临时目录）：任务启动时做一次文件清单快照，结束后对比得出新增集合，语义同上。

### 1.2 类型白名单

- v1 默认（用户拍板 2026-07-19）：`.html` / `.md` / `.txt`（可预览）+ `.csv` / `.json` / `.docx` / `.doc` / `.xlsx`（仅下载）。
- 白名单是 runtime 配置项，不硬编码为封闭枚举（遵循"注册表与服务端校验"原则）；后续扩 `.csv` / `.json` / `.pdf` 等仅下载型只改配置。
- 上传时按扩展名设置 `content_type`（`text/html` / `text/markdown` / `text/plain`），Web 预览分流依赖它。

### 1.3 噪音目录排除

排除常见 vendor/构建目录：`node_modules/`、`target/`、`dist/`、`build/`、`vendor/`、`__pycache__/` 等（runtime 配置，同 1.2 可扩）；另外**任何隐藏（点开头）路径组件一律排除**（涵盖 `.git`/`.venv`/`.superteam`/`.claude` 等）——runtime 与 provider 的元数据目录可能含敏感配置，隐藏文件不是交付物。此规则由首轮 E2E 实证补上：`.superteam/mcp/claude.mcp.json` 曾被白名单误采（2026-07-19）。

### 1.4 熔断（防爆炸，no silent caps）

- 单文件上限：**5MiB（用户拍板 2026-07-19，知悉风险后定）**——已知后果：内联 base64 图表/截图的自包含 HTML 报告容易超限被跳过（留痕，不静默丢）；隐含导向是报告轻量化，重资产拆分。好处：5MiB < CP 侧现行 `ArtifactMaxFileSizeBytes` 10MiB，**CP 侧无需改动**。该值为 runtime 配置项，未来迁入平台配置管理功能（§5）后可调。超限**跳过不 truncate**——附件截断即残品，与证据类工件 truncate 保尾的语义不同；跳过必须留痕（见下）。
- 数量上限：单次执行最多 **20 个**附件；超出按文件 mtime 取最新 20 个。
- 总量上限：单次执行附件合计 **50MiB**。
- 任何因熔断被丢弃的文件，在 conclusion/completion metadata 里留痕（文件名+原因），不得静默截断。

### 1.5 工件语义与上传

- 新增 `artifact_type = execution_output`（应用层注册校验，与现有类型并列），`is_evidence = false`——这是"自动兜底捕获的自报级产出"，不是平台核实的证据，也不是契约承诺的交付物（后者是 v2 声明式的事）。
- `metadata` JSONB 记录工作目录相对路径（`relative_path`），供 UI 展示与 v2 对齐 produces 用。
- **不走 prose 脱敏链路**：文件是 agent 原样产出，UI 上明确标注；脱敏只覆盖 transcript/conclusion 不变。
- 采集时机与现有三工件一致：complete writeback 之前同批采集。**上传失败语义与证据工件不同**：现有 `upload_artifacts()` 任一失败整批失败（证据的正确语义——不许声称未落库的证据），但附件是 best-effort 兜底，**单个附件上传失败不得拖垮 completion**——附件独立上传、失败留痕（文件名+原因进 metadata）不阻断，证据三工件维持整批失败不变。`wait_human` 分支维持现状不采集。

## 2. Control Plane 侧

- **无 schema 变更**：`project_artifact_refs` 现有字段（artifact_type/content_type/checksum/metadata/attempt 血缘）足够承载。
- `artifact_type` 注册表登记 `execution_output`。
- 物化路径复用 `materializeTaskCompletionEvidence()`，无逻辑改动（新类型随 artifact_refs 自然入库）。

## 3. Web 侧：预览与下载分流

工件面板（`project-artifact-report-panel.tsx`）按 `content_type` 分流：

- `text/markdown`、`text/plain`：拉取内容（走 302 端点），前端 sanitize 后渲染预览（md 渲染 / 等宽文本）。**⚠ CORS 硬依赖**：fetch 会跟随 302 到对象存储的跨域 presigned URL，受 CORS 约束——**bucket 必须配置允许 web origin**，否则预览白屏（HTML iframe 是导航请求不受 CORS 限制，无此问题）。实施与联调时先落 bucket CORS 配置，GATE 含此断言。
- `text/html`：**sandbox iframe 预览**。安全模型：agent 产出的 HTML 等同不可信用户内容——iframe `sandbox="allow-scripts"`（报告常含交互图表脚本），**绝不给 `allow-same-origin`**（否则报告内脚本可拿平台 cookie/storage）；src 直指 302 content 端点（presigned 域与平台域天然隔离，双保险）。
- 其他类型：维持现状，仅 302 下载链接。
- 展示分组：`execution_output` 附件与证据工件（transcript/diff）分区展示，附件标注"agent 原样产出，未经平台脱敏"。
- 预览交互形态（面板内展开 vs 弹层 vs 详情页）实施时按 DESIGN.md 定，改前必读。

## 4. GATE（真实 E2E，全过才算完成）

1. **正向**：发起真实任务让 provider（claude）在工作目录生成 `report.html` + `notes.md`，另在 `node_modules/` 下写一个 .md——完成后平台工件列表只出现前两者（噪音目录排除生效），类型/大小/checksum 正确，302 下载内容与远端原文件一致。
2. **预览**：Web 上 report.html 在 sandbox iframe 中渲染；notes.md 渲染为富文本；iframe 无 `allow-same-origin`（DOM 断言）。
3. **modified 不收**：任务中改动一个仓库原有文件——该文件不出现在附件列表，但 diff 证据工件包含其变更。
4. **熔断留痕**：造一个 >10MiB 的 .txt——不入库，completion 留痕可见丢弃原因。
5. **原有文件不触碰**：工作目录预置的 tracked .md 文件不被采集。

## 5. 非目标（v2 及以后，另行立项）

- 声明式交付物闭环：dispatch 注入 produces 清单、约定 `deliverables/` 目录、`TaskResultDeliverable.Ref` 回填 `artifact_ref_id`、判据→deliverable→文件血缘打通验收面板。
- **平台配置管理功能（用户方向 2026-07-19）**：附件大小/数量熔断、类型白名单、排除目录等散落配置项，未来收进统一的配置管理功能分门别类定义；本 spec 各限值先以 runtime 配置项落地（默认值写死可改），为迁移留口。
- 附件的保留/GC 策略（中间文件不上传，无存储压力；附件量到达痛点再议）。
- `wait_human` 阶段性采集、执行过程中的增量采集。
- 报告类富预览增强（目录、多文件站点式 HTML）。

## 6. 开放问题（实施前可默认，有异议再拍）

1. ~~附件单文件上限~~ 已拍板：**5MiB**（§1.4，用户 2026-07-19 知悉 HTML 报告超限风险后定，CP 侧免改）。数量/总量熔断维持默认 20 个 / 总量 50MiB。
2. ~~白名单范围~~ 已拍板：含 `.csv` / `.json` / `.docx` / `.doc` / `.xlsx`（仅下载不预览）。开放问题全部关闭，进入实施。

## 7. 诚实性自审记录（2026-07-19，用户要求的反迎合校验）

对本设计做过一轮找茬式自审，结论：主体可落地（管道存在性/git 同构操作/CP 零 schema 变更均为实测），但自审揪出 4 个原稿未覆盖的真实问题，已修入正文：

1. `.gitignore` 静默吞 untracked（§1.1 已知局限）——原稿完全未提。
2. 10MiB 上限卡自包含 HTML 报告这一头号用例（§1.4）。
3. md/txt 预览的 bucket CORS 硬依赖（§3）——不写明会成联调"神秘失败"。
4. 原稿"复用整批失败语义"是照抄现状的**错误设计**，附件必须 best-effort 不拖垮 completion（§1.5 已改）。

价值代价的明面化：v1 必然混入 agent 草稿类噪音（notes.md 等），白名单+上限只缓解不根治，根治靠 v2 声明式——接受此代价换"零 agent 配合、三 provider 统一生效、最低成本解决报告可见"的燃眉之急。若"工件列表出现草稿"不可接受，应跳过 v1 直做声明式。
