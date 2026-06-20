-- runtime_leases was an early foundation table. Runtime lease state now lives on
-- project_task_attempts and the legacy runtime task lease endpoint is stateless.
DROP TABLE IF EXISTS runtime_leases;
