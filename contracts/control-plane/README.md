# Control Plane Contract

`openapi.yaml` is the source of truth for Console to Control Plane REST APIs.

Generated clients should be placed under `packages/api-client`. Server handlers in `apps/control-plane/internal/api` must stay compatible with this contract.

Generate Go server models and chi interfaces from the repository root with:

```bash
pnpm generate:control-plane
```

The generated Go file is intentionally ignored by Git through `*.gen.go`; regenerate it locally when changing the contract.
