# SuperTeam Web 激进重铺设计

日期：2026-05-31
状态：已确认设计方向
范围：Web 前端重铺；Control Plane、Runtime Agent、数据库 schema 不在本轮重构范围内

## 1. 目标

本设计用于将当前 `apps/web` 从 Next.js App Router + 前端共享 packages 的结构，重铺为基于 `shadcn-admin` 的 Vite 单应用控制台。重铺后，Web 前端应成为一个自包含应用：页面、布局、UI 组件、认证状态、API client 和前端领域组合逻辑都位于 `apps/web/src` 下。

后端仍使用当前 Go Control Plane，现有认证、任务、Runtime 节点和用户相关 API 契约保持不变。本轮不改 Runtime Agent，不改 Control Plane 的职责边界，也不为未来 Desktop 端提前拆分 `packages/ui`、`packages/views`、`packages/core` 或 `packages/api-client`。

完成后，后续 Web 页面开发应直接围绕 `apps/web/src` 展开，不再通过跨 workspace package 组织前端业务代码。

## 2. 当前状态

当前仓库的 Web 前端具备以下特征：

- `apps/web` 使用 Next.js App Router。
- `packages/ui` 存放基础 UI 组件。
- `packages/views` 存放共享页面片段。
- `packages/core` 存放认证状态和领域 helper。
- `packages/api-client` 存放 TypeScript API client。
- 登录主链路已经接入真实 Control Plane API。
- 除登录、用户和少量真实 API client 外，大多数 Web 页面仍是占位控制台页面。

目标脚手架 `/Users/wangpei/src/github/Front/shadcn-admin` 的结构不同：

- 构建工具为 Vite。
- 路由使用 TanStack Router。
- 数据请求预期使用 TanStack Query。
- UI、布局、页面、hooks、context、store 和 API 组织在单个 `src` 内。
- 自带登录示例、Clerk 示例和 mock token，不符合 SuperTeam 当前 cookie session 认证方式。

因此，本轮不是页面迁移，也不是视觉换皮，而是 Web 应用壳重建。

## 3. 设计选择

采用激进重铺方案：删除旧 Web 和前端 packages 后，直接以 `shadcn-admin` 重建 `apps/web`。

选择该方案的原因：

- 当前 Web 除登录外主要是占位，迁移旧页面收益低。
- `shadcn-admin` 与当前 Next.js 技术栈差异较大，保守迁移会长期保留两套组织方式。
- 用户已明确不再为了未来 Desktop 提前拆分 UI 和 views。
- 将 API client、认证状态和页面逻辑收进 `apps/web/src` 后，早期 Web 功能开发更直接。
- Control Plane 后端契约已经是稳定边界，可以作为新 Web 的真实数据来源。

激进重铺不等于盲删。实施前必须先确认并迁移保留资产，尤其是当前唯一已验证的登录主链路。

## 4. 保留资产

删除旧前端结构前，必须保留以下资产和行为：

- Control Plane base URL 解析策略。
- `POST /api/auth/login` 登录调用。
- `GET /api/auth/me` 当前用户调用。
- `POST /api/auth/logout` 退出登录调用。
- `GET /api/auth/login-logs` 登录日志调用。
- 用户管理相关 API 调用。
- 任务和 Runtime 节点相关 API 调用。
- 跨域或跨端口请求必须带 `credentials: "include"`。
- 401 未认证响应必须进入未登录状态，并引导用户回登录页。
- 已登录用户访问登录页时应回到控制台入口。
- 未登录用户访问受保护页面时应被拦截。
- `apps/web/.env.example` 中与 Control Plane URL 相关的配置意图。
- 现有认证测试的意图，包括登录成功、登录失败、当前用户加载、退出登录和路由保护。

这些资产不要求逐文件保留，但新实现必须保留等价行为。

## 5. 新 Web 结构

新的 `apps/web` 采用单应用结构：

```text
apps/web/
  package.json
  vite.config.ts
  tsconfig.json
  tsconfig.app.json
  tsconfig.node.json
  components.json
  src/
    main.tsx
    routeTree.gen.ts
    routes/
    components/
      ui/
      layout/
      data-table/
    features/
      auth/
      dashboard/
      users/
      tasks/
      runtime/
      capabilities/
      approvals/
      workflows/
      audit/
    lib/
      api/
      auth/
      config/
      errors/
      utils.ts
    hooks/
    context/
    styles/
```

