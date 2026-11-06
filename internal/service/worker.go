package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jenvan/aiknow/internal/repository"
	"github.com/jenvan/aiknow/pkg/pgqueue"
)

// WorkerConfig Worker 配置
type WorkerConfig struct {
	Concurrency  int           // 并发消费者数量
	PollInterval time.Duration // 轮询间隔
	MaxRetries   int           // 最大重试次数
}

// DefaultWorkerConfig 默认配置
func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		Concurrency:  5,
		PollInterval: 1 * time.Second,
		MaxRetries:   3,
	}
}

// Worker 任务队列消费者
type Worker struct {
	db       *repository.PostgresDB
	pipeline *AIPipeline
	queue    *pgqueue.Queue
	config   WorkerConfig
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	running  bool

	// onProcessed 在文章处理完成后回调（用于微信入口的处理完成推送等）。
	// 可为 nil；回调失败只记日志，不影响任务归档。
	onProcessed func(ctx context.Context, articleID string)
}

// SetOnProcessed 注册「处理完成」回调。
func (w *Worker) SetOnProcessed(fn func(ctx context.Context, articleID string)) {
	w.onProcessed = fn
}

// NewWorker 创建 Worker
func NewWorker(
	db *repository.PostgresDB,
	pipeline *AIPipeline,
	queue *pgqueue.Queue,
	config WorkerConfig,
) *Worker {
	ctx, cancel := context.WithCancel(context.Background())

	if config.Concurrency <= 0 {
		config.Concurrency = DefaultWorkerConfig().Concurrency
	}
	if config.PollInterval <= 0 {
		config.PollInterval = DefaultWorkerConfig().PollInterval
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = DefaultWorkerConfig().MaxRetries
	}

	return &Worker{
		db:       db,
		pipeline: pipeline,
		queue:    queue,
		ctx:      ctx,
		cancel:   cancel,
		config:   config,
	}
}

// Start 启动 Worker
func (w *Worker) Start() error {
	if w.running {
		return fmt.Errorf("worker already running")
	}

	w.running = true
	w.ctx, w.cancel = context.WithCancel(context.Background())

	// 启动超时任务回收协程：消费者进程崩溃后，租约到期的任务由此重新变为可领取
	w.queue.StartReclaimer(func(err error) {
		fmt.Printf("worker: 回收超时任务失败: %v\n", err)
	})

	// 启动「孤儿文章」收敛协程：任务转死信后文章状态不会自己变，会一直显示
	// 「处理中」。这里兜底把"没有任何在途任务、状态却还停在 pending/processing"
	// 的文章标记为 failed，并给出可读原因。
	w.wg.Add(1)
	go w.sweepOrphanedArticles()

	// 启动消费者
	for i := 0; i < w.config.Concurrency; i++ {
		w.wg.Add(1)
		go w.consume(i)
	}

	return nil
}

// orphanGrace 孤儿文章的宽限：刚入队还没被领走的文章不能被误判。
// 必须大于 队列轮询间隔 + 一次正常处理的排队时间，15 分钟足够宽松。
const orphanGrace = 15 * time.Minute

// sweepOrphanedArticles 周期收敛「卡住但没人管」的文章。
//
// 判定条件：状态是 pending/processing、最后更新时间早于 orphanGrace、
// 且队列里没有它任何在途任务（pending/processing）。三者同时满足说明
// 任务已经转死信或丢失，文章不可能自己恢复。
func (w *Worker) sweepOrphanedArticles() {
	defer w.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(w.ctx, 10*time.Second)
			res, err := w.db.DB().ExecContext(ctx, `
				UPDATE articles a
				SET processing_status = 'failed',
				    last_error = '任务已转死信或丢失，请重试',
				    updated_at = now()
				WHERE a.processing_status IN ('pending', 'processing')
				  AND a.deleted_at IS NULL
				  AND a.updated_at < now() - make_interval(secs => $1::double precision)
				  AND NOT EXISTS (
				      SELECT 1 FROM job_queue j
				      WHERE j.status IN ('pending', 'processing')
				        AND j.payload->>'article_id' = a.id::text
				  )
			`, float64(orphanGrace/time.Second))
			cancel()
			if err != nil {
				if w.ctx.Err() == nil {
					fmt.Printf("worker: 收敛孤儿文章失败: %v\n", err)
				}
				continue
			}
			if n, _ := res.RowsAffected(); n > 0 {
				fmt.Printf("worker: 将 %d 篇卡在处理中的文章标记为 failed\n", n)
			}
		}
	}
}

// Stop 停止 Worker
func (w *Worker) Stop() {
	if !w.running {
		return
	}
	w.running = false
	w.queue.StopReclaimer()
	w.cancel()
	w.wg.Wait()
}

