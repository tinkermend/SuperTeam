# 2026-08-14 技能包替换与只读预览

> 状态：已拍板，待实现（2026-08-14 已过一轮对代码的审查，见 §15 现状核实台账）  
> 范围：Console + Control Plane；Runtime 沿用现有 checksum 收敛，本切片不改 Provider 方言  
> 本轮审查新增的人类决策：路径穿越改 400 拒绝（§6.5，对现有上传是行为变更）；替换用新 action `skill.archive.replace` 而非放宽 `skill.upload`（§6.2）

## 1. 背景

技能已是 zip 整包制品：上传写入对象存储，库表只存归档元数据（`archive_object_ref` / checksum / 文件数），Runtime 按 `archive_checksum_sha256` 在下次派发时物化。`skill_files` 与在线编辑已在 `025_skill_archive_storage` 删除，不得恢复。

产品缺口不是「控制台当 IDE」，而是：

- 列表只有「新建上传 / 删除」。同一技能要换包，人只能删绑定再传，或误走「再上传一次」指望 slug 撞上现有 `ON CONFLICT`。
- **「再上传一次」不是无害 workaround，它今天就在静默清空团队绑定。** `PgRepository.UpsertSkillPackage`（`pg_repository.go:141`）在同一事务里 `DELETE FROM team_skill_bindings WHERE tenant_id=$1 AND skill_id=$2`，再按请求里的 `TeamIDs` 重插。上传表单不带 `team_ids` 时，撞 slug 的那次「更新」把这条技能的团队绑定全删了，界面无任何提示。员工绑定（`skill_agent_bindings`）与项目绑定（`project_skill_bindings`）不受影响——只死团队绑定，所以极难被发现。这是 §6.1 收紧新建路径的首要动因，「看不出换过包」只是次要动因。
- 详情只有归档元数据，看不到包内目录与 `SKILL.md`。
- 每次 `UploadSkill` 把 `version` 写死为 `v0.1.0`（`service.go:290`），覆盖后界面上看不出换过包。

拍板：**本地编辑，平台只提供「更新 zip」和「只读摊开预览」。** 不做 `SKILL.md` 在线保存。

## 2. 目标

- 在既有技能卡片和详情上，对**这一条 `skillId`** 上传新 zip，解析后替换归档，团队 / 员工 / 项目绑定全部保留。**承重**：替换路径不得复用 `UpsertSkillPackage`（它会清空团队绑定，见 §1、§6.2）。
- 详情可浏览包内树；文本文件（含 `SKILL.md`）只读预览；二进制只展示类型与大小。
- 替换成功后提示版本变化，并写审计（谁、何时、旧/新 checksum 与 version）。
- 新建上传与「更新这一条」语义分开：新建不得再靠 slug 静默覆盖。

## 3. 非目标

- 控制台编辑、保存、写回 zip 或重建 `skill_files`。
- 版本历史表、回滚 UI、对比 diff（对象键已含 checksum，旧 blob 刻意残留）。**注意代价**：不做历史表 = 旧归档对象没有任何指针可追，删技能时清不掉，孤儿会线性累积；见 §10。
- 替换后立刻向 Runtime 推送；仍走下次任务派发的 skill convergence。
- 自动改 `skill_mcp_dependencies`（MCP 绑定仍走详情现有接口）。
- 技能市场、安装、卸载流程改造。
- 在浏览器或 Control Plane 执行包内脚本。

## 4. 身份与权威来源

| 概念 | 谁说了算 |
| --- | --- |
| 技能实体 | `skills.id`，更新钉死它 |
| Runtime 目录名 | `slug`，**替换时禁止改变** |
| 包内容 | 用户本地目录 → zip；对象存储整包为运行时权威 |
| 展示名 / 描述 / 版本 | 新包 `SKILL.md` frontmatter；表单可覆盖空缺 |
| 是否换包 | `archive_checksum_sha256` |

对象键保持 `skills/{tenant_id}/{slug}/{checksum}.zip`。指针前移后旧对象不删（本切片无 GC）——这是刻意的：正在跑的会话可能仍在按旧 presign URL 取包，删旧对象会打断它们。代价见 §10（`DeleteSkill` 只删当前指针那一个对象，历史版本会变成永久孤儿）。

## 4.1 预览怎么读对象存储（不解到磁盘）

zip **仍只作为整包**躺在对象存储。Console 不直连桶。Control Plane：

