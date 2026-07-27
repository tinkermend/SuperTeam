-- 飞书通道可观测 P1:connector 心跳快照 + outbox 运营索引。
-- heartbeat 按 (tenant_id, service_name) 单行 upsert;不建 runtime 级节点表。

CREATE TABLE IF NOT EXISTS feishu_connector_heartbeats (
    tenant_id uuid NOT NULL,
    service_name varchar(128) NOT NULL,
    version varchar(64) NOT NULL DEFAULT '',
    last_heartbeat_at timestamptz NOT NULL,
    last_outbox_poll_at timestamptz,
    apps_snapshot jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, service_name)
);

COMMENT ON TABLE feishu_connector_heartbeats IS '飞书 connector 进程心跳快照(每 tenant+service_name 一行);Console 健康摘要与断连看门狗读取';
COMMENT ON COLUMN feishu_connector_heartbeats.apps_snapshot IS '每 app 连接态数组:[{app_id,config_id,ws_status,last_ws_event_at?}]';

CREATE INDEX IF NOT EXISTS idx_feishu_outbox_ops_status
    ON feishu_outbox (tenant_id, status, updated_at DESC)
    WHERE status IN ('failed', 'skipped_unbound');
