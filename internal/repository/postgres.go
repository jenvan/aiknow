package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// PostgresDB PostgreSQL 数据库封装
type PostgresDB struct {
	pool *pgxpool.Pool
	mu   sync.RWMutex
	// Repos
	ArticleRepo  ArticleRepository
	ChunkRepo    ChunkRepository
	CategoryRepo CategoryRepository
	TagRepo      TagRepository
	SettingRepo  SettingRepository
	APIKeyRepo   APIKeyRepository
	MCPLogRepo   MCPLogRepository
}

// NewPostgresDB 创建数据库连接
func NewPostgresDB(ctx context.Context, dsn string) (*PostgresDB, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}

	// 设置连接池参数
	config.MaxConns = 20
	config.MinConns = 2
	config.MaxConnLifetime = 30 * 60 // 30 分钟
	config.MaxConnIdleTime = 10 * 60 // 10 分钟

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	// 验证连接
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	db := &PostgresDB{pool: pool}
	db.initRepos()
	return db, nil
}

// NewPostgresDBWithPool 从已有连接池创建 PostgresDB（用于测试）
func NewPostgresDBWithPool(pool *pgxpool.Pool) *PostgresDB {
	db := &PostgresDB{pool: pool}
	db.initRepos()
	return db
}

// initRepos 初始化各仓库实例
func (db *PostgresDB) initRepos() {
	db.ArticleRepo = NewArticleRepository(db)
	db.ChunkRepo = NewChunkRepository(db)
	db.CategoryRepo = NewCategoryRepository(db)
	db.TagRepo = NewTagRepository(db)
}

// GetPool 返回连接池（用于迁移执行器）
func (db *PostgresDB) GetPool() *pgxpool.Pool {
	return db.pool
}

// Tx 执行事务
func (db *PostgresDB) Tx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback(ctx)
			panic(r)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("%w (rollback: %v)", err, rbErr)
		}
		return err
	}

	return tx.Commit(ctx)
}

// DB 返回底层 sql.DB（用于 pkg/pgqueue）
func (db *PostgresDB) DB() *sql.DB {
	return stdlib.OpenDBFromPool(db.pool)
}

// Close 关闭连接
func (db *PostgresDB) Close() {
	db.pool.Close()
}

// GenerateUUID 生成 UUID v4
func GenerateUUID() string {
	return uuid.New().String()
}

// GetArticleRepo 获取文章仓库
func (db *PostgresDB) GetArticleRepo() ArticleRepository { return db.ArticleRepo }

// GetChunkRepo 获取 Chunk 仓库
func (db *PostgresDB) GetChunkRepo() ChunkRepository { return db.ChunkRepo }

// GetCategoryRepo 获取分类仓库
func (db *PostgresDB) GetCategoryRepo() CategoryRepository { return db.CategoryRepo }

// GetTagRepo 获取标签仓库
func (db *PostgresDB) GetTagRepo() TagRepository { return db.TagRepo }