1. 按 `archive_object_ref` `GetObject` 拿到 zip 字节（与上传校验同一套 `archive/zip` Reader）。
2. **列目录**：读 zip central directory，返回 path/size/是否可预览；**不把整包解成文件系统**。
3. **看某个文件**：`zip.File.Open()` 只解那一个 entry，受 `skill.archive_preview_max_bytes` 截断。

这和 Runtime 物化不同：Runtime 才是「下载整包 → 校验 checksum → 解到员工家目录」。预览是管理面只读查看，CP 进程内存完成，无工作区、无 `skill_files` 回写。

**接口缺口（实现前必读）**：`skill.ObjectStore` 接口（`service.go:49-55`）只有 `PutObject` / `DeleteObject` / `PresignGet`，**没有 `GetObject`**。`storage.S3ObjectStore.GetObject(ctx, key)` 已存在（`storage.go:289`），但返回 `*storage.Object{Body io.ReadCloser}`，而 `zip.NewReader` 要的是 `io.ReaderAt` + size——必须先把整包读进内存（或落临时文件再 `os.File` 当 ReaderAt）。本期取内存路径，接口按最小面加一个 `GetObject(ctx, key) (io.ReadCloser, error)`。

**性能与内存放大面**：`skill.upload_max_bytes` 的 `MaxValue` 是 200MiB（`systemconfig/registry.go:117-125`），即单次预览最坏 200MiB 常驻 CP 堆；开一次树 + 点五个文件 = 六次全量拉取 + 六份拷贝。管理面只读功能不该给 CP 开这么大的放大面，故本期加一道**预览体积闸**：

- 新配置 `skill.archive_preview_max_archive_bytes`（默认 8MiB，min 1MiB，max 64MiB）。归档字节数超过它时，`entries` 与 `content` 一律返回 **413**，文案「技能包过大，平台不提供在线预览，请在本地解压查看」。这不影响上传、绑定、派发与 Runtime 物化——预览是纯管理面便利功能，超限降级是可接受的。
- 该闸只看 `skills.archive_size_bytes`（库里现成），**在 `GetObject` 之前判**，超限连字节都不拉。

关于按需读取：S3 **支持 Range GET**，基于 ranged GET 实现 `io.ReaderAt` 即可只读 central directory + 目标 entry，这是这类预览的标准做法。本期不做，理由是实现成本（要自己管 range 窗口与重试），不是「S3 不支持」——先前版本此处的判断有误，勿据以反推。若后续技能包普遍变大，优先级是「ranged ReaderAt」而非「整包缓存」。本期同样不做按 checksum 的服务端缓存；无论如何都不要把文件内容落库。

## 5. 产品流

### 5.1 更新技能包

入口：技能市场卡片「更新」；详情页主操作「更新技能包」。

对话框：

1. 展示当前名称、slug、version、checksum 短码、绑定数量（团队 + 员工 + 项目）。**不要为此新造 count 端点**：`GetSkill` 返回的 `Skill` 已带 `TeamBindings` / `AgentBindings` / `ProjectBindings` 三个数组（`skill/types.go:38-40`），详情页取现成的长度即可。
2. 选择 zip（校验规则与新建上传相同：`skill.upload_max_bytes` 体积上限、必须含 `SKILL.md`、路径穿越拒绝、§4.1 预览体积闸只影响预览不影响上传）。
3. 明确文案：绑定保留；下次任务才会按新 checksum 物化；不会改 slug。
4. 提交成功：toast「已更新到 {newVersion}」（若 version 未变： 「已替换归档，checksum {short}」），关闭对话框，刷新卡片与详情。

> **勿沿用旧表述**：本设计早期稿写过「校验规则与新建上传相同：解包文件数/字节上限」。核实后：CP 上传路径**只**校验 `skill.upload_max_bytes`（`handler.go:115-132`，默认 50MiB）。`skill.archive_unpack_max_bytes` / `skill.archive_unpack_max_file_count` 是**平台限额、经心跳下发给 Runtime**（`app.go:518-519` → `runtimepkg.PlatformLimits`），CP 侧一次都没拿它们校验过上传。见 §6.3。

权限：新增 `skill.archive.replace`（见 §6.2），资源为该 `skill` 而非租户级「随便造一条」。

### 5.2 只读预览

详情新增「包内容」：左树右预览。

- 树：规范化相对路径，忽略 `__MACOSX` / `.DS_Store` / `._*`，与上传解析一致。
- 选中文本：展示内容（UTF-8）；`SKILL.md` 默认选中。
- 选中二进制：类型、大小、不可预览。
- 无保存、无 Monaco 写回、无拖拽改结构。

