# Design System Prototypes

这个目录是设计系统拆分后的静态原型验证 kit，用来检查 `DESIGN.md` 与 `docs/design-system/` 是否足够指导页面生成。它不是生产组件库，也不是业务模板。

## 文件

- `design-system-prototypes.css`：静态原型共享样式，镜像 `apps/web/src/styles/theme.css` 的浅色 token。生产 token 仍以 `theme.css` 为事实源。
- `shell-dashboard.html`：覆盖 Shell、侧栏、顶部栏、Tabs、指标卡、表格和状态流。
- `overlays-forms.html`：覆盖 Dialog、Sheet、Popover、Toast、表单、按钮和语义反馈。
- `data-topology.html`：覆盖数据表、状态、趋势图、拓扑、空状态和权限状态。
- `project-closure-review.html`：覆盖项目验收、证据清单、短表单和人工意见。
- `runtime-node-detail.html`：覆盖 Runtime 健康、槽位进度、Provider 会话和日志区域。
- `workflow-template-review.html`：覆盖流程模板、策略摘要、阶段清单和拓扑预览。
- `prototype-icons.js`：原型图标 fallback。外部 Lucide 成功时使用 Lucide；网络不可用时渲染本地线性 SVG，保持验证稳定。
- `verify-prototypes.mjs`：本地验证脚本，启动临时静态服务并检查桌面/移动视口。

原型 HTML 使用固定的 `lucide@1.17.0` UMD 资源，与当前 `apps/web` 安装的 `lucide-react` 版本保持一致，避免 `latest` 漂移影响截图。

## 运行

轻量文档和引用校验：

```bash
corepack pnpm verify:design-system
```

浏览器布局与截图校验：

```bash
corepack pnpm verify:design-prototypes
```

脚本会尝试从仓库根目录和 `apps/web` workspace 解析已有的 `playwright` 依赖；如果 Playwright 自带浏览器不存在，会尝试使用本机 Chrome/Chromium。脚本不会安装依赖，也不会执行 `npx playwright install`。

验证内容：

- `1280x720` 桌面和 `390x844` 移动视口均可打开。
- 页面没有整体横向溢出。
- 常见按钮、Badge、标题和固定格式控件没有文本溢出。
- 图标已经渲染为 Lucide SVG，而不是停留在占位元素。
- 控制台没有 error 或 warning。
- 每个页面和视口保存一张截图到 `__screenshots__/`，截图文件不提交。

## 使用边界

- 新增原型时先按 `DESIGN.md` 路由读取最小设计子文档，再复用本目录 CSS。
- 原型文案只能是用于验证布局的中性样例，不要把某个业务场景的数据字段沉淀成设计规范。
- 如果 `apps/web/src/styles/theme.css` 的 token 变化，先更新生产 token，再同步本目录 CSS 的 token 镜像。
- 如果验证脚本报图标未渲染，优先检查外部 Lucide 资源是否可用；需要完全离线验证时，再把图标渲染本地化。
