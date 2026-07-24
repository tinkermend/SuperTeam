# 09 · 阶段 F — 文档、校验脚本与历史策略

## 目标

现行设计文档与校验脚本使用 Soft-Flat 无版本前缀语言；历史材料不制造假历史。

## 必更新（现行规范）

| 文件 | 动作 |
| --- | --- |
| `DESIGN.md` | v3 用语 → Soft-Flat；组件/token 名跟映射表；迁移状态 → Done |
| `docs/design-system/tokens.md` | 全表改新名；删除「存量 v3 不强制改写」过时句 |
| `docs/design-system/actions.md` | 唯一 Button 路径 |
| `docs/design-system/surfaces.md` | glass class、token 名 |
| `docs/design-system/data-display.md` | 组件名 |
| `docs/design-system/layout-density.md` | layout token 名 |
| `docs/design-system/navigation.md` | shell token |
| `docs/design-system/visual-language.md` | 用语 |
| `docs/design-system/principles.md` | 基线名 |
| `docs/design-system/forms.md` / `overlays.md` / `icons.md` | 检索替换现行 API |
| `docs/design-system/verify-design-system.mjs` | 断言新路径/新 token 名；原型目录名可仍叫 design-direction-v3 |
| 本迁移 `README.md` 状态 | Done + 完成日期 |

## 刻意不更新（默认）

| 区域 | 策略 |
| --- | --- |
| `docs/prototypes/design-direction-v3/**` | **不改路径**；在 `docs/prototypes/README` 或 DESIGN 路由表注明历史代号 v3 |
| `docs/superpowers/plans|specs/**` 旧文 | 不批量替换 |
| `CHANGELOG.md` 旧条目 | 不改正文；可选在迁移完成时新增一条 changelog 记录本次工程 |

## AGENTS.md

- 若 AGENTS 提及 v3 组件/token，同步到新名（检索 `V3`/`v3-`）

## 完成定义

- 新加入的开发者只读 DESIGN + design-system 不会学到 `V3Button`/`text-v3-ink` 作为现行 API
- verify 脚本与代码一致

## 验证

- [ ] `pnpm verify:design-system`（或仓库等价脚本）
- [ ] 人工通读 DESIGN 路由表链接无死链
