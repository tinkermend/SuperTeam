# 角色治理界面（Role Governance Console）

- 日期：2026-08-05
- 状态：**立项（未实施）**
- 系列：剧本可落地化的界面补齐（批一 `7a7064b5`；批二 `7b0369de` + `1f36c944`）
- 交付性质：**纯 Web + 少量只读端点**；无 schema 变更、无新业务规则
- 目标读者：实施会话（本文自包含）
- **是批三 `2026-08-05-semantic-casting-expansion-design.md` 的前置**

---

## 0.0 开工须知（实施会话先读这一节）

### 两份 spec 的关系

| | 谁先 | 说明 |
|---|---|---|
| 本文（角色治理界面） | **先** | P0a+P0b 是批三的硬前置 |
| `2026-08-05-semantic-casting-expansion-design.md`（批三 语义扩编） | 后 | 其 H2/H9b 判据依赖本文的 P0a/P0b |

**阻塞点只有两条**：批三的「去注册角色」深链需要本文 P0a 的页面做落点；批三"发现器建议某角色 → 人选人"需要本文 P0b 让员工真的持有该角色，否则候选列表是空的。
本文 **P0c（剧本只读视图）不阻塞任何人**，可后置或并行。批三的 P0a（发现器纯引擎，DB-free）与 P1（planner 注入）也不依赖本文，可并行。

### 环境

服务用 `./scripts/dev-services.sh start|status|restart <service>` 管理（OpenFGA 需单独管）。

| 项 | 值 |
|---|---|
| Web | `http://127.0.0.1:3100`（**不是 3000**） |
| Control Plane | `http://127.0.0.1:8080` |
| 登录 | `POST /api/auth/login`，`admin` / `admin` |
| 数据库 | `apps/control-plane/config/config.yaml` 的 `postgres.url`（远程 dev 库，**无备份**） |
| 迁移校验 | 本机无 docker；`make -C apps/control-plane migrate-validate` 需自建干净 PG16 并传 `DEV_URL`。改迁移后必须 `atlas migrate hash` |
| 门禁 | `verify:contracts` / `verify:control-plane` / `verify:web` / `verify:runtime-agent` |

**`verify:runtime-agent` 有既有并发 flake**：全量跑可能随机挂一条，单独跑该用例即通过。**不是你弄坏的**，本系列 runtime 目录零改动。

### 当前数据现状（2026-08-05 实测，批二 spec §10 已过时，以本节为准）

租户 `00000000-0000-0000-0000-000000000001`，团队「默认团队」`00000000-0000-0000-0000-000000000101`。

**角色词表 10 项**：developer / reviewer / tester / collector / analyst / diagnostician / operator / verifier / researcher / writer

**数字员工 5 个**（角色绑定已被 E2E 会话扩充，**与批二 §10.2 写的不一致**）：

| 名称 | id | 已绑角色 | 能力 |
|---|---|---|---|
| 开发-A | `0be393bb-9dfd-48c8-b010-4b5abb114f23` | developer, diagnostician | code_implementation |
| 审查-B | `7a16f593-9a99-490e-bcab-77bb8b326afa` | reviewer, verifier | code_review |
| 测试-C | `157b1a2c-b2af-4a08-99f3-f16abe291ed1` | tester | test_execution |
| 运维-D | `9a623b40-c9ec-4d7d-99a4-17b1f569b52e` | analyst, collector, diagnostician | log_analysis |
| 诊断-E | `3683f032-2e24-43da-af06-5af1b8ce71a4` | diagnostician, verifier | incident_triage |

**没有任何员工持有 `operator`** —— 这是批二 G2 的判别条件，**不要"顺手"给谁补上**。

**项目**：`批二基线项目 P1` = `ca82b054-de2d-4810-9a2b-dd41f5e50a2c`
**现有编制 6 条**：`software_delivery` 三角色齐全；`incident_response` 三角色齐全
**demands = 0，open inbox = 0**（历史 E2E 数据已硬删）

