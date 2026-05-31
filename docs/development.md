# 开发指南

## 本地开发

### Web 控制台

Web 控制台位于 `apps/web`，使用 Vite、React、TanStack Router、TanStack Query 和 shadcn/ui。

本地启动：

```bash
pnpm dev:web
```

浏览器环境变量使用 `VITE_CONTROL_PLANE_URL` 指向 Control Plane，例如：

```dotenv
VITE_CONTROL_PLANE_URL=http://localhost:8080
```

验证：

```bash
pnpm verify:web
```
