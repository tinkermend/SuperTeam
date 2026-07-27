-- connector 探活已迁 Redis TTL（见 feishu.RedisHeartbeatStore）；PG 快照表退役。
-- 历史迁移 20260727021805 仍保留 CREATE（Atlas 已应用历史不可改写）；本迁移 drop 表。
-- outbox 运营索引 idx_feishu_outbox_ops_status 仍在用，不在此删。

DROP TABLE IF EXISTS feishu_connector_heartbeats;
