# 新建团队页面优化 Plan（2026-06-25）

> 复核状态：06-25团队创建页面优化

## 背景与问题

`apps/web/src/features/teams/components/create-team-page.tsx` 的新建团队页把"创建必填项"与"创建后待办项"混在同一视觉层，导致：

1. **右栏治理策略/对外借调显示 `待配置`/`待设置`，用 amber 警告 pill**——语义是"创建后开放的能力"，样式却等同"表单缺失/警告"，读起来像漏填。
2. **`团队标识 slug` 与名称并排同等大小**，不知道是啥；且帮助文案写"由名称自动生成"，但 `updateName` 实际并没有生成 slug（文案与实现不一致）。
3. **配色与图标**：实时预览根本没渲染所选图标（用的是 `previewName.slice(0,1)` 首字母方块），选了没反馈；且只有 5 个耦合的"图标=色调"组合，看起来单薄无用。
4. **团队负责人**：`UserSearchSelect` 始终内联平铺整列候选用户，且选中后该组件自带卡片 + 页面又有一张确认卡，三段重复、很高、观感 low。
5. **附带**：`create-team-members-step.tsx` 顶部"基础信息"小表（名称/slug/负责人）与右栏实时预览重复，且此刻全是 `-`，观感差。

## 设计原则（对齐 v3 Soft-Flat / DESIGN.md）

- 视觉语义分层：**创建必填（身份/负责人/成员）** 与 **创建后待办（治理/借调/员工/能力）** 彻底区分；待办用中性 pill，不用 amber 警告色。
- 复用项目级组件：`TeamIconTile`、`UserIdentity`，不新造卡片/pill 样式；token 用 `--v3-*`。
- 预览卡为唯一实时事实源，真实反映图标+配色。

## 改动清单

### 1. `create-team-draft.ts`
- 新增 `slugify(name)`：中文/空格/符号 → kebab、小写、去重连字符。
- `updateName` 路径真正自动生成 slug（仅在用户未手动改 slug 时）。为此在 draft 增加 `slugTouched` 标志，`updateSlug` 时置 true。
- 拆分耦合的 `teamIconOptions`，新增独立的 `teamIconChoices`（5 个 icon_key）与 `teamToneChoices`（5 个 color_tone），让"图标"和"颜色"可独立选择，**不扩展 union、不动后端 metadata 契约**。

### 2. `create-team-page.tsx`
- **预览卡**：把首字母方块替换为 `TeamIconTile`（传 `draft.display`），选图标/颜色即时反馈。
- **团队身份区**：
  - 名称单独占一行（主输入）。
  - slug 降级为名称下方一行：`URL 标识：team-slug ✎`，默认只读 chip + 编辑切换；帮助文案给真实示例"用于网址与接口，例如 /teams/team-slug"。
  - "配色与图标"拆成两排：图标一排（`teamIconChoices`）+ 颜色一排（`teamToneChoices` 色板圆点）。
- **负责人区**：页面内管理 `editingOwner` 状态。选中且非编辑态 → 紧凑负责人卡（`UserIdentity` + `人类·团队管理者` pill + "更换"按钮），不再内联平铺候选列表；点击"更换"或未选中 → 展开 `UserSearchSelect`。默认预填当前操作人由上层 props/draft 决定（本次不改预填逻辑）。
- **右栏生命周期**：`待配置`/`待设置` 改为中性 `stateTone`（去 amber），文案改"创建后配置"；标题区"创建只是第一步"→强调"创建后解锁"。`LifecycleRow` 的 warning 分支保留但本页不再使用（或直接删除 warning 分支，统一中性 + 锁语义）。

### 3. `create-team-members-step.tsx`
- 删除顶部"基础信息"小表（`<section>` 名称/slug/负责人 dl 块），消除与预览卡重复。保留下方"特权角色申请"提示与候选/已选成员表。

## 不在本次范围
- 左侧导航重复渲染 bug（"核心导航/流程编排"重复多次）——属导航数据源问题，单独处理。
- 扩展图标/色板数量（需改 union + TeamIconTile + 后端 metadata 校验）。
- 负责人默认预填逻辑变更。

## 验证
- `corepack pnpm --filter ./apps/web run test`（受影响：create-team-page.test、create-team-members-step、team-icon-tile）。
- 真实 Web：`scripts/dev-services.sh status` 确认 Web/Control Plane 在跑，用 Chrome plug 打开 `/teams/new`，验证：预览随图标/颜色变化、slug 自动生成且可编辑、负责人塌缩卡、右栏中性 pill、无横向溢出、能真实创建团队。
