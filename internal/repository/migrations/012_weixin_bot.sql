-- 微信 iLink Bot 账号（扫码登录凭据 + 长轮询游标）
--
-- 为什么凭据落库而不进 .env：
--   bot_token 由扫码动态产生，属 L3 凭据；写进配置文件会随部署物料进 git、
--   进备份，且换账号要重启改文件。落库后 serve 启动时直接读，扫码与运行解耦。
--
-- 为什么游标必须持久化：
--   get_updates_buf 是长轮询的消费位点。三个开源 Go 实现都只放在内存变量，
--   重启即归零 → 重收旧消息（重复入库、重复回复）。内嵌在 aiknow 里发版、
--   跑迁移、改配置都会重启，必须落库。
CREATE TABLE IF NOT EXISTS weixin_bot_accounts (
    bot_id              TEXT PRIMARY KEY,              -- 形如 xxx@im.bot
    bot_token           TEXT NOT NULL,                 -- Bearer 凭据，勿打印
    base_url            TEXT NOT NULL DEFAULT '',
    owner_user_id       TEXT NOT NULL DEFAULT '',      -- 扫码授权的微信用户 xxx@im.wechat
    allow_from          TEXT NOT NULL DEFAULT '',      -- 逗号分隔白名单；空 = 仅 owner 可用
    get_updates_cursor  TEXT NOT NULL DEFAULT '',
    last_context_token  TEXT NOT NULL DEFAULT '',      -- 最近一次 inbound 的 context_token
    last_seen_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- v1 单 bot：取最近更新的一行即可，按 updated_at 建索引
CREATE INDEX IF NOT EXISTS idx_weixin_bot_accounts_updated_at
    ON weixin_bot_accounts (updated_at DESC);
