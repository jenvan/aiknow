-- 微信文件传输助手（网页版协议）会话
--
-- 只存「只读客户端」需要的凭据：uin/sid/skey/pass_ticket + 会话 cookie。
-- 这些合起来等价于账号的网页登录态，**必须加密存储**（应用层 AES-GCM），
-- 泄露即以账号身份操作文件传输助手，因此本表只放密文，不落明文。
CREATE TABLE IF NOT EXISTS wechatweb_sessions (
    id             TEXT PRIMARY KEY,              -- v1 单账号，固定 'default'
    creds_enc      BYTEA NOT NULL,                -- 加密后的凭据 JSON（Token）
    user_name      TEXT NOT NULL DEFAULT '',      -- 自己的 UserName（仅留档）
    sync_key       JSONB,                         -- 同步游标，丢失=重登后只能收到新消息
    last_sync_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
