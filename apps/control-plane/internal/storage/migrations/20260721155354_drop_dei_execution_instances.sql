-- 物理拆表版 dei 退役:删除 readiness 视图与 digital_employee_execution_instances 表。
-- 外科手术版(20260721152932)已清空数据并封写;应用层不再读这两对象。

DROP VIEW IF EXISTS digital_employee_runtime_readiness;
DROP TABLE IF EXISTS digital_employee_execution_instances;
