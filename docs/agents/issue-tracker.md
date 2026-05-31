# Issue tracker: Local Markdown

本仓库的 issue 和 PRD 以 markdown 文件形式存放在 `.scratch/` 下。

## 约定

- 每个功能一个目录：`.scratch/<feature-slug>/`
- PRD 文件：`.scratch/<feature-slug>/PRD.md`
- 实现 issue：`.scratch/<feature-slug>/issues/<NN>-<slug>.md`，从 `01` 编号
- 分类状态以 `Status:` 行记录在 issue 文件顶部附近（标签字符串见 `triage-labels.md`）
- 评论和讨论历史追加到文件底部 `## Comments` 标题下

## 当技能提示 "publish to the issue tracker"

在 `.scratch/<feature-slug>/` 下创建新文件（必要时先创建目录）。

## 当技能提示 "fetch the relevant ticket"

读取指定路径的文件。用户通常会直接提供路径或 issue 编号。
