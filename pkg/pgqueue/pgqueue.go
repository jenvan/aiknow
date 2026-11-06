// Package pgqueue 基于数据表 + FOR UPDATE SKIP LOCKED 实现的任务队列。
//
// 替代 PGMQ 扩展（需编译安装且不在 ParadeDB 镜像内）。相比直接调用 pgmq 函数，
// 以下三项由本包自行实现：
//
//   - 可见性超时：Pop 领取任务时写入租约到期时间，消费者进程崩溃后
//     租约到期，任务可被重新领取，不会永久卡死。
//   - 重试与死信：每次领取自增 retry_count，超过 max_retries 后
//     由 Fail 置为 failed 状态，不再进入出队扫描。
//   - 归档：Archive 将成功任务移入 job_archive 表。
//
// 表结构见 migrations/007_job_queue.sql。
package pgqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // 注册 pgx 数据库驱动
)

// 默认参数
const (
	// DefaultLeaseDuration 默认租约时长：领取后多久未确认即视为消费者失联。
	//
	// 必须显著大于单篇文章的处理上限（见 service.pipelineTimeout），否则
	// "还在正常处理"的任务会被回收并重投，造成同一篇文章被并发处理两次。
	DefaultLeaseDuration = 10 * time.Minute
	// DefaultBaseBackoff 重试退避基数，按 2^retry_count 指数增长
	DefaultBaseBackoff = 10 * time.Second
	// DefaultMaxBackoff 重试退避上限
	DefaultMaxBackoff = 10 * time.Minute
	// DefaultReclaimInterval 超时任务回收扫描间隔
	DefaultReclaimInterval = 1 * time.Minute
)

// Config 队列配置
type Config struct {
	// LeaseDuration 租约时长。Pop 后任务在此时长内对其他消费者不可见。
	LeaseDuration time.Duration
	// BaseBackoff 重试退避基数
	BaseBackoff time.Duration
	// MaxBackoff 重试退避上限
	MaxBackoff time.Duration
	// ReclaimInterval 回收扫描间隔
	ReclaimInterval time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		LeaseDuration:   DefaultLeaseDuration,
		BaseBackoff:     DefaultBaseBackoff,
		MaxBackoff:      DefaultMaxBackoff,
		ReclaimInterval: DefaultReclaimInterval,
	}
}

// Queue 任务队列
type Queue struct {
	db     *sql.DB
	name   string
	config Config

	mu       sync.Mutex
	cancel   context.CancelFunc
	reclaimW sync.WaitGroup
}

// Message 队列消息
type Message struct {
	// MsgID 任务 ID（job_queue.id）
	MsgID int64 `json:"message_id"`
	// Value 任务载荷
	Value json.RawMessage `json:"value"`
	// RetryCount 本次为第几次尝试（领取时已自增，首次为 1）
	RetryCount int `json:"retry_count"`
	// MaxRetries 最大尝试次数
	MaxRetries int `json:"max_retries"`
	// CreatedAt 入队时间
	CreatedAt time.Time `json:"created_at"`
}

// NewQueue 创建队列，使用默认配置
func NewQueue(db *sql.DB, name string) *Queue {
	return NewQueueWithConfig(db, name, DefaultConfig())
}

// NewQueueWithConfig 使用指定配置创建队列
func NewQueueWithConfig(db *sql.DB, name string, config Config) *Queue {
	def := DefaultConfig()
	if config.LeaseDuration <= 0 {
		config.LeaseDuration = def.LeaseDuration
	}
	if config.BaseBackoff <= 0 {
		config.BaseBackoff = def.BaseBackoff
	}
	if config.MaxBackoff <= 0 {
		config.MaxBackoff = def.MaxBackoff
	}
	if config.ReclaimInterval <= 0 {
		config.ReclaimInterval = def.ReclaimInterval
	}
	return &Queue{db: db, name: name, config: config}
}

// Name 返回队列名
func (q *Queue) Name() string { return q.name }

