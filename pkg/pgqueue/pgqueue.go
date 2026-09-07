package pgqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Queue PGMQ 队列封装
type Queue struct {
	db   *sql.DB
	name string
}

// Message PGMQ 消息
type Message struct {
	MsgID     string          `json:"message_id"`
	Value     json.RawMessage `json:"value"`
	Delay     int             `json:"delay"`
	GroupID   string          `json:"group_id"`
	CreatedAt time.Time       `json:"created_at"`
}

// NewQueue 创建队列
func NewQueue(db *sql.DB, name string) *Queue {
	return &Queue{db: db, name: name}
}

// CreateQueue 创建队列
func (q *Queue) CreateQueue(ctx context.Context) error {
	_, err := q.db.ExecContext(ctx, "SELECT pgmq.create($1)", q.name)
	return err
}

// Send 发送消息
func (q *Queue) Send(ctx context.Context, message interface{}, delay int) (string, error) {
	data, err := json.Marshal(message)
	if err != nil {
		return "", fmt.Errorf("marshal message: %w", err)
	}

	var msgID string
	err = q.db.QueryRowContext(ctx,
		"SELECT msg_id FROM pgmq.send($1, $2, $3)",
		q.name, data, delay,
	).Scan(&msgID)

	return msgID, err
}

// Pop 消费消息
func (q *Queue) Pop(ctx context.Context) (*Message, error) {
	var msg Message
	err := q.db.QueryRowContext(ctx,
		"SELECT msg_id, value, delay, group_id, created_at FROM pgmq.read($1, $2, 1)",
		q.name, 1,
	).Scan(&msg.MsgID, &msg.Value, &msg.Delay, &msg.GroupID, &msg.CreatedAt)

	if err != nil {
		return nil, err
	}

	return &msg, nil
}

// Archive 归档消息
func (q *Queue) Archive(ctx context.Context, msgID string) error {
	_, err := q.db.ExecContext(ctx,
		"SELECT pgmq.archive($1, $2)", q.name, msgID,
	)
	return err
}

// Delete 删除消息
func (q *Queue) Delete(ctx context.Context, msgID string) error {
	_, err := q.db.ExecContext(ctx,
		"SELECT pgmq.delete($1, $2)", q.name, msgID,
	)
	return err
}

// Depth 查询队列深度
func (q *Queue) Depth(ctx context.Context) (int, error) {
	var depth int
	err := q.db.QueryRowContext(ctx,
		"SELECT vt_count FROM pgmq.view_queue($1)", q.name,
	).Scan(&depth)
	return depth, err
}

// OldestMessageAge 查询最早消息的等待时间
func (q *Queue) OldestMessageAge(ctx context.Context) (time.Duration, error) {
	var oldestMsgID string
	var oldestCreatedAt time.Time

	err := q.db.QueryRowContext(ctx,
		"SELECT msg_id, created_at FROM pgmq.read($1, 'oldest', 1)", q.name,
	).Scan(&oldestMsgID, &oldestCreatedAt)

	if err != nil {
		return 0, err
	}

	return time.Since(oldestCreatedAt), nil
}
