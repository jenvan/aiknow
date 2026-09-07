package repository

import (
	"context"
	//nolint:revive
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migration 表示一个数据库迁移
type Migration struct {
	Version int
	Name    string
	Content string
}

// RunMigrations 执行所有待执行的迁移
func RunMigrations(ctx context.Context, tx pgx.Tx) error {
	// 创建迁移记录表
	_, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	// 获取已执行的迁移版本
	var appliedVersions []int
	rows, err := tx.Query(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return fmt.Errorf("query applied migrations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return fmt.Errorf("scan migration version: %w", err)
		}
		appliedVersions = append(appliedVersions, v)
	}

	// 解析所有迁移文件
	migrations, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	// 执行未应用的迁移
	for _, m := range migrations {
		if versionExists(appliedVersions, m.Version) {
			continue
		}

		fmt.Printf("Applying migration %d: %s\n", m.Version, m.Name)

		// 执行迁移 SQL（分割多条语句）
		statements := splitSQL(m.Content)
		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			// 跳过 CREATE EXTENSION IF NOT EXISTS，因为某些扩展可能不存在
			if strings.Contains(stmt, "CREATE EXTENSION") {
				continue
			}
			if _, err := tx.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("execute migration %s: %w", m.Name, err)
			}
		}

		// 记录迁移
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", m.Version); err != nil {
			return fmt.Errorf("record migration %s: %w", m.Name, err)
		}
	}

	return nil
}

// loadMigrations 加载所有迁移文件
func loadMigrations() ([]Migration, error) {
	var migrations []Migration

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		data, err := migrationFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		// 从文件名提取版本号（001_..., 002_...）
		version := parseMigrationVersion(entry.Name())

		migrations = append(migrations, Migration{
			Version: version,
			Name:    entry.Name(),
			Content: string(data),
		})
	}

	// 按版本号排序
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// parseMigrationVersion 从文件名提取版本号
func parseMigrationVersion(filename string) int {
	// 格式：001_initial_schema.sql
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) == 0 {
		return 0
	}

	var version int
	fmt.Sscanf(parts[0], "%d", &version)
	return version
}

// splitSQL 将 SQL 文件分割成单个语句
func splitSQL(content string) []string {
	var statements []string
	var current strings.Builder
	inString := false
	escapeNext := false

	for _, ch := range content {
		if escapeNext {
			current.WriteRune(ch)
			escapeNext = false
			continue
		}

		if ch == '\\' {
			escapeNext = true
			current.WriteRune(ch)
			continue
		}

		if ch == '\'' && !inString {
			inString = true
			current.WriteRune(ch)
			continue
		}

		if ch == '\'' && inString {
			inString = false
			current.WriteRune(ch)
			continue
		}

		if ch == ';' && !inString {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
			continue
		}

		current.WriteRune(ch)
	}

	// 添加最后一个语句
	if stmt := strings.TrimSpace(current.String()); stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}

// versionExists 检查版本是否已存在
func versionExists(versions []int, target int) bool {
	for _, v := range versions {
		if v == target {
			return true
		}
	}
	return false
}