### ⚠ 一处已知的数据与产品不一致

`incident_response` 的 `operator` 位上编制的是**开发-A**，但开发-A **并不持有 `operator` 角色**。

原因：`PutCasting` 只校验 `role_key` 在词表里注册且 active，**不校验被编制的员工是否持有该角色**。UI 候选列表按角色过滤，但直接调 API 可以绕过。

对实施会话的影响：

- 批二 §10.3 的期望表（「故障排查只能走到仅诊断根因，缺 operator」）在**当前数据下不成立**，别照着它核对
- 若要复现那个条件，先删掉 `incident_response` 的 `operator` 编制行
- **是否把「编制时校验员工持有该角色」改成硬校验，是一个未决产品问题**（人类未拍板）。不要自行决定并顺手实现——发现它影响你的判据时，先报告

---

## 0. 为什么现在必须做

批二把角色做成了一等注册表，但**没有配套界面**。今天的实际状态：

| 能力 | 后端 | 界面 |
|---|---|---|
| 角色词表增删改查 | ✅ `/api/v1/role-vocabulary` | ❌ **完全没有** |
| 员工角色多值绑定 | ✅ `PUT /digital-employees/{id}/roles` | ❌ **完全没有** |
| 项目 × 剧本编制 | ✅ | ✅ `PlaybookCastingPanel` |
| 剧本查看/编辑 | ✅ | ⚠️ 裸 JSON textarea（`create-dialog.tsx`） |

后果是一条**断路**：

```text
词表里没有「网络诊断」这个角色
      → 人想注册一个
      → 无处可去（只能手写 SQL）
      → 注册不了角色，就没人能持有它
      → 编制/扩编的候选列表永远查不到人
```

批三的语义扩编会**主动把人推到这条断路上**：发现器返回 `external: true`，卡上说"需要一个词表外的角色"，人点「去注册角色」——如果没有落点，这个功能就是死的。所以本 spec 必须排在批三之前。

同时这也补上 `TODO.md` 2026-08-04 记的「剧本管理页面 UI 大改」的**必要部分**（完整的剧本可视化编辑仍不在本批，见 §2）。

---

## 1. 目标

1. 角色词表可被人管理：看、建、改名、停用，且**停用前能看见会砸到谁**。
2. 员工的角色绑定可被人编辑（多值），并说清它与旧 `role` 字段的区别。
3. 剧本的角色引用**可读**：从剧本能看到它需要哪些角色、每个角色由谁承担。

---

## 2. 非目标

| 不做 | 理由 |
|---|---|
| 剧本可视化编辑器（拖拽骨架、编排依赖） | 基线 §5 已否决；本批只做**只读**的剧本角色视图 |
| 剧本 spec 的表单化编辑（替代 JSON textarea） | 范围大且与本批正交，留在 `TODO.md` |
| 角色的权限语义（角色 ≠ 权限） | 角色是**编制单位**，权限走权限中心，两者不可混 |
| 角色删除 | 只做停用。角色被历史编制引用，删了会让审计断链（与批一"模板只 disable 不删"同理） |
| 跨租户角色共享 | 词表是租户级，与能力词表同构 |

---

## 3. 地基核对（2026-08-05 核实）

