package service

import (
	"context"
	"database/sql"
	"encoding/json"
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

// Worker PGMQ 消息队列消费者
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

	// 启动消费者
	for i := 0; i < w.config.Concurrency; i++ {
		w.wg.Add(1)
		go w.consume(i)
	}

	return nil
}

// Stop 停止 Worker
func (w *Worker) Stop() {
	w.running = false
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
			if err == sql.ErrNoRows {
				// 队列为空，等待后继续轮询
				time.Sleep(w.config.PollInterval)
				continue
			}
			fmt.Printf("worker %d: pop message error: %v\n", workerID, err)
			time.Sleep(5 * time.Second)
			continue
		}

		// 解析消息
		var message map[string]interface{}
		if err := json.Unmarshal(msg.Value, &message); err != nil {
			fmt.Printf("worker %d: unmarshal message error: %v\n", workerID, err)
			// 删除无效消息
			if delErr := w.queue.Delete(w.ctx, msg.MsgID); delErr != nil {
				fmt.Printf("worker %d: delete invalid message error: %v\n", workerID, delErr)
			}
			continue
		}

		// 处理文章
		if err := w.processMessage(w.ctx, workerID, message); err != nil {
			fmt.Printf("worker %d: process message error: %v\n", workerID, err)
			// 递增重试计数
			w.incrementRetryCount(w.ctx, message)
		} else {
			// 成功，归档消息
			if archErr := w.queue.Archive(w.ctx, msg.MsgID); archErr != nil {
				fmt.Printf("worker %d: archive message error: %v\n", workerID, archErr)
			}
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

// incrementRetryCount 递增重试计数
func (w *Worker) incrementRetryCount(ctx context.Context, message map[string]interface{}) {
	// TODO: 实现重试逻辑
	_ = ctx
	_ = message
}

// GetQueue 获取队列
func (w *Worker) GetQueue() *pgqueue.Queue {
	return w.queue
}
