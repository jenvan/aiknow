package model

import "time"

// Chunk Chunk 模型
type Chunk struct {
	ID         string    `json:"id" db:"id"`
	ArticleID  string    `json:"article_id" db:"article_id"`
	ChunkIndex int       `json:"chunk_index" db:"chunk_index"`
	Content    string    `json:"content" db:"content"`
	WordCount  int       `json:"word_count" db:"word_count"`
	Embedding  []float32 `json:"embedding,omitempty" db:"embedding"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

// ChunkListByArticle Chunk 列表（按文章）
type ChunkListByArticle struct {
	ArticleID string  `json:"article_id"`
	Chunks    []Chunk `json:"chunks"`
}
