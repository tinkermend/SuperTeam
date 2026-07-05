# Prototype Instructions

Run the local server yourself and open the preview in the in-app browser. Do not give the user server-start instructions when you can run it.

Before making substantial visual changes, use the Product Design plugin's `get-context` skill when the visual source is unclear or no longer matches the current goal. When the user gives durable prototype-specific design feedback, preferences, or decisions, record them in `AGENTS.md`.

When implementing from a selected generated mock, treat that image as the source of truth for layout, component anatomy, density, spacing, color, typography, visible content, and hierarchy.

For the runtime overview office map, distinguish visual-lock prototypes from production implementation. A visual-lock prototype may use the full reference image as a ground-truth visual layer to prove 1:1 fidelity first. Production implementation must not hide live team cards, counts, employee avatars, selected rings, or statuses inside screenshot tiles; employee presence must be a separate coordinate-driven data layer using fields such as `employeeId`, `teamId`, `x`, `y`, `avatar`, `status`, `role`, and current task, so the right detail panel and selected team state can be driven by the same record.