// CreateQueue 校验队列所依赖的表是否已就绪。
//
// 表由数据库迁移创建（见 migrations/007_job_queue.sql），本方法不建表，
// 仅做存在性检查以便在启动阶段尽早暴露迁移未执行的问题
func (q *Queue) CreateQueue(ctx context.Context) error {
	var exists bool
	err := q.db.QueryRowContext(ctx,
		`SELECT to_regclass('public.job_queue') IS NOT NULL`,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("检查 job_queue 表: %w", err)
	}
	if !exists {
		return fmt.Errorf("job_queue 表不存在，请先执行数据库迁移")
	}
	return nil
}

// Send 投递消息，delay 为延迟可见的秒数（0 表示立即可见）。
// 最大重试次数取 job_queue.max_retries 的表默认值。
func (q *Queue) Send(ctx context.Context, message interface{}, delay int) (int64, error) {
	return q.send(ctx, message, delay, nil)
}

// SendWithMaxRetries 投递消息并指定该任务的最大尝试次数
func (q *Queue) SendWithMaxRetries(ctx context.Context, message interface{}, delay, maxRetries int) (int64, error) {
	if maxRetries < 1 {
		maxRetries = 1
	}
	return q.send(ctx, message, delay, &maxRetries)
}

func (q *Queue) send(ctx context.Context, message interface{}, delay int, maxRetries *int) (int64, error) {
	data, err := json.Marshal(message)
	if err != nil {
		return 0, fmt.Errorf("序列化消息: %w", err)
	}

	if delay < 0 {
		delay = 0
	}

	var msgID int64
	if maxRetries == nil {
		err = q.db.QueryRowContext(ctx, `
			INSERT INTO job_queue (queue_name, payload, visible_at)
			VALUES ($1, $2, now() + make_interval(secs => $3::double precision))
			RETURNING id
		`, q.name, data, float64(delay)).Scan(&msgID)
	} else {
		err = q.db.QueryRowContext(ctx, `
			INSERT INTO job_queue (queue_name, payload, visible_at, max_retries)
			VALUES ($1, $2, now() + make_interval(secs => $3::double precision), $4)
			RETURNING id
		`, q.name, data, float64(delay), *maxRetries).Scan(&msgID)
	}
	if err != nil {
		return 0, fmt.Errorf("投递消息: %w", err)
	}

	return msgID, nil
}

// Pop 领取一条可见消息。
//
// 使用 FOR UPDATE SKIP LOCKED 保证并发消费者不会领到同一条消息。
// 领取后写入租约到期时间并自增 retry_count；消费者须在租约内调用
// Archive（成功）或 Fail（失败），否则任务会被回收重投。
//
// 队列为空时返回 sql.ErrNoRows。
func (q *Queue) Pop(ctx context.Context) (*Message, error) {
	var msg Message
	err := q.db.QueryRowContext(ctx, `
		UPDATE job_queue
		SET status      = 'processing',
		    retry_count = retry_count + 1,
		    visible_at  = now() + make_interval(secs => $2::double precision)
		WHERE id = (
			SELECT id FROM job_queue
			WHERE queue_name = $1
			  AND status IN ('pending', 'processing')
			  AND visible_at <= now()
			  AND retry_count < max_retries
			ORDER BY id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, payload, retry_count, max_retries, created_at
	`, q.name, q.config.LeaseDuration.Seconds()).Scan(
		&msg.MsgID, &msg.Value, &msg.RetryCount, &msg.MaxRetries, &msg.CreatedAt,
	)
	if err != nil {
		// sql.ErrNoRows 表示队列为空，交由调用方判断，不包装以保留哨兵值
		return nil, err
	}

	return &msg, nil
}

