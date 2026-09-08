package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jenvan/aiknow/internal/model"
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

	// 启动消费者
	for i := 0; i < w.config.Concurrency; i++ {
		w.wg.Add(1)
		go w.consume(i)
	}

	return nil
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
			}
			continue
		}

		// 成功，归档消息
		if archErr := w.queue.Archive(w.ctx, msg.MsgID); archErr != nil {
			fmt.Printf("worker %d: 归档消息 %d 失败: %v\n", workerID, msg.MsgID, archErr)
		}
	}
}

// processMessage 处理单条消息
func (w *Worker) processMessage(ctx context.Context, workerID int, message map[string]interface{}) error {
	articleService := NewArticleService(
		w.db.GetArticleRepo(),
		w.db.GetChunkRepo(),
		w.db.GetCategoryRepo(),
		w.db.GetTagRepo(),
	)

	// 从消息中构建 ArticleInput
	title, _ := message["title"].(string)
	content, _ := message["content"].(string)
	sourceType, _ := message["source_type"].(string)
	if sourceType == "" {
		sourceType = "manual"
	}

	input := &model.ArticleInput{
		Title:      title,
		Content:    content,
		SourceType: sourceType,
	}

	// 处理可选字段
	if categoryID, ok := message["category_id"].(string); ok && categoryID != "" {
		input.CategoryID = &categoryID
	}
	if categoryName, ok := message["category_name"].(string); ok && categoryName != "" {
		input.CategoryName = &categoryName
	}
	if tags, ok := message["tags"].([]interface{}); ok {
		for _, t := range tags {
			if tag, ok := t.(string); ok {
				input.Tags = append(input.Tags, tag)
			}
		}
	}

	article, err := articleService.Create(ctx, *input)
	if err != nil {
		return fmt.Errorf("create article: %w", err)
	}

	// 运行 AI 处理流水线
	if err := w.pipeline.Process(ctx, article); err != nil {
		return fmt.Errorf("AI pipeline: %w", err)
	}

	return nil
}

// GetQueue 获取队列
func (w *Worker) GetQueue() *pgqueue.Queue {
	return w.queue
}
