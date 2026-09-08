-- 基于数据表 + FOR UPDATE SKIP LOCKED 的任务队列，替换 PGMQ 扩展
--
-- PGMQ 代做而此处需自行实现的三件事：
--   1. 可见性超时：消费者取走任务后若进程崩溃，租约到期后任务可被重新领取
--   2. 重试与死信：retry_count 达到 max_retries 后转为 failed，不再被领取
--   3. 归档：成功的任务移入 job_archive
--
-- 超时回收由应用层 Go 定时器执行（见 pkg/pgqueue），不使用 pg_cron：
-- pg_cron 只能配置在 postgres 库上且不可更改，跨库调度需 cron.schedule_in_database()，
-- 会额外引入部署期依赖。

CREATE TABLE IF NOT EXISTS job_queue (
    id          BIGSERIAL PRIMARY KEY,
    queue_name  TEXT        NOT NULL,
    payload     JSONB       NOT NULL,
    -- 任务可被领取的时间点；status='processing' 时表示当前租约到期时间
    visible_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- 已领取次数（领取时自增，即"第 N 次尝试"）
    retry_count INTEGER     NOT NULL DEFAULT 0,
    max_retries INTEGER     NOT NULL DEFAULT 3,
    -- pending：待领取；processing：已领取且租约未到期；failed：重试耗尽（死信）
    status      TEXT        NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending', 'processing', 'failed')),
    last_error  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 出队扫描索引：仅覆盖可领取状态，failed 行不进索引
CREATE INDEX IF NOT EXISTS idx_job_queue_poll
    ON job_queue (queue_name, visible_at, id)
    WHERE status IN ('pending', 'processing');

-- 死信查询索引
CREATE INDEX IF NOT EXISTS idx_job_queue_failed
    ON job_queue (queue_name, updated_at DESC)
    WHERE status = 'failed';

-- 归档表：成功处理的任务
CREATE TABLE IF NOT EXISTS job_archive (
    id           BIGINT      PRIMARY KEY,
    queue_name   TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    retry_count  INTEGER     NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL,
    archived_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_job_archive_archived_at
    ON job_archive (archived_at);

-- updated_at 自动维护
CREATE OR REPLACE FUNCTION touch_job_queue_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_touch_job_queue ON job_queue;
CREATE TRIGGER trg_touch_job_queue
    BEFORE UPDATE ON job_queue
    FOR EACH ROW
    EXECUTE FUNCTION touch_job_queue_updated_at();
