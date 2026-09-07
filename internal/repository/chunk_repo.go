package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/pgvector/pgvector-go"
)

// ChunkRepositoryImpl Chunk 仓库实现
type ChunkRepositoryImpl struct {
	db *PostgresDB
}

// NewChunkRepository 创建 Chunk 仓库
func NewChunkRepository(db *PostgresDB) *ChunkRepositoryImpl {
	return &ChunkRepositoryImpl{db: db}
}

// Create 创建单个 Chunk
func (r *ChunkRepositoryImpl) Create(ctx context.Context, chunk *model.Chunk) error {
	chunk.ID = GenerateUUID()
	chunk.CreatedAt = time.Now()

	_, err := r.db.pool.Exec(ctx, `
		INSERT INTO chunks (
			id, article_id, chunk_index, content, word_count,
			embedding, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, chunk.ID, chunk.ArticleID, chunk.ChunkIndex, chunk.Content,
		chunk.WordCount, pgvector.NewVector(chunk.Embedding), chunk.CreatedAt, chunk.CreatedAt)

	if err != nil {
		return fmt.Errorf("insert chunk: %w", err)
	}

	return nil
}

// BulkCreate 批量创建 Chunk
func (r *ChunkRepositoryImpl) BulkCreate(ctx context.Context, chunks []*model.Chunk) error {
	tx, err := r.db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, chunk := range chunks {
		chunk.ID = GenerateUUID()
		chunk.CreatedAt = time.Now()

		_, err := tx.Exec(ctx, `
			INSERT INTO chunks (
				id, article_id, chunk_index, content, word_count,
				embedding, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, chunk.ID, chunk.ArticleID, chunk.ChunkIndex, chunk.Content,
			chunk.WordCount, pgvector.NewVector(chunk.Embedding), chunk.CreatedAt, chunk.CreatedAt)

		if err != nil {
			return fmt.Errorf("insert chunk: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// ListByArticle 查询文章的所有 Chunk
func (r *ChunkRepositoryImpl) ListByArticle(ctx context.Context, articleID string) ([]model.Chunk, error) {
	rows, err := r.db.pool.Query(ctx, `
		SELECT id, article_id, chunk_index, content, word_count, embedding, created_at
		FROM chunks
		WHERE article_id = $1
		ORDER BY chunk_index
	`, articleID)
	if err != nil {
		return nil, fmt.Errorf("query chunks: %w", err)
	}
	defer rows.Close()

	var chunks []model.Chunk
	for rows.Next() {
		var chunk model.Chunk
		var emb pgvector.Vector
		err := rows.Scan(&chunk.ID, &chunk.ArticleID, &chunk.ChunkIndex,
			&chunk.Content, &chunk.WordCount, &emb, &chunk.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan chunk: %w", err)
		}
		chunk.Embedding = emb.Slice()
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// DeleteByArticle 删除文章的所有 Chunk
func (r *ChunkRepositoryImpl) DeleteByArticle(ctx context.Context, articleID string) error {
	_, err := r.db.pool.Exec(ctx,
		"DELETE FROM chunks WHERE article_id = $1", articleID)
	return err
}

// UpdateEmbedding 更新 Chunk 的向量
func (r *ChunkRepositoryImpl) UpdateEmbedding(ctx context.Context, id string, embedding []float32) error {
	_, err := r.db.pool.Exec(ctx, `
		UPDATE chunks SET
			embedding = $1,
			updated_at = now()
		WHERE id = $2
	`, pgvector.NewVector(embedding), id)

	if err != nil {
		return fmt.Errorf("update embedding: %w", err)
	}

	return nil
}

// CountByArticle 统计文章的 Chunk 数量
func (r *ChunkRepositoryImpl) CountByArticle(ctx context.Context, articleID string) (int, error) {
	var count int
	err := r.db.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM chunks WHERE article_id = $1", articleID).Scan(&count)

	if err != nil {
		return 0, fmt.Errorf("count chunks: %w", err)
	}

	return count, nil
}

// GetTotalCount 统计总 Chunk 数量
func (r *ChunkRepositoryImpl) GetTotalCount(ctx context.Context) (int, error) {
	var count int
	err := r.db.pool.QueryRow(ctx, "SELECT COUNT(*) FROM chunks").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("get total count: %w", err)
	}
	return count, nil
}

// GetByID 根据 ID 查询 Chunk
func (r *ChunkRepositoryImpl) GetByID(ctx context.Context, id string) (*model.Chunk, error) {
	chunk := &model.Chunk{}
	var emb pgvector.Vector
	err := r.db.pool.QueryRow(ctx, `
		SELECT id, article_id, chunk_index, content, word_count, embedding, created_at
		FROM chunks
		WHERE id = $1
	`, id).Scan(&chunk.ID, &chunk.ArticleID, &chunk.ChunkIndex,
		&chunk.Content, &chunk.WordCount, &emb, &chunk.CreatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query chunk: %w", err)
	}

	chunk.Embedding = emb.Slice()
	return chunk, nil
}