| 事实 | 锚点 |
|---|---|
| 角色词表 API 已全 | `internal/rolevocab`；Web 侧 `lib/api/casting.ts` 已有 `listRoleVocabulary` |
| 员工角色写入 API 已有 | `PUT /api/v1/digital-employees/{employeeId}/roles`（`server.go:388`） |
| 员工侧无角色 UI | `features/employees/config.tsx` 只有 `PermissionTierSection`（角色与权限），指的是**权限层**，与剧本角色无关 |
| 旧 `role` 列已降级 | 迁移注释：「显示用自述标签…禁止再用本列做匹配」。但**界面上它仍叫「角色」** |
| 剧本编辑是裸 JSON | `features/scenario-templates/create-dialog.tsx` 用 `<textarea>` 收 `spec（JSON）` |
| 扩编卡已能选角色 | `inbox-action-dialog.tsx` 在 `needs_external_role` 时拉词表让人挑；缺的是「词表里没有」的出口 |
| 编制面板已存在 | `PlaybookCastingPanel`，挂在提交需求对话框与项目配置页两处 |
| 候选按角色查 | `GET /projects/{id}/role-candidates?role_key=`，返回能力标注与团队分组 |

---

## 4. 页面一：角色词表管理

### 4.1 落点

平台治理区新增「角色词表」。**与「场景模板」并列**——它们是同一层概念（租户级注册表），且剧本引用角色，放一起人才能理解这层关系。

> 实施时先读 `DESIGN.md` 的页型与主从布局约定；列表用既有列表页型，不手写栅格。

### 4.2 列表

| 列 | 说明 |
|---|---|
| 角色 | 中文名 + `role_key`（等宽） |
| 说明 | description |
| 状态 | active / disabled（中文经 `status-labels.ts`） |
| **被引用** | **本批的关键列**，见 §4.3 |

### 4.3 引用计数（停用前必须看得见）

停用一个角色会同时砸到三处，界面必须在**停用前**说清楚：

| 引用方 | 影响 |
|---|---|
| 剧本 `roles[].key` | 该剧本将无法通过 `validateSpecRoles`，改版即被拒 |
| 员工角色绑定 | 这些员工不再作为该角色的候选出现 |
| 已有编制 | 编制行仍在（历史事实），但可达收口计算会认为该角色无人可用 |

停用确认弹窗要列出这三个数字与具体对象名（剧本名、员工名），**不是**一句"确定停用吗"。

**这条是本页最有价值的部分**：没有引用计数，人不敢停用任何角色，词表只会越积越脏——这正是批一在能力词表上遇到过的问题（E2E 残留键混在生产词表里）。

### 4.4 新建 / 编辑

- 字段：`role_key`（下划线小写，创建后不可改）、中文名、说明
- `role_key` 校验：与能力词表同规范；重复时给明确冲突提示
- 改名只改中文名与说明，不改 key——key 是被剧本与编制引用的稳定标识

---

## 5. 页面二：员工角色绑定

### 5.1 落点

员工配置页新增「剧本角色」分区，**与既有「角色与权限」分区区分开**——后者是权限层，同名不同义，必须在文案上分清，否则用户会以为改了权限。

### 5.2 交互

- 多选（一人可兼多角色，对应 `collapse_rules` 承认的兼任）
- 候选来自 active 角色词表
- 保存走 `PUT /digital-employees/{id}/roles`（整套替换）
- 旁边显示该员工已声明的**能力**（`external_capabilities`）作为参考——角色是编制单位，能力是佐证，人在绑角色时应该看得到

### 5.3 旧 `role` 字段的处置

界面上它现在也叫「角色」，和新分区并列会让人以为改它有用。改为：

- 标签改成**「显示标签」**
- 加一行说明：「仅用于列表展示，不参与剧本匹配与编制」

这条不改数据、不改后端，纯文案，但它消除的是一个**会持续误导人**的歧义。

---

## 6. 页面三：剧本的角色视图（只读）

在场景模板详情里增加只读的「角色与收口」区：

```text
软件开发 software_delivery

角色           建议能力                  本租户持有者
开发 developer   code_implementation      2 人
审查 reviewer    code_review              6 人
测试 tester      test_execution           2 人

收口档位（浅 → 深）
交付分支 branch_ref        需要：开发
审查通过并合入 review_verdict  需要：开发 + 审查（须不同人）
发布上线 release_record    需要：开发 + 审查 + 测试
```