## 6. API

### 6.1 收紧新建上传

`POST /api/v1/skills/uploads`：

- 仍解析 zip 并推导 slug。
- **冲突检查必须前移到 `PutObject` 之前**：解析出 slug 后立刻查 `(tenant_id, slug)` 是否已有未删除行；命中就直接 **409**，**一个字节都不写对象存储**。否则每次误操作都会往桶里丢一个无人指向的孤儿 blob（本切片无 GC，见 §4）。
- 409 body 带已有 `skill_id` / `slug` / `name`，提示走「更新技能包」。
- 删除 `ON CONFLICT DO UPDATE` 作为新建路径的副作用。唯一索引是 `(tenant_id, slug) WHERE deleted_at IS NULL`，所以软删技能占用的 slug 仍可被新建复用——这是既有语义，不改。
- **路径穿越改为拒绝（行为变更，已拍板）**：见 §6.5。

### 6.2 替换归档（新）

`POST /api/v1/skills/{skillId}/archive`

- multipart 与 uploads 相同（`file` 必填；`name` / `description` / `risk_level` / `tags` / `runtime_dependencies` 可选）。
- 404：技能不存在或已删。
- 400：zip 非法、无 `SKILL.md`、超 `skill.upload_max_bytes`、路径不安全（§6.5）。
- **slug 冲突**：从新包推导的 slug ≠ 当前技能 slug → 400，说明「更新不得改 slug；若这是另一个技能请走新建」。不根据新包 name 改 slug。**该检查同样前移到 `PutObject` 之前**（同 §6.1 的孤儿 blob 理由）。
- 成功 200：返回完整 `Skill`（新 version、checksum、filename、updated_at）。绑定数组与替换前一致。
- 元数据：`name`/`description` 优先表单，否则 frontmatter / 正文启发式（与现 Upload 一致）。`version` 见 §7。
- `risk_level` / `tags` / `runtime_dependencies`：请求有则更新，无则保留。
- 不改 `created_by`、不碰 binding 表。

**【承重·实现陷阱】不得复用 `UpsertSkillPackage`。** 该仓储方法在同一事务里 `DELETE FROM team_skill_bindings WHERE tenant_id=$1 AND skill_id=$2` 再按 `req.TeamIDs` 重插（`pg_repository.go:141-151`）。替换路径复用它 = 本设计的头号承诺当场破功，且只死团队绑定（`skill_agent_bindings` / `project_skill_bindings` 不受影响），线上极难察觉。

替换走**新的仓储方法** `ReplaceSkillArchive(ctx, ReplaceSkillArchiveRequest)`：

- 单条 `UPDATE skills SET archive_object_ref, archive_filename, archive_size_bytes, archive_checksum_sha256, archive_file_count, version, name, description, [risk_level, tags, metadata], updated_at = NOW() WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`；
- 同一条语句里拿到旧 version / 旧 checksum，供 §8 审计与 UI 提示，避免多一次 SELECT 的竞态（PG 16 没有 `RETURNING OLD.*`，具体写法见 §8 的 SQL）；
- 影响行数为 0 → `ErrNotFound`；
- **一行不碰任何 binding 表**，不碰 `slug`、`created_by`、`source`、`icon_key`、`color_token`。

授权：**新增 `ActionSkillArchiveReplace`（`skill.archive.replace`）+ `ResourceSkill{id}`**。

> 不能沿用 `skill.upload`：`authz/authorizer.go:160-165` 的 `ActionSkillUpload` 分支硬要求 `resourceMatchesUUID(req.Resource, ResourceTenant, req.TenantID)`，传 `ResourceSkill` 必然 `deny(ReasonInvalidResource)` → 403。新增独立 action 让「能新建」与「能替换任意技能的包」可分别授权，也不放松租户级 upload 的判别。

**落点只有两处**（已勘察 OpenFGA 侧，见下）：

1. `authz/types.go`：加常量 `ActionSkillArchiveReplace = "skill.archive.replace"`（挨着 `types.go:67-70` 那四个 skill action）。
2. `authz/authorizer.go`：switch 里按 `ActionSkillDelete` 同形判别——`validUUIDResource(req.Resource, ResourceSkill)` + `checkTenantAdminAccess`。最省的写法是直接并进 `ActionSkillDelete, ActionSkillInstall` 那个 case（`authorizer.go:166-172`）。

**不要动 `openfga_mapping.go`——动了反而会坏。** 理由：

