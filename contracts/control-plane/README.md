# Control Plane Contract

`openapi.yaml` is the source of truth for Console to Control Plane REST APIs.

Console TypeScript API 访问代码现在位于 `apps/web/src/lib/api`。Go 服务端 handler 位于 `apps/control-plane/internal/api`，并且必须与该契约保持兼容。

Generate Go server models and chi interfaces from the repository root with:

```bash
pnpm generate:control-plane
```

The generated Go file is intentionally ignored by Git through `*.gen.go`; regenerate it locally when changing the contract.
