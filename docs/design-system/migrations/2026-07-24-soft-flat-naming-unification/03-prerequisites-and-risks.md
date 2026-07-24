# 03 · 前置条件、风险与约束

## 1. 前置条件（全部满足前不得开工实现）

### 1.1 决策前置

- [ ] `11-human-review-checklist.md` 中 Q1–Q10 均已勾选人类选择（或明确采纳草案推荐）
- [ ] 人类确认阶段切片与「禁止巨型 PR」策略
- [ ] 人类确认本迁移**不包含**其它 UI 债（Glass/深色/断点等）

### 1.2 工程前置

- [ ] 实现使用**独立 git 分支**（建议 `codex/soft-flat-naming-unification`）；共享 worktree 时遵守 AGENTS.md：禁止 `git add -A`、禁止切换/删除他人分支
- [ ] 开工前 `git status` 干净或仅含本迁移文档；若有他人脏文件，改用独立 worktree
- [ ] 本地可跑：`corepack pnpm --filter @superteam/web typecheck`、`test`、`verify:design-system`（根 package 脚本以仓库为准）
- [ ] 刷新 `01-inventory` 数字，确认与草案无数量级偏差（若 V3Button 数量翻倍，先停）

### 1.3 文档前置

- [ ] 本目录 00–12 经人类审阅无「阻断级」异议
- [ ] `02-naming-map.md` 被指定为唯一命名真相（写在 PR 模板/阶段 A 规范中）

### 1.4 知识前置（实现者必读）

- [ ] `DESIGN.md` 容器规则（SoftCard / WorkSurface / Glass）
- [ ] `docs/design-system/tokens.md` 与 `theme.css` 关系（theme 是事实源）
- [ ] Tailwind v4 `@theme inline` 暴露机制（改 CSS 变量名必须同步 `@theme`）
- [ ] 双轨现状：`V3Button` 与 `ui/button` **不是**包装关系，是平行实现

### 1.5 明确不作为前置的事项

- 不需要先做视觉改版评审
- 不需要先清空全部历史 docs 里的「v3」字样
- 不需要先启动全站 e2e 浏览器（阶段终验再做关键路径抽检）

---

## 2. 技术风险寄存器

| ID | 风险 | 可能性 | 影响 | 缓解 |
| --- | --- | --- | --- | --- |
| R1 | 组件改名与 `ui/button` 导出名冲突导致错误打包/错误导入 | 中 | 高 | 映射表 §2；features 禁 ui/button；typecheck |
| R2 | codemod 误替换 `design-direction-v3` 路径或文案 | 中 | 中 | 白名单目录 `apps/web/src`；禁止 docs/prototypes 自动替换 |
| R3 | token 改名漏 `var(--v3-*)` arbitrary 写法 | 高 | 高 | 双挂期；rg 双模式扫描；分族 PR |
| R4 | `@theme` 未暴露新色名导致 class 无样式 | 中 | 高 | 先挂 `@theme` 再 codemod class；目视 + 截图测试 |
| R5 | `data-slot` 与测试不同步 | 高 | 中 | 同 PR 改测试；专用 rg |
| R6 | 阶段 C 对齐 Button 尺寸引起截图大面积红 | 中 | 中 | Q9 预批；限定对齐清单；先跑 button 相关测试 |
| R7 | 共享 worktree 暂存到他人文件 | 中 | 高 | 显式 path add；提交前 `git status`/`git diff --cached` |
| R8 | 暗色主题只改 light 定义 | 中 | 高 | theme.css `.dark` 块同步双挂/改名 |
| R9 | feature CSS（aurora）漏网 | 中 | 中 | `find apps/web -name '*.css'` 纳入 E |
| R10 | verify-design-system 仍校验旧字符串导致假红/假绿 | 中 | 中 | 阶段 A/F 同步脚本 |
| R11 | 删除 alias 过早，外部未入库分支仍用 V3Button | 中 | 中 | Q8；删除前全仓 rg；通知并行会话 |
| R12 | `rounded-inner` 等短名与其它工具类语义撞车 | 低 | 中 | 改名前在 TW 生成物/文档中检索 |
| R13 | `Tone` 类型名过于通用，污染全局 IDE 符号 | 低 | 低 | Q1 可选 SemanticTone |
| R14 | 截图基线海量更新掩盖真回归 | 中 | 高 | 分 PR；禁止无说明更新截图；关键页人工看 |
| R15 | 并行功能开发与 rename 严重冲突 | 高 | 高 | 独立分支/worktree；选相对冷窗；先文档锁映射 |
| R16 | tokens.md 色值表与 theme 历史不一致，改文档时被「纠正」色值 | 中 | 中 | **禁止**本迁移改 hex；只改名字 |
| R17 | shadcn CLI 未来 regenerate 覆盖 button | 低 | 中 | button 样式源放在非 CLI 覆盖文件或文档警告 |
| R18 | 幽灵 class `shadow-v3-soft` 改名后仍无效 | 低 | 低 | 盘点时删除或补 token，不静默保留 |

