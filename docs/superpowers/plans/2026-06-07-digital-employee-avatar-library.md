# Digital Employee Avatar Library Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为数字员工创建和展示接入平台内置头像库，用户只能从 Control Plane 返回的资产中选择头像。

**Architecture:** 图片作为内置静态资源放在 Web public 目录，头像资产清单由 Control Plane 暴露和校验。数字员工创建保存 `avatar_asset_id` 与头像快照到 `metadata`，列表和详情从响应或 metadata 展示，历史数据使用稳定回退。

**Tech Stack:** Go chi/net/http、OpenAPI、React、TanStack Query、Vitest、Pillow/Sharp 图像裁剪。

---

### Task 1: 头像资产生成与裁剪

**Files:**
- Create: `apps/web/public/images/digital-employee-avatars/*.webp`

- [x] 生成 10 张男性、10 张女性虚构亚洲工程师写实头像。
- [x] 保留 512x512 WebP 主图。
- [x] 生成 256x256 WebP 缩略图。
- [x] 通过脚本检查每个文件存在、尺寸正确、数量为 40 个 WebP。

### Task 2: Web 头像库模型与展示测试

**Files:**
- Create: `apps/web/src/features/employees/avatar-library.ts`
- Create: `apps/web/src/features/employees/avatar.tsx`
- Modify: `apps/web/src/features/employees/create.test.tsx`
- Modify: `apps/web/src/features/employees/index.test.tsx`
- Modify: `apps/web/src/features/employees/detail.test.tsx`

- [x] 先写失败测试：创建请求必须带 `avatar_asset_id`。
- [x] 先写失败测试：列表页显示头像图片。
- [x] 先写失败测试：详情页显示头像图片。
- [x] 实现前端头像类型、稳定回退和图片组件。

### Task 3: Control Plane 头像资产 API 与创建校验

**Files:**
- Create: `apps/control-plane/internal/employee/avatar_assets.go`
- Modify: `apps/control-plane/internal/employee/types.go`
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/handler.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/employee_routes_test.go`
- Modify: `contracts/control-plane/openapi.yaml`

- [x] 先写失败测试：`GET /api/v1/digital-employee-avatar-assets` 返回 active 资产。
- [x] 先写失败测试：创建请求中的 `avatar_asset_id` 进入 service request。
- [x] 先写失败测试：无效头像资产 ID 被服务层拒绝。
- [x] 实现内置头像资产注册表、路由、OpenAPI schema 和创建校验。

### Task 4: 创建页接入头像选择

**Files:**
- Modify: `apps/web/src/lib/api/employees.ts`
- Modify: `apps/web/src/features/employees/create.tsx`

- [x] 创建页查询头像资产 API。
- [x] 身份步骤展示头像选择网格。
- [x] 创建提交带 `avatar_asset_id` 和 metadata 快照。
- [x] 缺少头像资产时阻断提交并显示错误。

### Task 5: 验证与记录

**Files:**
- Modify: `CHANGELOG.md`

- [x] 运行 `pnpm --filter @superteam/web test -- employees`。
- [x] 运行 `pnpm --filter @superteam/web typecheck`。
- [x] 运行 `go test ./apps/control-plane/internal/employee ./apps/control-plane/internal/api`。
- [x] 运行契约生成或校验命令，确认 OpenAPI 无明显破损。
- [x] 更新 `CHANGELOG.md`，新增北京时间变更记录。