// Fail 标记任务处理失败。
//
// 若剩余重试次数，按 2^retry_count 指数退避后重新可见；
// 重试耗尽则置为 failed（死信），不再被 Pop 领取。
// 返回值指示该任务是否已进入死信状态。
func (q *Queue) Fail(ctx context.Context, msgID int64, cause error) (dead bool, err error) {
	var errText sql.NullString
	if cause != nil {
		errText = sql.NullString{String: cause.Error(), Valid: true}
	}

	// 读取当前尝试次数以计算退避时长
	var retryCount, maxRetries int
	if err = q.db.QueryRowContext(ctx,
		`SELECT retry_count, max_retries FROM job_queue WHERE id = $1`, msgID,
	).Scan(&retryCount, &maxRetries); err != nil {
		return false, fmt.Errorf("查询任务 %d: %w", msgID, err)
	}

	if retryCount >= maxRetries {
		if _, err = q.db.ExecContext(ctx, `
			UPDATE job_queue SET status = 'failed', last_error = $2 WHERE id = $1
		`, msgID, errText); err != nil {
			return false, fmt.Errorf("标记死信 %d: %w", msgID, err)
		}
		return true, nil
	}

	backoff := q.backoffFor(retryCount)
	if _, err = q.db.ExecContext(ctx, `
		UPDATE job_queue
		SET status     = 'pending',
		    last_error = $3,
		    visible_at = now() + make_interval(secs => $2::double precision)
		WHERE id = $1
	`, msgID, backoff.Seconds(), errText); err != nil {
		return false, fmt.Errorf("重新入队 %d: %w", msgID, err)
	}
	return false, nil
}

// backoffFor 计算第 n 次尝试后的退避时长（指数增长并截断到上限）
func (q *Queue) backoffFor(retryCount int) time.Duration {
	if retryCount < 1 {
		retryCount = 1
	}
	// 限制指数上界，避免 float64 溢出
	if retryCount > 30 {
		return q.config.MaxBackoff
	}
	backoff := time.Duration(float64(q.config.BaseBackoff) * math.Pow(2, float64(retryCount-1)))
	if backoff > q.config.MaxBackoff || backoff <= 0 {
		return q.config.MaxBackoff
	}
	return backoff
}

// Archive 归档已成功处理的消息：移入 job_archive 并从队列删除
func (q *Queue) Archive(ctx context.Context, msgID int64) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启事务: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO job_archive (id, queue_name, payload, retry_count, created_at)
		SELECT id, queue_name, payload, retry_count, created_at
		FROM job_queue WHERE id = $1
		ON CONFLICT (id) DO NOTHING
	`, msgID)
	if err != nil {
		return fmt.Errorf("归档消息 %d: %w", msgID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// 消息不存在或已归档，仍尝试清理队列中的残留行
		if _, err = tx.ExecContext(ctx, `DELETE FROM job_queue WHERE id = $1`, msgID); err != nil {
			return fmt.Errorf("清理消息 %d: %w", msgID, err)
		}
		return tx.Commit()
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM job_queue WHERE id = $1`, msgID); err != nil {
		return fmt.Errorf("删除已归档消息 %d: %w", msgID, err)
	}

	return tx.Commit()
}

// Delete 直接删除消息（不归档），用于丢弃无效消息
func (q *Queue) Delete(ctx context.Context, msgID int64) error {
	if _, err := q.db.ExecContext(ctx,
		`DELETE FROM job_queue WHERE id = $1`, msgID,
	); err != nil {
		return fmt.Errorf("删除消息 %d: %w", msgID, err)
	}
	return nil
}

// Depth 返回队列中待处理的消息数（不含死信）
func (q *Queue) Depth(ctx context.Context) (int, error) {
	var depth int
	err := q.db.QueryRowContext(ctx, `
		SELECT count(*) FROM job_queue
		WHERE queue_name = $1
		  AND status IN ('pending', 'processing')
		  AND retry_count < max_retries
	`, q.name).Scan(&depth)
	if err != nil {
		return 0, fmt.Errorf("查询队列深度: %w", err)
	}
	return depth, nil
}

// DeadLetterCount 返回死信数量
func (q *Queue) DeadLetterCount(ctx context.Context) (int, error) {
	var n int
	err := q.db.QueryRowContext(ctx, `
		SELECT count(*) FROM job_queue WHERE queue_name = $1 AND status = 'failed'
	`, q.name).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("查询死信数量: %w", err)
	}
	return n, nil
}

