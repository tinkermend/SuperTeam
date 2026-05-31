# Control Plane 契约

`openapi.yaml` 是 Console 调用 Control Plane REST API 的事实源。

Console TypeScript API 访问代码现在位于 `apps/web/src/lib/api`。Go 服务端 handler 位于 `apps/control-plane/internal/api`，并且必须与该契约保持兼容。

在仓库根目录运行以下命令生成 Go 服务端模型和 chi 接口：

```bash
pnpm generate:control-plane
```

生成的 Go 文件通过 `*.gen.go` 被 Git 忽略；修改契约后需要在本地重新生成。
