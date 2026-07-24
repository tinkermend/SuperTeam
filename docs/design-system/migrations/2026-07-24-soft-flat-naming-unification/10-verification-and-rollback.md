# 10 · 验证矩阵与回滚手册

## 1. 通用验证命令

以仓库根为准；脚本名以 `package.json` 现存为准。

```bash
# 类型
corepack pnpm --filter @superteam/web typecheck

# 测试（实现阶段可先跑相关子集，阶段结束跑全量）
corepack pnpm --filter @superteam/web test

# 设计系统校验
corepack pnpm verify:design-system
# 若根脚本名不同：corepack pnpm exec node docs/design-system/verify-design-system.mjs

# 静态清零检查（阶段完成后）
rg '\bV3[A-Za-z]+\b' apps/web/src -g '!**/__screenshots__/**' || true
rg --hidden -n '--v3-|[^a-z]v3-[a-z]' apps/web/src -g '!**/__screenshots__/**' || true
rg -n "from ['\"]@/components/ui/button['\"]" apps/web/src/features apps/web/src/routes || true
```

## 2. 分阶段验证矩阵

| 检查项 | A | B | C | D | E | F |
| --- | --- | --- | --- | --- | --- | --- |
| typecheck | ✓ | ✓ | ✓ | ✓ 每切片 | ✓ 每子阶段 | ✓ |
| unit/browser tests | guard | 全量或 superteam+依赖 | button 相关+抽全量 | 切片相关→阶段末全量 | 全量 | verify |
| verify:design-system | 可选 | | | | E3 后强制 | ✓ |
| import guard | 上线 A2a | | | 升严格 | | |
| 视觉抽检 light | | 可选 | ✓ 按钮 | ✓ 切片页 | ✓ 强制 | |
| 视觉抽检 dark | | | 建议 | | ✓ 强制 | |
| rg 旧组件名业务引用 | | 仅剩 alias | | | | |
| rg 旧 token | | | | | E3=0 | |
| 截图更新说明 | | 通常无需 | 若有需列表 | 若有 | 可能大量 | |
| git 仅显式 path | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

## 3. 人工抽检清单（阶段 C/D/E 后）

最低集：

1. 登录页 / auth shell  
2. 登录后壳：侧栏选中态、顶栏搜索  
3. 项目列表 + 一密集表（审计或用户）  
4. 数字员工创建或团队创建（主/次/危险按钮）  
5. 收件箱或审批一条  
6. Dark 模式切换后重复 2–3  

记录：路由、是否异常透明/白块/按钮高度不一致。

## 4. 回滚手册

### 4.1 文档/A 阶段

```bash
git revert <sha>
```

无数据迁移。

### 4.2 B 组件改名

- 整 PR revert 最安全  
- 若已有后续 C 依赖新名，按依赖逆序 revert  

### 4.3 C Button 同源

- revert 后恢复双实现；注意 features 若已按新 variant 名修改需一并回滚  

### 4.4 D 切片

- 单切片 revert；guard allowlist 可能需暂时恢复  

### 4.5 E

| 子阶段 | 回滚 |
| --- | --- |
| E1 双挂 | revert；应无 class 依赖新名 |
| E2 某族 codemod | revert 该族；确认 `@theme` 仍双挂 |
| E3 删旧名 | **高危**：revert E3；若主干已有只写新名的新提交，需 rebase/补旧 alias 紧急双挂 |

### 4.6 紧急止血（E3 后线上白屏/掉样式）

1. 立即热修：在 `theme.css` 恢复 `--v3-*: var(--new)` 与 `@theme` 旧色名（重新双挂）  
2. 再安排正确修复  
3. 不要在热修里改业务功能  

## 5. 完成迁移的最终 DoD 检查单

复制到最后 PR：

- [ ] Q1–Q10 人类选择已归档（链接 checklist）
- [ ] features/routes 无 ui/button|badge|card
- [ ] 无业务 `V3*` 标识符（alias 已删）
- [ ] 无 `--v3-` / `text-v3-` / `bg-v3-` / `rounded-v3-` / `shadow-v3` 于 `apps/web/src`
- [ ] buttonVariants 唯一定义
- [ ] DESIGN + design-system 现行名一致
- [ ] verify:design-system 绿
- [ ] typecheck + test 绿
- [ ] 人工 light/dark 抽检通过
- [ ] 本目录 README 状态 = Done
- [ ] CHANGELOG 新增工程条目（可选但推荐）

## 6. 与「真实 e2e」关系

按 AGENTS.md：命名迁移属工程重构，**若无行为/交互契约变化**，不必完整业务 e2e；但 C/E 涉及全站视觉时，应用浏览器对上述抽检路径做一次真实加载（本地 dev 即可）。不得声称「用户流程已完整回归」除非额外执行。