`src/components/ui` 继承 `shadcn-admin` 的组件源码。业务页面不再从 `packages/ui` 或 `packages/views` 导入组件。

`src/lib/api` 内收原 `packages/api-client` 的职责，负责 Control Plane HTTP 调用、请求配置、错误类型和 response 类型。它不持有 React 状态。

`src/features/auth` 内收原 `packages/core/auth` 的职责，负责登录页、认证 provider、当前用户加载、退出登录和路由守卫。

`src/features/*` 存放业务页面和页面内组件。首轮只要求恢复认证主链路和控制台壳，任务、用户、Runtime 节点等页面可以分阶段接入真实数据。

## 6. 认证与数据流

认证方式继续使用 Control Plane 的 cookie session，不使用 `shadcn-admin` 自带 mock token，不使用 Clerk。

登录流程：

1. 用户访问 `/login`。脚手架中的 `/sign-in` 示例不作为 SuperTeam 主登录路由；如短期保留，只能重定向到 `/login`。
2. 登录表单调用 `src/lib/api/auth.ts` 中的 login 方法。
3. 请求携带 JSON body 和 `credentials: "include"`。
4. Control Plane 设置 HTTP-only session cookie。
5. 前端保存当前用户摘要到认证状态。
6. 跳转到控制台首页。

当前用户流程：

1. 应用启动后调用 `/api/auth/me`。
2. 成功时进入已登录状态。
3. 401 时进入未登录状态。
4. 其他错误显示可恢复错误状态，不伪装成已登录。

退出流程：

1. 用户点击退出。
2. 前端调用 `/api/auth/logout`，携带 `credentials: "include"`。
3. 成功或 session 已失效后清空本地认证状态。
4. 跳转到登录页。

受保护路由通过 TanStack Router 的路由层或布局层实现。路由守卫只依赖认证状态，不直接散落在每个页面里。

## 7. 路由与页面策略

首轮新 Web 的路由分为三类：

- 公开路由：登录页、错误页。
- 受保护控制台路由：首页、任务、用户、Runtime 节点、能力、审批、工作流、审计。
- 未实现业务页：保留在导航中，但使用明确占位状态，不接 mock 业务数据冒充真实能力。

控制台壳直接使用 `shadcn-admin` 的侧边栏、顶部栏、主题切换、用户菜单和响应式布局。导航文案使用 SuperTeam 的业务语言，不沿用脚手架示例里的 apps、chats、settings 等默认业务含义。

第一阶段页面重点：

- 登录页接真实 Control Plane。
- 登录后控制台首页可访问。
- 用户菜单展示当前用户。
- 退出登录可用。
- 登录日志或用户页至少保留一个真实 API 验证入口。

后续页面再按任务中心、Runtime 节点、用户管理、能力、审批、工作流和审计逐步展开。

## 8. 前端 packages 退场

本轮实施完成后，Web 不再依赖以下 workspace package：

- `@superteam/ui`
- `@superteam/views`
- `@superteam/core`
- `@superteam/api-client`

如果仓库中没有其它应用继续依赖这些 packages，应删除对应目录，并更新：

- `pnpm-workspace.yaml`
- 根 `package.json` 的 `verify:web`、`test:ts`、`typecheck` 等脚本
- 相关 README 和开发文档
- `CHANGELOG.md`

如果 Desktop 空壳仍因 package workspace 配置受到影响，应让 Desktop 保持空壳自洽，而不是重新引入 Web 共享包。

## 9. 错误处理

API 错误采用统一类型封装：

- 401：清空认证状态并进入登录页。
- 403：进入无权限页面。
- 404：显示资源不存在状态。
- 5xx 或网络错误：显示可重试错误状态，并保留当前页面上下文。

表单错误不应只用 toast 表达。登录失败应在表单区域显示明确错误信息；系统级错误可以同时使用 toast 或错误页。

新 Web 不应在页面中直接散落 `fetch`。页面通过 `src/lib/api` 和 TanStack Query mutation/query 访问 Control Plane。

## 10. 验证策略

本轮至少需要以下验证：

```bash
pnpm install
pnpm --filter @superteam/web typecheck
pnpm --filter @superteam/web test
pnpm --filter @superteam/web build
go test ./apps/control-plane/...
```

