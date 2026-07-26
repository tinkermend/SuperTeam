-- 通讯录反查手机号腿(spec 2026-07-27-feishu-channel-access-management §6 提前项):
-- 国内飞书档案手机号必填、邮箱常空,邮箱单腿反查实操命中率低。auth_users 增加
-- 手机号列供 batch_get_id mobiles 反查;可空,不设唯一约束(联系方式非登录标识)。
ALTER TABLE auth_users ADD COLUMN mobile varchar(32);

COMMENT ON COLUMN auth_users.mobile IS '手机号(含国际区号,如 +8613800138000);用于飞书通讯录反查绑定,非登录凭据';
