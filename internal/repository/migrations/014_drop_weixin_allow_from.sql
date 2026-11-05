-- 删除 weixin_bot_accounts.allow_from
--
-- 为什么删：白名单只在 Bot 启动时读一次，后台改完不重启不生效（"看着有用实际无效"）；
-- 而 v1 是单用户自用，默认行为就是「只放行扫码者本人」，这个配置项只增加困惑。
-- 现在 Bot 只认 owner_user_id，不再有白名单概念。
ALTER TABLE weixin_bot_accounts DROP COLUMN IF EXISTS allow_from;