// consume 消费消息
func (w *Worker) consume(workerID int) {
	defer w.wg.Done()

	for {
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		msg, err := w.queue.Pop(w.ctx)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// 队列为空，等待后继续轮询
				select {
				case <-w.ctx.Done():
					return
				case <-time.After(w.config.PollInterval):
				}
				continue
			}
			// ctx 取消导致的错误属于正常退出
			if w.ctx.Err() != nil {
				return
			}
			fmt.Printf("worker %d: 领取消息失败: %v\n", workerID, err)
			select {
			case <-w.ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		// 解析消息
		var message map[string]interface{}
		if err := json.Unmarshal(msg.Value, &message); err != nil {
			fmt.Printf("worker %d: 消息解析失败: %v\n", workerID, err)
			// 载荷无效，重试也不会成功，直接丢弃
			if delErr := w.queue.Delete(w.ctx, msg.MsgID); delErr != nil {
				fmt.Printf("worker %d: 删除无效消息失败: %v\n", workerID, delErr)
			}
			continue
		}

		// 处理文章
		if err := w.processMessage(w.ctx, workerID, message); err != nil {
			fmt.Printf("worker %d: 处理消息 %d 失败（第 %d/%d 次尝试）: %v\n",
				workerID, msg.MsgID, msg.RetryCount, msg.MaxRetries, err)

			// 交由队列判定重试或转入死信
			dead, failErr := w.queue.Fail(w.ctx, msg.MsgID, err)
			if failErr != nil {
				fmt.Printf("worker %d: 标记消息 %d 失败状态出错: %v\n", workerID, msg.MsgID, failErr)
				continue
			}
			if dead {
				fmt.Printf("worker %d: 消息 %d 重试耗尽，已转入死信\n", workerID, msg.MsgID)
				// 队列转死信后必须回写文章状态：流水线开始时已置为 processing，
				// 只有 LLM 阶段失败会自行回写 failed，其余阶段（切分、向量、写库）
				// 出错时无人收尾，文章会永久停留在 processing
				w.markArticleFailed(message, err)
			}
			continue
		}

		// 成功：先通知（如微信回推），再归档
		w.notifyProcessed(message)

		if archErr := w.queue.Archive(w.ctx, msg.MsgID); archErr != nil {
			fmt.Printf("worker %d: 归档消息 %d 失败: %v\n", workerID, msg.MsgID, archErr)
		}
	}
}

// notifyProcessed 在处理成功后回调外部观察者。
//
// 用独立 context：w.ctx 可能已在停止流程中取消，而这条通知恰恰是
// 「告诉用户处理完了」的唯一时机。
func (w *Worker) notifyProcessed(message map[string]interface{}) {
	if w.onProcessed == nil {
		return
	}
	articleID, _ := message["article_id"].(string)
	if articleID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	w.onProcessed(ctx, articleID)
}

// processMessage 处理单条消息。
//
// 载荷只携带 article_id：文章由写入侧同步落库（PRD 要求"提交后立即返回"），
// worker 只负责后续的 AI 加工。此前这里从载荷取 title/content 再调一次
// Create，与写入侧的同步创建重复，且必然撞上内容去重；因为当时没有任何一处
// 入队，这段代码从未真正执行过。
func (w *Worker) processMessage(ctx context.Context, workerID int, message map[string]interface{}) error {
	articleID, _ := message["article_id"].(string)
	if articleID == "" {
		return fmt.Errorf("消息缺少 article_id")
	}

	article, err := w.db.GetArticleRepo().GetByID(ctx, articleID)
	if err != nil {
		return fmt.Errorf("load article %s: %w", articleID, err)
	}
	if article == nil {
		// 文章已被删除，重投也不会成功，直接当作处理完成
		fmt.Printf("worker %d: 文章 %s 不存在或已删除，跳过\n", workerID, articleID)
		return nil
	}

	if err := w.pipeline.Process(ctx, article); err != nil {
		return fmt.Errorf("AI pipeline: %w", err)
	}

	fmt.Printf("worker %d: 文章 %s 处理完成\n", workerID, articleID)
	return nil
}

// markArticleFailed 任务转入死信后把文章状态回写为 failed。
//
// 单独用一个带超时的 context：此处可能在 worker 停止流程中被调用，
// 复用 w.ctx 会因已取消而写不进去，反而留下 processing 状态。
func (w *Worker) markArticleFailed(message map[string]interface{}, cause error) {
	articleID, _ := message["article_id"].(string)
	if articleID == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := w.db.GetArticleRepo()
	article, err := repo.GetByID(ctx, articleID)
	if err != nil || article == nil {
		return
	}

	article.ProcessingStatus = "failed"
	reason := cause.Error()
	article.LastError = &reason
	if err := repo.Update(ctx, article); err != nil {
		fmt.Printf("worker: 回写文章 %s 失败状态出错: %v\n", articleID, err)
	}
}

// GetQueue 获取队列
func (w *Worker) GetQueue() *pgqueue.Queue {
	return w.queue
}