- FGA model（`authz/openfga/model.fga`）只有 `user` / `tenant` / `team` / `project` 四个类型，**没有 `skill` 类型**。
- `openFGARelationForAction`（`openfga_mapping.go:79-95`）里，四个 skill action 只有 `ActionSkillInstall` 被映射；`skill.read` / `skill.upload` / `skill.delete` **都不在映射表里**，走的是「不映射」路径。`ActionSkillInstall` 之所以能用，是因为 `openFGAObjectForRequest`（`:126-136`）给它开了特例，把对象从 `skill:{id}` **改写成 `tenant:{tenantID}`**。
- 所以若把新 action 加进 `openFGARelationForAction` 却不加那个对象改写特例，FGA Check 会拿 `skill:{uuid}` 去查一个模型里不存在的类型 → 校验报错。shadow 模式下这个 error 会被记成 `diff: true`（`shadow_authorizer.go:84-87`），凭空造出一堆假背离告警。
- 不加映射是**安全**的：`OpenFGACheckForRequest` 返回 `!ok` 时，`ShadowAuthorizer.Check`（`:55-58`）直接返回主(DB)决策，不查、不记、不算背离。这与 `skill.upload` / `skill.delete` 今天的行为完全一致。

**`authzcenter` 也不用改**：`authzcenter/generated.go:40-60` 的 `CheckPermissionRequestAction` 是从 OpenAPI 生成的封闭枚举，里面**一个 skill action 都没有**（只有 authz_center / console / runtime_scope / task / team 五族）。加 `skill.archive.replace` 反而会让它与既有四个 skill action 不一致。要补就五个一起补，不属本期。

**已知既有债（不是本期造成的，也不在本期修）**：`AUTHZ_ENGINE=openfga` 纯执行模式下，未映射 action 会命中 `ReasonUnsupportedAction` 直接**拒绝**（`openfga_authorizer.go:93-100`）。也就是说今天切到纯 OpenFGA，`skill.read` / `skill.upload` / `skill.delete` 全部 403，技能管理面整体不可用。默认引擎是 `db`（`config.go:228`），rollout 脚本用的是 `openfga_shadow`（`scripts/openfga-bootstrap.sh:66`），所以现实中没暴露。新 action 与这三个同命运，不新增退化。

> 已登记到 **`docs/OPENFGA_MIGRATION_DEBT.md` D4**（skill 族靠对象改写伪装）与 **D1**（action 覆盖率 25/80）。真正切 OpenFGA 时按那份台账处理，不要在本 spec 的实现里顺手补映射。

### 6.3 包树（新）

`GET /api/v1/skills/{skillId}/archive/entries`

授权：`skill.read` + `ResourceSkill{id}`（`ActionSkillRead` 已同时接受 `ResourceTenant` 与 `ResourceSkill`，见 `authorizer.go:150-159`，**无需改 authz**）。

Control Plane 从对象存储拉 zip，在内存列出条目，**不落库**。先过 §4.1 的预览体积闸（超限 413，不拉字节）。

每项：

```yaml
path: SKILL.md          # 规范化相对路径，按 §6.5 去重
kind: file              # file | directory
size_bytes: 1234        # 解压后字节数（zip.File.UncompressedSize64）
previewable: true       # 仅表示"是文本"。超 preview_max_bytes 的文本仍是 true，
                        # 取内容时截断（§6.4），不因大小变成不可预览
content_type: text/markdown
```

**条目数上限**：`skill.archive_unpack_max_file_count`（默认 10000）。

> 口径澄清（早期稿此处有误）：`skill.archive_unpack_max_bytes` / `..._max_file_count` 今天**只是下发给 Runtime 的平台限额**（`app.go:518-519` → `runtimepkg.PlatformLimits` → 心跳），CP 侧从未用它们校验过任何东西。本期是**首次在 CP 侧启用**，且只用 `max_file_count` 给 entries 兜底；`max_bytes`（默认 200MiB）是给 Runtime 解包定的，尺度太大，管理面改用 §4.1 的 `skill.archive_preview_max_archive_bytes`。

### 6.4 文件预览（新）

`GET /api/v1/skills/{skillId}/archive/content?path=SKILL.md`

授权同 §6.3。

- `path` 必须命中 §6.3 entries 返回的条目白名单；不在白名单（含 `..`、绝对路径、被忽略的条目）→ **404**。不要在这里重新实现路径校验——**以 entries 的结果为唯一判据**。
- 成功一律返回 **JSON**：`{ path, content, truncated, size_bytes, content_type }`，`Content-Type: application/json`。

  > 早期稿写的是「`text/plain; charset=utf-8` 或 JSON」，二选一没定。定为 JSON：`truncated` 这个信号在 `text/plain` 下无处安放，而截断是必须让人看见的行为。契约里只留这一种。
