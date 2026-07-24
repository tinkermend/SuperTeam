# 反馈与异步状态

## 何时阅读

实现列表加载、提交、Toast、空态、权限不足、长任务或「点了没反应」类交互时阅读。组件落点：`LoadingState` / `EmptyState` / `ErrorState` / `PermissionDenied` / `StateSurface`（`primitives.tsx`），Toast 见 `overlays.md`（Sonner）。

## 目标

用户始终能回答三件事：**现在怎样、为什么、下一步做什么**。反馈样式统一，业务只填槽位文案与数据。

## 状态类型

| 类型 | 含义 | 首选 UI |
| --- | --- | --- |
| 首屏加载 | 尚无可用 UI 骨架 | 页级 `LoadingState` 或 `StateSurface isLoading` |
| 局部刷新 | 表/卡/详情布局已知 | `TableSkeleton` / `CardGridSkeleton` / `DetailSkeleton` |
| 局部刷新 | 已有内容，后台再取 | 保持旧数据 + 轻量指示（角标「刷新中」、按钮 spinner）；**不要**整页替换成全屏转圈 |
| 空 | 成功响应但无条目 | `EmptyNoData` / 通用 `EmptyState` |
| 错误 | 请求/校验/系统失败 | `ErrorState` 或表单字段错；可重试则给 `onRetry` |
| 权限不足 | 401/403 或鉴权拒绝 | `PermissionDenied`；未登录走登录重定向（布局已处理） |
| 提交中 | 写操作进行中 | 主按钮 loading/disabled，防重复提交 |
| 长任务 | 超短请求时长的异步活 | 进度/阶段文案或入口跳转查看；禁止无限转圈无文案 |
| 成功轻反馈 | 写成功且停留本页 | Toast 短文案；列表需同步更新或 invalidate |
| 警告/提示 | 非阻断信息 | `Callout`（次级 Banner），不用 Error 实心大红整页 |

## 选用矩阵

| 场景 | 做法 | 禁止 |
| --- | --- | --- |
| 进入列表首次加载 | 主区 `LoadingState` | 空白主区无说明 |
| 筛选后无结果 | `EmptyNoMatch`（清筛选行动），勿与 `EmptyNoData` 同文案 | 与首次空列表同一句「暂无数据」且无行动 |
| 列表请求失败 | `ErrorState` + 重试 | 仅 Toast 后留下空白表 |
| 保存成功仍留页 | Toast「已保存」+ 表单去 dirty | 只改按钮文字一闪而过 |
| 保存失败 | 字段错优先；无字段则 Toast/顶栏错误 + 保留输入 | 清空用户已填内容 |
| 删除/归档等危险成功 | Toast + 返回列表或移除行 | 无反馈仍停在已删对象页 |
| 未登录访问鉴权页 | 重定向登录并带 `redirect` | 鉴权页闪一下 Empty |
| 403 | `PermissionDenied` | 伪装成 Empty「暂无数据」 |
| 后台 job | 明确「处理中」入口或进度 | 无超时、无失败态 |

## 文案槽位（业务填写，规范只定结构）

**EmptyState**

1. 标题：对象 + 状态（如「暂无项目」）
2. 说明：为何为空或如何开始（一句）
3. 行动：主按钮「创建…」或「清除筛选」（可选）

**ErrorState**

1. 标题：失败类型（「加载失败」）
2. 说明：可行动建议；技术细节可次级展示
3. 行动：重试（可选）

**PermissionDenied**

1. 标题：无权限
2. 说明：找谁/要什么权限（业务提供，不编造角色名）
3. 行动：返回或联系管理员（可选）

**Toast**

- 短、结果导向（「已归档」）；不复述整段错误栈。
- 同一操作成功/失败不要叠多条重复 Toast。

## 异步与数据

- **TanStack Query 等**：`isLoading`（无数据）与 `isFetching`（有数据再刷）必须区分 UI。
- 变更成功后：`invalidate` 或乐观更新，避免 Toast 成功但列表仍旧。
- 长任务：至少具备 `pending | running | success | failed` 心智；失败可重试或跳详情。

## 与组件的对应

| 组件 | 用途 |
| --- | --- |
| `StateSurface` | 统一 `isLoading` / `isError` / `denied` / `empty` 分支 |
| `LoadingState` | 块级加载 |
| `EmptyState` | 空 |
| `ErrorState` | 错 + 可选重试 |
| `PermissionDenied` | 无权限 |
| Button `loading`/disabled | 提交中 |
| Sonner Toast | 轻量结果；业务优先 `notifySuccess` / `notifyError` / `notifyWarning` / `notifyInfo` |

## 检查清单

- [ ] 首屏加载、空、错、无权限均有态，不靠空白
- [ ] 有数据刷新不整页闪白
- [ ] 写操作有进行中与结果反馈
- [ ] 403 不用 Empty 冒充
- [ ] 文案来自业务，结构符合槽位

### 空态预设（Batch D）

| 预设 | 场景 |
| --- | --- |
| `EmptyNoData` | 列表/集合本身为空 |
| `EmptyNoMatch` | 筛选/搜索后无命中 |
| `EmptyUnconfigured` | 能力未开通或必填配置缺失 |

