# 12 · 方案自检记录

> 起草方在提交人类审核前的复检。发现新问题应回写 01–10，而不是只写在对话里。

## 复检轮次

### Round 1 — 2026-07-24（初稿）

**已覆盖**

- 目标/非目标/阶段拆分
- 组件、token、class、data-slot、文档用语映射
- shadcn 重名冲突与 import 边界
- 双挂删除纪律、回滚、并发 git 约束
- 测试 data-slot 依赖
- 开放决策 Q1–Q10
- 人工放行清单

**主动排查的问题点 → 处理**

| 问题点 | 是否写入方案 | 落点 |
| --- | --- | --- |
| V3 是版本号应去除 | 是 | 00/02 |
| Button 双实现非包装关系 | 是 | 00/06/03 |
| actions.md 制度鼓励双轨 | 是 | 04 |
| features 约 20 处 ui/button | 是 | 01/07 |
| superteam 自身引用 ui/button | 是 | 07 D0 |
| `@theme` 与 CSS 变量双层 | 是 | 02/08 |
| shell/aurora 多未进 TW color | 是 | 01/08 |
| dark 块必须同步 | 是 | 03 R8 / 08 |
| 测试绑定 data-slot/class | 是 | 01/05/10 |
| 截图海量更新风险 | 是 | 03 R14 / 10 |
| 共享 worktree add -A | 是 | 03/00 |
| 历史 prototypes 路径 | 是 | 00/09 Q10 |
| CHANGELOG 不改假历史 | 是 | 00/09 |
| 色值表与 theme 可能不一致 | 是 | 03 R16 禁止改 hex |
| 幽灵 shadow-v3-soft | 是 | 01/08 E4 |
| LegacyTone 映射 | 是 | 01 |
| task-launch 等 feature CSS | 是 | 03 R9 / 08 |
| verify 脚本旧字符串 | 是 | 04/09 |
| packages/ui 空包勿激活 | 是 | 00 |
| Tailwind「v3」词义混淆 | 是 | 00 背景 |
| 与其它 UI 债边界 | 是 | 00 非目标 |
| e2e 期望管理 | 是 | 10 §6 |
| codemod 误伤 prototypes | 是 | 03 R2 / 08 |
| variant default/destructive 映射 | 是 | 07 |
| size=lg 无对应 | 是 | 07 |
| PageTabs vs ui/tabs 并存 | 是 | 02 不强制本迁移消灭 ui/tabs |
| Primary 与 brand 合并 | 是 | Q5 |
| Alias 删除时机 | 是 | Q8 |
| 紧急热修双挂 | 是 | 10 §4.6 |

**已知残留 / 需人类知晓的缺口**

1. **Inventory 数字是快照**，开工必刷新；未在 CI 固化自动 inventory 报告（可后续加，非阻断）。  
2. **未逐行人工 diff 全部 90 个 CSS 变量的 dark 值**写入映射表——映射按「去前缀、后缀不变」规则覆盖；若某变量无 dark 覆写，双挂时保持同样结构即可。  
3. **未运行 codemod dry-run**（本阶段禁止开工）。  
4. **`ui/tabs` 业务引用**保留为允许；与 `PageTabs` 双轨是**有意缩小范围**，可能残留第二套 Tabs 视觉——已列非本迁移必达，建议人类知悉，可另开「Tabs 收敛」小迁移。  
5. **`ui/table` 目前业务引用少**，但 data-table 自建 table 标记；`DataTable` 命名需避免与 `@tanstack/react-table` 概念口语混淆（文档中称 Soft-Flat DataTable 套件）。  
6. **Badge 在 team-role** 的语义是角色标签不是状态——D 阶段应映射到合适 chip/badge 样式，避免全部变成 StatusPill。已在 07 原则提及，**建议实现时加一眼评审**。  
7. **input/select 等表单控件**仍走 shadcn，本迁移不统一 Form 视觉——知悉。  
8. **AGENTS.md** 是否含 v3 需 F 阶段检索，已写 09，但初稿未全文扫描 AGENTS（应用 `rg` 在 F0）。  
9. **根 package 脚本确切名**以当时 package.json 为准；10 中命令可能需微调。  
10. **国际化/无障碍**无额外回归套件，依赖既有测试 + 抽检。

### Round 2 — 2026-07-24（结构复检）

**检查项**

- [x] 多文档交叉链接：README 目录齐全
- [x] 阶段依赖无环：A→B→C→D→E→F
- [x] 每个阶段有目标/非目标/验证/完成定义/回滚
- [x] 开放决策集中在 02 与 11，不分散遗漏
- [x] 高风险 E 单独强调双挂
- [x] 映射表含组件/token/class/slot/文档
- [x] 前置条件可勾选
- [x] 明确「先改文档再改映射外代码」

**Round 2 补充写回**

- 在 README 强调 Draft 禁止开工  
- 在 07 强调 team-role Badge 非状态（本 log 缺口 #6）  
- 在 02 增加 Q 汇总表  

**Round 2 结论**：方案包达到「可交人类审核」标准；**未达**「可免审直接开工」。

### Round 3 — 交付前（文档链完整性）

```text
期望文件：
README.md
00-program-overview.md
01-inventory-and-scope.md
02-naming-map.md
03-prerequisites-and-risks.md
04-phase-a-governance.md
05-phase-b-component-rename.md
06-phase-c-button-single-source.md
07-phase-d-import-boundaries.md
08-phase-e-token-rename.md
09-phase-f-docs-and-history.md
10-verification-and-rollback.md
11-human-review-checklist.md
12-self-review-log.md
```

（生成后应 `ls` 核对。）

## 建议人类重点看的三处

1. **`02` Q5**：card/background 与 shadcn 合并方式——决定 E 阶段 class 风暴形态。  
2. **`06` Q9**：是否批准 ui 按钮几何对齐 Soft-Flat——决定 C 是否触碰截图。  
3. **`07` 范围**：ui/tabs 不强制收敛是否可接受——决定「双轨」残留程度。

## 下一步（仅在 11 放行后）

1. 人类勾选 `11-human-review-checklist.md`  
2. 若有修改意见 → 先改方案文档 → Round N 自检  
3. 放行后从 **阶段 A** 单独 PR 开工，禁止跳步 E


### Round 4 — 2026-07-24（人类反馈：Q5 澄清 / Q9 批 / Q4 代决）

- Q9：已批准，写人 06/11/13
- Q4：评估后决定本迁移不强制 data-table 换 import，写人 07/11/13
- Q5：补充白话说明于 13；仍待人类选 A/B


### Round 5 — 2026-07-24（Q5 = A）

- 人类确认 Q5 选项 A 真值合并；已写 02/08/11/13。
- 仍待：11 中其余 Q（Q1–Q3、Q6–Q8、Q10）与整体放行勾选。