- 二进制（entries 里 `previewable: false`）→ **415**，不返回字节；单文件超 `skill.archive_preview_max_bytes` → 200 + 内容截断至上限 + `truncated: true`（**不是** 413，前端在正文上方显示「已截断，完整内容请在本地查看」）；整包超 `skill.archive_preview_max_archive_bytes` → **413**（§4.1）。三者别混：415 = 类型不支持，413 = 整包太大连树都不给，截断 = 正常成功但内容不全。
- 预览上限新配置 `skill.archive_preview_max_bytes`（默认 256KiB），与解包总上限分开。

不要把整包预签 URL 暴露给 Console 当「文件浏览器」；预览走 Control Plane，避免把对象存储凭证语义泄漏到浏览器。

### 6.5 路径穿越：拒绝（行为变更）

**已拍板：上传与替换都改成整包 400 拒绝。**

现状是静默跳过：`normalizeFilePath`（`service.go:759-765`）对 `../` 开头与绝对路径返回空串，`extractSkillMarkdown` 只是跳过该条目——**而且仍把它计进 `fileCount`**（`service.go:637-640`，`fileCount++` 在归一化之前），导致 `archive_file_count` 与实际可用文件数对不上。

改为：`POST /api/v1/skills/uploads` 与 `POST /api/v1/skills/{skillId}/archive` 在解析阶段一旦发现任一条目归一化为空串（`../`、绝对路径、盘符路径），立即 400，错误里带上第一个违规条目的原始路径。

- 这是对**现有** `/skills/uploads` 的行为变更：存量里若有带脏条目的包，再传会被拒。可接受——这类包的 `archive_file_count` 本来就是错的，而且 Runtime 侧解包也不会落这些条目。
- `__MACOSX` / `.DS_Store` / `._*` 仍走**忽略**语义（`isIgnoredArchiveEntry`），不算穿越，不触发 400。两套语义要分清：忽略 = 打包噪音；拒绝 = 不安全路径。
- 顺带修正 `fileCount` 口径：只统计归一化后非空、且未被忽略的条目。

**归一化撞名**：两个不同 zip 条目归一化后可能得到同一个 `path`（如 `SKILL.md` 与 `./SKILL.md`）。`entries` 按 path 去重，**保留 zip 中最先出现的那一个**并在契约里写死；`content?path=` 取的也是它。撞名不算错误，不返 400。

## 7. 版本

从 `SKILL.md` YAML frontmatter 读 `version`，与现有 `name`/`description` 同一次解析——`parseSkillMarkdown`（`service.go:717-720`）里那个匿名 struct 加一个字段即可：

```go
var meta struct {
    Name        string `yaml:"name"`
    Description string `yaml:"description"`
    Version     string `yaml:"version"` // 新增
}
```

> 已实测确认无隐患：`gopkg.in/yaml.v3` 把 `version: 1.0` 这类裸浮点标量解进 `string` 字段**不报错**（解得 `"1.0"`），`0.1.0` / `v1` 同样正常。所以加字段不会让 `yaml.Unmarshal` 失败、进而连带毁掉 `name`/`description` 的提取（`service.go:721` 那里是 `err == nil` 才赋值）。

| 情况 | 行为 |
| --- | --- |
| frontmatter 有 version | 写入 `skills.version` |
| 表单显式传 version | 覆盖 frontmatter |
| 都没有 | **保留**替换前 version，不以 `v0.1.0` 覆盖 |
| 新建上传且都没有 | 默认 `v0.1.0`（仅创建） |

不做强制 semver。UI 用新旧 version + checksum 短码提示。

新建上传同样改为：能解析到 version 就用，禁止无脑写死 `v0.1.0`（仅缺省时）。

**长度校验（必须做）**：`skills.version` 是 `VARCHAR(80)`（`009_skill_management.sql:7`）。frontmatter 或表单里塞一个超长 version，不校验就会在 INSERT/UPDATE 时炸成 Postgres `22001 string_data_right_truncation` → 500。规则：`TrimSpace` 后为空按上表兜底；**长度 > 80 → 400**（不截断，因为版本号被悄悄截断比报错更坏）。`name` / `description` 同理需核对各自列宽后再定，本期至少把 `version` 补上。

## 8. 审计

替换成功写一条审计，不单独做日志中心 UI（详情可后续挂审计流）。

