# 11 · 人工审核清单（放行门禁）

> 请人类 Owner 逐项勾选。未完成前 **禁止** 实现阶段 A–F 的代码开工（本清单文档本身的修订除外）。
> 日期：__________ 审核人：__________

## 已决事项（对话确认 · 2026-07-24）

- [x] **Q9**：批准阶段 C 将 ui/Button 几何对齐 Soft-Flat（消歧，不改色值）
- [x] **Q4**：**不强制** data-table 改 superteam Button；C 同源后基础设施白名单继续 `ui/button`（评估见 `13-decisions-log.md`）
- [x] **Q5**：已决选项 A — 真值合并（`bg-v3-card`→`bg-card` 等；独有 token 去前缀）

## 一、方案包完整性

- [x] 已阅读 `README.md` 与 `00-program-overview.md`
- [x] 已阅读 `01-inventory-and-scope.md`，认可范围纳入/排除
- [x] 已阅读 `02-naming-map.md` 映射策略
- [x] 已阅读 `03-prerequisites-and-risks.md` 风险寄存器
- [x] 已浏览阶段 A–F 文档，认可拆分粒度
- [x] 已阅读 `10-verification-and-rollback.md`
- [x] 已阅读 `12-self-review-log.md` 中的已知缺口与残留问题

## 二、目标与非目标

- [x] 同意目标 G1–G6（去 V3 前缀 + Button 双轨收敛 + 边界）
- [x] 同意非目标：不借机视觉改版、不做 packages/ui、不改历史 prototypes 路径
- [x] 同意其它 UI 债（Glass Tier、深色硬编码、容器断点）**不在**本迁移

## 三、开放决策（必须选）

在选项上打勾；若采纳草案推荐，勾「采用推荐」即可。

### Q1 `V3Tone` 目标名

- [x] 采用推荐：`Tone`
- [ ] 改为：`SemanticTone`
- [ ] 其它：__________

### Q2 表格套件命名

- [x] 采用推荐：`DataTable` + `Th`/`Td`/`Tr`（不导出 `Table`）
- [ ] 其它：__________

### Q3 业务按钮 data-slot

- [x] 采用推荐：`app-button`
- [ ] 使用：`button`（接受与 shadcn slot 可能混淆）
- [ ] 其它：__________

### Q4 data-table 是否改为 superteam Button

- [x] **已决：否（本迁移不强制）**，长期允许 data-table → ui/button（须阶段 C 样式同源）
- [ ] 推翻为：是（请说明）__________

### Q5 card/bg 与 shadcn token 合并

- [x] **已决：选项 A 真值合并**
- [ ] 推翻为：__________

### Q6 glass 类名

- [x] 采用推荐：`.glass` / `.glass-inner`
- [ ] `.surface-glass` / …
- [ ] 其它：__________

### Q7 是否允许永久 `st-`/`ds-` 前缀

- [x] 采用推荐：**否**
- [ ] 是，前缀为：__________

### Q8 alias 窗口

- [x] 采用推荐：B 后业务引用清零即可删；最长不超过 E 结束
- [ ] 固定保留至：__________

### Q9 阶段 C 将 ui/Button 对齐 Soft-Flat 尺寸/圆角（消歧）

- [x] **已决：批准**
- [ ] 推翻为：__________

### Q10 历史 `design-direction-v3` 路径

- [x] 采用推荐：**不改路径**
- [ ] 要改路径为：__________

## 四、执行策略

- [x] 同意阶段顺序 A→B→C→D→E→F，禁止单 PR 打穿
- [x] 同意共享 worktree 显式 `git add` 路径纪律
- [x] 同意映射表外命名必须先改文档
- [x] 同意阶段 A guard 使用 **A2a 基线冻结**（非立即严格 0）
- [x] 指定实现分支名：__________（建议 `codex/soft-flat-naming-unification`）
- [x] 指定是否需要独立 git worktree：是

## 五、放行

- [x] **我确认：可以开始阶段 A（仅文档制度 + guard，仍可先不 B）**
- [x] **我确认：可以在 A 完成后进入 B–F 实现**（可另次勾选）

### 阻断意见（如有）

```
（人类填写）
```

### 签字

```
审核人：
日期：
```
