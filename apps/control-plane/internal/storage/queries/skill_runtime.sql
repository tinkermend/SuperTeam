-- name: ListRequiredToolsForNode :many
-- dei retired: required tools are delivered via dispatch payload/MCP config, not employee-node bindings.
SELECT ''::text AS tool
WHERE false;
