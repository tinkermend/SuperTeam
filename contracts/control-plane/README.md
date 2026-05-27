# Control Plane Contract

`openapi.yaml` is the source of truth for Console to Control Plane REST APIs.

Generated clients should be placed under `packages/api-client`. Server handlers in `apps/control-plane/internal/api` must stay compatible with this contract.
