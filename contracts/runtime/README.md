# Runtime Local Contract

This contract describes the Runtime Agent local diagnostic HTTP API implemented by `apps/runtime-agent`.

`contracts/runtime/openapi.yaml` covers the local runtime endpoints:

- `GET /health`
- `GET /providers`
- `POST /runs`
- `GET /runs/{runId}`
- `GET /runs/{runId}/events`
- `POST /runs/{runId}/cancel`

Business task claim, event writeback, completion, failure, and lease renewal are Control Plane APIs. Their canonical contract lives in `contracts/control-plane/openapi.yaml`, and the human-readable endpoint guide is in `docs/api.md`.
