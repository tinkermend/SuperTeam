-- 081: Drop project_placements — the legacy single-node runtime pin.
--
-- Plan B (migrations 048/049) moved node-selection authority to the
-- project_runtime_nodes eligibility set: dispatch, readiness, and the
-- pre-dispatch gate all read only that set, while the placement panel's
-- writes landed here with no effect ("intentionally not consulted").
-- The panel now manages project_runtime_nodes directly via
-- PUT/DELETE /projects/{id}/runtime-nodes/{nodeId}, so the zombie write
-- path and its table go away together.

DROP TABLE IF EXISTS project_placements;