建议字段：

- `event_type`: 与现有 skill/capability 事件同类（实现时对齐 `oplog` 既有分类）
- `resource_type`: `skill`
- `resource_id`: skill id
- `action`: `skill.archive.replace`
- `details`: `slug`, `name`, `old_version`, `new_version`, `old_checksum`, `new_checksum`, `archive_filename`, `archive_size_bytes`, `binding_team_count`, `binding_employee_count`, `binding_project_count`

`old_version` / `old_checksum` 必须与 UPDATE 同一条语句拿到，**不要**在 UPDATE 之前单独 SELECT 一次——那样两个并发替换会互相写错对方的审计旧值。本项目是 **Postgres 16**（`docker-compose.dev.yml:5`），`RETURNING OLD.*` 要 PG 18 才有，用自连接 + 行锁：

```sql
UPDATE skills AS s
SET archive_object_ref = $3, archive_filename = $4, archive_size_bytes = $5,
    archive_checksum_sha256 = $6, archive_file_count = $7,
    version = $8, name = $9, description = $10, updated_at = NOW()
FROM (
    SELECT id, version, archive_checksum_sha256
    FROM skills
    WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL
    FOR UPDATE
) AS prev
WHERE s.id = prev.id
RETURNING prev.version AS old_version, prev.archive_checksum_sha256 AS old_checksum,
          s.version AS new_version, s.archive_checksum_sha256 AS new_checksum;
```

`FOR UPDATE` 顺带把并发替换串行化。影响行数为 0 → `ErrNotFound`（§6.2）。上面只列了必改列，`risk_level` / `tags` / `metadata` 按 §6.2 的「有则更新、无则保留」条件拼接。

失败（校验失败）不写成功审计；保持现有 API 错误即可。

## 9. Runtime：用 checksum 当内容版本，不是 `skills.version`

员工工作目录里技能是否「已经是最新」，**不看**库表 `skills.version`（那是给人看的展示字符串），也不看 `skills.id`。

现有链路：

1. 派发 payload 里每个技能带 `archive_object_ref` + `archive_checksum_sha256`。`revision_id` **已经等于 checksum**（`runtimeSkillsPayload` 里 `RevisionID: s.ArchiveChecksum`），不是单独 UUID。
2. 家目录 stamp 的指纹是 `skill_key:checksum` 拼接的 sha256；`.skill-checksum` marker 也存 checksum。
3. 两者与 payload 一致 → **不下载**；不一致 → presign GET 新 zip，解到 `provider_skills_root/<slug>/`，覆盖目录并写新 marker。

因此替换归档时必须改 DB 里的 `archive_checksum_sha256` 和 `archive_object_ref`（新对象键 `…/{newChecksum}.zip`）。下次派发指纹必变，会重拉。若只改 `version` 文案、checksum 不变，Runtime **不会**重拉——这是正确行为。

不产生新的 `skill_id`。不新增 revision 表。正在跑的会话不打断；已物化目录要到**下一次任务派发**才收敛。

`capability_manifest_version`（CP 侧 `cmv1:sha256:`）同样含各技能 checksum，替换后下次派发会变；Runtime 不拿它做跳过判定，只做 attestation 留痕。

## 10. 数据模型

本切片**无新表、无恢复 `skill_files`**、**无迁移**。`skills` 行原地更新归档列与 `version`/`name`/`description`/`updated_at`。

新增的两个 systemconfig key（`skill.archive_preview_max_bytes`、`skill.archive_preview_max_archive_bytes`）走注册表登记，不落迁移。

**已知代价：孤儿 blob 会随替换次数线性累积。** `DeleteSkill`（`service.go:328-333`）只删**当前** `archive_object_ref` 指向的那一个对象。替换 N 次后再删技能，桶里留下 N-1 个永久孤儿，且库里已无任何指针可追——连事后写清理脚本都没有依据。本期接受（§4 已说明保留旧对象是为了不打断在跑的会话），但这是 `skill_archive_revisions` 的**主要动机**，不只是「做回滚」：

可选后续（不进本期）：`skill_archive_revisions` 记历史指针 →（a）回滚 UI；（b）**让 GC 有据可依**（删技能时按历史表逐条清对象）。两者之中 (b) 更急。

## 11. Console

