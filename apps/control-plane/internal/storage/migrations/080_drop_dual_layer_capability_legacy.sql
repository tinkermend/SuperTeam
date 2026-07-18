-- 080: Drop the migration-018 dual-layer capability legacy tables.
--
-- The MCP registry (migration 037: mcp_servers + team_mcp_bindings +
-- digital_employee_mcp_bindings_v2 + project_mcp_bindings) replaced this
-- layer end to end: the web console only calls the v2/registry endpoints,
-- and runtime dispatch projects MCP config exclusively from the registry
-- bindings. user_credentials existed only to back the legacy per-user MCP
-- token flow; registry credentials are env-var references sealed elsewhere.

DROP TABLE IF EXISTS digital_employee_mcp_bindings;
DROP TABLE IF EXISTS team_mcp_servers;
DROP TABLE IF EXISTS user_credentials;