**为什么值得做**：这是**唯一**能让人回答"这个剧本到底要什么人"的地方。今天要回答它，只能去读 JSON textarea 里的 spec。

收口档位与所需角色**必须复用** `PruneSkeletonForExit` / `ExitCondMet`（批二已提到 `scenariotemplate` 共用包），不得在前端另算——两套一定会漂。

---

## 7. 端点

角色词表 CRUD 已存在。本批需要新增/补齐两个**只读**端点：

```http
GET /api/v1/role-vocabulary/{roleKey}/references
    → { scenario_templates: [{key, name}], employee_count: N, casting_count: N }

GET /api/v1/scenario-templates/{templateKey}/role-view
    → { roles: [{role_key, title, required_capabilities, holder_count}],
        exits:  [{deliverable, label, required_roles, role_independence_pairs}] }
```

第二个端点的 `exits[].required_roles` **在服务端算**（复用共用包），前端只渲染。

---

## 8. 分期

| 切片 | 内容 | 可独立验收 |
|---|---|---|
| **P0a** | 角色词表页：列表 + 新建 + 改名 + 停用（含引用计数弹窗）+ `references` 端点 | 浏览器：建一个角色 → 绑给员工 → 停用时看见引用计数 |
| **P0b** | 员工「剧本角色」分区 + 旧 `role` 改名为「显示标签」 | 浏览器：给员工绑两个角色，编制候选里能查到 |
| **P0c** | 剧本角色视图（只读）+ `role-view` 端点 | 浏览器：软件开发剧本显示三档收口与所需角色 |

P0a + P0b 是**批三的最小前置**（缺了它们，批三的「去注册角色」与"注册后有人持有"都不成立）；P0c 可后置。

---

## 9. 验收 GATE（真实 E2E）

| ID | 步骤 | 期望 |
|---|---|---|
| R1 | 在角色词表页新建 `network_diagnostics` | 列表出现，`role_key` 校验生效（大写/空格被拒并提示） |
| R2 | 把新角色绑给一个员工 | 员工配置页「剧本角色」多选保存成功 |
| R3 | 在项目里为某剧本编制该角色 | `role-candidates` 能查到该员工 |
| R4 | 停用一个**被剧本引用**的角色 | 弹窗列出引用它的剧本名、持有员工数、编制数；确认后该角色不再出现在编制候选 |
| R5 | 停用后改版引用它的剧本 | `validateSpecRoles` 拒绝并点名该角色（后端既有行为，本批只验证界面能看懂） |
| R6 | 员工配置页 | 旧字段显示为「显示标签」并有说明；与「剧本角色」分区不混淆 |
| R7 | 剧本角色视图 | 软件开发显示三档收口与各档所需角色；「审查通过并合入」标注须不同人 |
| R8 | 从扩编卡的「去注册角色」深链 | 直达角色词表页（批三 H9b 的前置） |
| R9 | `verify:web` + `verify:contracts` + `verify:control-plane` | 全过 |

**完成定义**：R1–R6 + R9 全过（P0c 交付时补 R7）；R8 待批三一并验。

---

## 10. 风险

| 风险 | 缓解 |
|---|---|
| 「剧本角色」与「角色与权限」被混淆 | §5.1 文案强制区分；R6 |
| 停用角色造成静默破坏 | §4.3 引用计数 + 停用确认列出具体对象；R4 |
| 前端另算收口档位导致与规划期漂移 | §6 强制服务端算、复用共用包 |
| 角色 key 命名走样 | 与能力词表同规范（下划线小写），创建后不可改 |
| 范围膨胀成剧本编辑器 | §2 非目标已划线：本批剧本侧**只读** |

---

## 11. 一句话方案

> **把批二建好的角色注册表和员工角色绑定接上界面：词表可管且停用前看得见会砸到谁，员工可绑多角色且与权限分区分清，剧本能只读地回答"这个剧本要什么人、走多深需要谁"。**
