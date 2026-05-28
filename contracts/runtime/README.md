# Runtime Contract

This contract defines HTTP APIs used between Runtime Agent nodes and the Control Plane.

The Runtime Agent implementation lives in `apps/runtime-agent` as a Rust/Tokio daemon. It still speaks neutral HTTP claim/lease APIs to the Go Control Plane, and real-time execution events are intended to be returned over the runtime WebSocket channel rather than embedded in provider-specific process output.

The first baseline covers node heartbeat, task claim, and lease renewal only. Task state naming and full workflow transitions are intentionally left out until the product model is clearer.