---

## 3. 组织 / 流程风险

| ID | 风险 | 缓解 |
| --- | --- | --- |
| O1 | 一次 PR 做完所有阶段 | 阶段门禁；README 禁止 |
| O2 | 实现中即兴改名 | 映射表外 → 先改文档 |
| O3 | 人类审核未完成 Agent 已写代码 | README 状态 Draft；本条前置 1.1 |
| O4 | 与其它 UI 迁移纠缠 | 非目标列表；PR 描述检查 |

---

## 4. 回滚总原则

1. **阶段独立可回滚**：每个 phase PR 应可 `git revert` 而不依赖后续阶段。
2. **先加后删**：alias/双挂未删除前，回滚 codemod 即可；删除旧名后的回滚成本显著上升 → 删除 PR 必须单独且验证最严。
3. **禁止**在回滚 PR 中夹带新功能。
4. 详见 `10-verification-and-rollback.md`。

---

## 5. 并发与 Git 约束（SuperTeam 特有）

摘自 AGENTS.md，本迁移强制执行：

- 只用 `git add <显式路径>`
- 禁止 `git add -A` / `git add .`
- 禁止在共享 checkout 上 `checkout`/`switch`/`branch -D` 影响他人
- 提交前 `git symbolic-ref HEAD` 确认分支
- 与他人改动交织同一文件时：只暂存自己的 hunk，或独立 worktree

**本迁移高冲突文件**（尽量单会话占用）：

- `apps/web/src/styles/theme.css`
- `apps/web/src/styles/index.css`
- `apps/web/src/components/superteam/v3-components.tsx`（及改名后文件）
- `apps/web/src/components/ui/button.tsx`
- `DESIGN.md` / `docs/design-system/*`

---

## 6. 依赖顺序（硬）

```text
人类确认 Q* 
  → A（立法/guard 骨架）
  → B（组件改名）
  → C（Button 同源）  ※ C 不可先于「公共 Button 名已存在」
  → D（业务去掉 ui/button）※ 依赖公共 Button 可用
  → E（token）※ 可与 D 部分并行，但禁止同一文件混战
  → F 终态文档
```

允许：

- A 与「仅文档」F 草案更新同步
- E1（只双挂 token、不 codemod class）在 D 中期插入

禁止：

- 未双挂就删 `--v3-*`
- 未改测试就改 `data-slot`
- 未 typecheck 就推删除 alias

---

## 7. 安全与无障碍

- 本迁移不改变 DOM 结构意图；`asChild`/focus ring 行为保持
- 若阶段 C 调整 button class，必须保留 `focus-visible:ring-*`
- 不删除 `aria-*`；IconButton 仍需可访问名（既有规范）

---

## 8. 沟通模板（并行开发者）

> 我们将进行 Soft-Flat 命名去版本化（去 V3）与 Button 双轨收敛。  
> 映射表：`docs/design-system/migrations/2026-07-24-soft-flat-naming-unification/`  
> 请勿再新增 `V3*` 组件或 `ui/button` 的 feature 引用。  
> 高冲突文件：theme.css / v3-components / ui/button。  
> 细节以该目录文档为准，不要即兴起名。