- `apps/web/src/features/skills/index.tsx`：卡片增加「更新」，打开替换对话框（勿跳到 `/skills/upload`）。
- **同时改删除区文案**：`index.tsx:553` 现在写着「删除会同时解除全部绑定并清除归档文件」。加了「更新」之后，这句必须改成只描述删除（例如「删除会解除全部绑定并清除当前归档文件；如果只是想换包，请用「更新」」），否则两个动作的后果描述会打架，人会以为更新也解绑。
- `detail.tsx`：主操作「更新技能包」；「包内容」树 + 预览。
- 绑定数量直接取 `GetSkill` 已返回的 `team_bindings` / `agent_bindings` / `project_bindings` 长度，勿新造 count 端点。
- 新建页文案：若 slug 已存在，展示 409 并链到该技能详情。
- 用户可见中文走 `status-labels` / 本模块文案，错误不要直接甩英文 `upload skill request failed`。新增错误态至少要有：409 撞 slug、400 slug 变了、400 路径穿越、413 包太大不能预览、415 二进制。
- 交互与布局遵循 `DESIGN.md`（SoftCard / SoftDialog / 详情主从，不新开视觉体系）。

## 12. 安全

- zip 路径穿越、symlink、绝对路径：**上传/替换阶段直接 400 拒绝**（§6.5，行为变更）；预览 `path` 必须落在 entries 白名单内，不在白名单一律 404。
- 条目数上限：`skill.archive_unpack_max_file_count`（本期首次在 CP 侧启用，见 §6.3）。上传体积上限：`skill.upload_max_bytes`（既有）。
- 预览体积闸：`skill.archive_preview_max_archive_bytes`，在拉字节之前判（§4.1），避免管理面成为 CP 的内存放大器。
- 预览不执行、不解压到磁盘工作区（Control Plane 内存读指定条目）。
- 二进制不回传内容。

## 13. 实现顺序

1. **契约**：OpenAPI 增加 `POST /skills/{skillId}/archive`、`GET /skills/{skillId}/archive/entries`、`GET /skills/{skillId}/archive/content`；`/skills/uploads` 补 409。改完走生成与 `verify:contracts`。
2. **Authz**（已勘察，改动面比预想小）：只动 `authz/types.go` 加常量 + `authz/authorizer.go` 并进 `ActionSkillDelete, ActionSkillInstall` 那个 case。**明确不动** `openfga_mapping.go` 与 `authzcenter`——理由与「动了会造假背离告警」的机制见 §6.2。**这一步不做，§6.2 必 403。**
3. **仓储**：新增 `ReplaceSkillArchive`（只 UPDATE `skills`，一行不碰 binding 表），**不动** `UpsertSkillPackage`；新建路径把 slug 冲突检查前移到 `PutObject` 之前并去掉 `ON CONFLICT DO UPDATE`。
4. **对象存储**：`skill.ObjectStore` 接口加 `GetObject`；新增两个 preview systemconfig key。
5. **Control Plane**：拆「创建」与「按 id 替换」；frontmatter `version` + 长度校验；路径穿越 400 与 `fileCount` 口径修正；entries / content 两个只读端点；审计。
6. **Web**：更新对话框 + 详情预览 + 删除区文案修正。
7. **单测**：slug 不变、**三张 binding 表行数各自不变**、同 checksum 仍成功、无 `SKILL.md` 400、路径穿越 400、新建撞 slug 409 且**未写对象存储**、version 超 80 字符 400、content 请求白名单外路径 404。
8. **真链**：对本 checkout 已有技能点更新，确认绑定还在、详情树能打开 `SKILL.md`、下次派发 checksum 变化后 convergence 拉新包。

## 14. 验收

- 更新后 `skill_id` 不变，checksum 变；**绑定行数分表断言**：`team_skill_bindings`、`skill_agent_bindings`、`project_skill_bindings` 三张表针对该 `skill_id` 的行数各自与替换前一致（**只断言总数会漏掉「团队绑定被清空」这个最可能的实现错误**，见 §6.2）。
- 用改了 `name` 导致推导 slug 变化的 zip 更新 → 400，库中 slug 不变。
- 同 slug 新建 → 409，且**桶里没有新增对象**、原技能的 `archive_checksum_sha256` 未变（验 §6.1 的冲突前移）。
- 替换后**旧 checksum 对应的对象仍存在于桶中**（验 §4「不删旧对象」，也是在跑的会话不被打断的前提）。
- 带 `../` 条目的 zip → 上传与替换都 400，库中该技能归档列未变。
- 只改 `version`、checksum 不变的替换 → 下次派发 Runtime **不重拉**（stamp/marker 双命中，§9）；checksum 变的替换 → 下次派发重拉并写新 `.skill-checksum`。
- 详情能看到 `SKILL.md` 正文与脚本路径；脚本不可在 UI 执行。
- 超过预览体积闸的技能包 → entries 返 413 且详情页给出中文降级提示，不是白屏或英文报错。
- 无「保存 SKILL.md」按钮。
- 审计可查 `skill.archive.replace`，`details` 里 old/new checksum 与 version 齐全。
- 声称 E2E 时复核 `control-plane` / `web` 的 pid 与 `owner=` 为本 checkout。