// OldestMessageAge 返回队列中最早未完成消息的等待时长。
//
// 队列为空时返回 0 与 sql.ErrNoRows。
func (q *Queue) OldestMessageAge(ctx context.Context) (time.Duration, error) {
	var createdAt time.Time
	err := q.db.QueryRowContext(ctx, `
		SELECT created_at FROM job_queue
		WHERE queue_name = $1
		  AND status IN ('pending', 'processing')
		  AND retry_count < max_retries
		ORDER BY created_at
		LIMIT 1
	`, q.name).Scan(&createdAt)
	if err != nil {
		return 0, err
	}
	return time.Since(createdAt), nil
}

// ReclaimExpired 将租约已到期但仍处于 processing 的任务重置为 pending，
// 返回被回收的任务数。用于消费者进程崩溃后的任务恢复。
func (q *Queue) ReclaimExpired(ctx context.Context) (int, error) {
	// 还能重试的：回到 pending，等下一个消费者重新领取。
	res, err := q.db.ExecContext(ctx, `
		UPDATE job_queue
		SET status = 'pending', updated_at = now()
		WHERE queue_name = $1
		  AND status = 'processing'
		  AND visible_at <= now()
		  AND retry_count < max_retries
	`, q.name)
	if err != nil {
		return 0, fmt.Errorf("回收超时任务: %w", err)
	}
	n, _ := res.RowsAffected()

	// 重试已耗尽的：转死信。若不管它们，这些行会永远停在 processing：
	// 上面的条件把它们排除在外，而消费者只领取 pending，结果就是"卡住但没人处理"。
	// 实测踩过：一篇 40 分钟不动，队列里留着一个僵尸 processing 行。
	res2, err := q.db.ExecContext(ctx, `
		UPDATE job_queue
		SET status     = 'failed',
		    last_error = COALESCE(NULLIF(last_error, ''), '租约过期且重试已耗尽'),
		    updated_at = now()
		WHERE queue_name = $1
		  AND status = 'processing'
		  AND visible_at <= now()
		  AND retry_count >= max_retries
	`, q.name)
	if err != nil {
		return int(n), fmt.Errorf("回收超时任务（转死信）: %w", err)
	}
	n2, _ := res2.RowsAffected()
	return int(n + n2), nil
}

// PurgeArchive 清理归档表中早于 before 的记录，返回删除行数
func (q *Queue) PurgeArchive(ctx context.Context, before time.Time) (int, error) {
	res, err := q.db.ExecContext(ctx, `
		DELETE FROM job_archive WHERE queue_name = $1 AND archived_at < $2
	`, q.name, before)
	if err != nil {
		return 0, fmt.Errorf("清理归档: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// StartReclaimer 启动后台协程，按 ReclaimInterval 周期回收超时任务。
//
// 不使用 pg_cron：pg_cron 只能配置在 postgres 库上且不可更改，
// 跨库调度需 cron.schedule_in_database()，会引入额外部署期依赖。
// 重复调用无副作用（已启动时直接返回）。
func (q *Queue) StartReclaimer(onError func(error)) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.cancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	q.cancel = cancel

	q.reclaimW.Add(1)
	go func() {
		defer q.reclaimW.Done()
		ticker := time.NewTicker(q.config.ReclaimInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := q.ReclaimExpired(ctx); err != nil && onError != nil {
					// ctx 取消导致的错误属于正常退出，不上报
					if ctx.Err() == nil {
						onError(err)
					}
				}
			}
		}
	}()
}

// StopReclaimer 停止后台回收协程并等待其退出
func (q *Queue) StopReclaimer() {
	q.mu.Lock()
	cancel := q.cancel
	q.cancel = nil
	q.mu.Unlock()

	if cancel != nil {
		cancel()
		q.reclaimW.Wait()
	}
}