如果 Go 集成测试依赖外部数据库、Redis 或测试环境，应按现有文档说明跳过或显式提供环境变量，不能把环境缺失误报为代码成功。

浏览器自测至少覆盖：

- 未登录访问控制台会进入登录页。
- 错误账号登录失败并显示错误。
- 正确账号登录成功并进入控制台。
- 刷新页面后仍能通过 `/api/auth/me` 恢复当前用户。
- 退出登录后再次访问控制台会被拦截。

测试迁移原则：

- 原 packages 中仍有价值的纯函数测试，迁移到 `apps/web/src` 对应模块旁边。
- 原占位页面测试不迁移。
- 新增认证、API config、路由守卫和关键页面 smoke test。

## 11. 实施阶段

### 阶段一：迁移前快照

记录旧 Web 中需要保留的认证、API、配置和测试资产。确认当前工作区无未迁移的重要本地改动。

### 阶段二：重铺 Web 壳

删除旧 `apps/web` 的 Next.js 结构，将 `shadcn-admin` 复制为新的 `apps/web` 基础。调整 package name、脚本、路径 alias、Vite 配置和 shadcn 配置。

### 阶段三：接入真实认证

移除 Clerk、mock token 和脚手架示例认证。内收并改写认证 API、AuthProvider、登录页、退出登录和路由保护。

### 阶段四：建立 SuperTeam 控制台导航

将脚手架默认业务页面替换为 SuperTeam 控制台导航和页面骨架。保留 layout、sidebar、header、theme、command menu 等通用体验。

### 阶段五：前端 packages 清理

确认 Web 不再引用 `packages/*` 后删除前端 packages，更新 workspace、根脚本、文档和 CHANGELOG。

### 阶段六：验证与修复

运行 Web typecheck、test、build 和必要后端测试。启动本地 Web 与 Control Plane，完成登录主链路浏览器自测。

## 12. 非目标

本轮不做以下工作：

- 不重写 Control Plane。
- 不修改 Runtime Agent。
- 不改数据库 schema。
- 不引入 OpenFGA、Temporal 或新的权限模型。
- 不实现完整任务中心、审批中心、工作流编排、能力调用或审计分析。
- 不为 Desktop 端提前恢复共享 UI/views packages。
- 不保留脚手架中的 Clerk 业务流。
- 不把脚手架 mock 数据包装成 SuperTeam 的真实功能。

## 13. 风险与应对

风险一：删除旧 Web 时丢失登录主链路。

应对：先建立保留资产清单，再以登录验收作为第一阶段完成标准。

风险二：脚手架依赖和仓库根依赖冲突。

应对：让 `apps/web/package.json` 自己声明 Vite、TanStack Router、TanStack Query、shadcn/ui 和测试依赖；根 package 只保留 workspace 级脚本。

风险三：旧 packages 删除影响 Desktop 空壳。

应对：实施前检查 Desktop 当前依赖。若 Desktop 只是空壳，应让它独立保留最小依赖，不把 Web 抽象重新拉回 packages。

风险四：登录 cookie 在 Vite dev server 与 Control Plane 之间失效。

应对：保持 `credentials: "include"`，确认 Control Plane CORS、cookie domain、SameSite 和本地端口配置。必要时使用 Vite proxy，但不改变后端认证语义。

风险五：脚手架示例页面带来产品语义污染。

应对：只保留通用布局和组件能力，默认示例业务页面必须删除或改为 SuperTeam 领域页面。

## 14. 验收标准

本轮完成的验收标准：

- `apps/web` 使用 Vite + TanStack Router + shadcn-admin 结构。
- Web 前端业务代码集中在 `apps/web/src`。
- Web 不再依赖 `@superteam/ui`、`@superteam/views`、`@superteam/core`、`@superteam/api-client`。
- 登录、当前用户、退出登录和路由保护接入真实 Control Plane。
- 未登录和已登录跳转行为正确。
- 至少一个登录后页面能调用真实 Control Plane API。
- 根脚本、workspace 配置和 Web package 脚本与新结构一致。
- 旧前端 packages 已删除或被证明仍有非 Web 消费方。
- `CHANGELOG.md` 记录本轮重铺。
- Web typecheck、test、build 通过。
- 浏览器自测覆盖登录主链路。

只有以上标准都有当前证据证明后，才能认为 Web 激进重铺完成。