## 15. 现状核实台账（2026-08-14 审查，按代码逐条验过）

实现时不要重新推导这些事实，也不要据「现状」反推规范。

| 断言 | 结论 | 锚点 |
| --- | --- | --- |
| `skill_files` 已删、不得恢复 | 属实 | `025_skill_archive_storage.sql:34` `DROP TABLE IF EXISTS skill_files` |
| `UploadSkill` 写死 `v0.1.0` | 属实 | `skill/service.go:290` |
| 新建走 `ON CONFLICT (tenant_id, slug) DO UPDATE` | 属实 | `skill/pg_repository.go:116` |
| 撞 slug 的「再上传」会清空团队绑定 | **属实，且是本期首要动因** | `skill/pg_repository.go:141`；员工/项目绑定不受影响 |
| `revision_id` 已等于 checksum | 属实 | `employee/service.go:1283` `RevisionID: s.ArchiveChecksum` |
| 家目录指纹 = 排序后 `key:checksum` 拼接取 sha256，**不含 version** | 属实 | `runtime-agent/src/skills_convergence.rs:86-105` |
| `.skill-checksum` marker 存 checksum | 属实 | `runtime-agent/src/skills.rs:99-105` |
| `cmv1` 只收 `skill_key` + `archive_checksum_sha256` | 属实 | `employee/capability_fingerprint.go:16-19` |
| CP 上传校验解包文件数/字节上限 | **不属实**，只校验 `skill.upload_max_bytes` | `skill/handler.go:115-132`；unpack 两个 key 只下发 Runtime（`app/app.go:518-519`） |
| `skill.upload` 可配 `ResourceSkill` | **不属实**，硬要求 `ResourceTenant` 否则 403 | `authz/authorizer.go:160-165` |
| `skill.read` 可配 `ResourceSkill` | 属实（双形态） | `authz/authorizer.go:150-159` |
| FGA model 里有 `skill` 类型 | **不属实**，只有 user/tenant/team/project | `authz/openfga/model.fga` |
| skill 族 action 已接进 OpenFGA | **不属实**，只有 `skill.install` 映射，且靠对象改写成 `tenant:{id}` | `openfga_mapping.go:79-95`、`:126-136` |
| 新 action 需同步 `openfga_mapping.go` | **不属实，且同步了会坏**（造假背离告警） | 机制见 §6.2；`shadow_authorizer.go:55-58`、`:84-87` |
| `authzcenter` 有 skill action 词表要补 | **不属实**，封闭枚举里一个 skill action 都没有 | `authzcenter/generated.go:40-60` |
| `AUTHZ_ENGINE=openfga` 下 skill 管理面可用 | **不属实**，未映射 action 一律拒绝（既有债，默认引擎是 `db` 故未暴露） | `openfga_authorizer.go:93-100`、`config.go:228` |
| `skill.ObjectStore` 有 `GetObject` | **不属实**，接口只有 Put/Delete/PresignGet | `skill/service.go:49-55`；底层 `storage.go:289` 有 |
| S3 不能按 range 读 zip entry | **不属实**，S3 支持 Range GET | 本期不做仅因实现成本，见 §4.1 |
| 路径穿越条目会被拒 | **不属实**，静默跳过且仍计入 `fileCount` | `skill/service.go:637-640`、`759-765` |
| `skills.version` 列宽 | `VARCHAR(80)`，超长会 500 | `009_skill_management.sql:7` |
| `Skill` 已带三类 binding 数组 | 属实 | `skill/types.go:38-40` |
| yaml.v3 把 `version: 1.0` 解进 string 会失败 | **不属实**，正常解成 `"1.0"` | 本次实测 |
| 可以用 `UPDATE ... RETURNING OLD.*` 拿旧值 | **不属实**，PG 16 无此语法（PG 18 才有），用 §8 的自连接写法 | `docker-compose.dev.yml:5` `postgres:16` |
| `DeleteSkill` 会清掉该技能的全部历史归档对象 | **不属实**，只删当前指针那一个 | `skill/service.go:328-333`，见 §10 |
