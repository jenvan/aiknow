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
//
// 支持三种不应被 ';' 切分的上下文：
//   - 单引号字符串 'foo;bar'（'' 为转义）
//   - 美元引用块 $$...$$ / $tag$...$tag$（PL/pgSQL 函数体必需）
//   - 行注释 -- 与块注释 /* */
func splitSQL(content string) []string {
	var statements []string
	var current strings.Builder

	runes := []rune(content)
	var dollarTag string // 非空表示正处于美元引用块内

	flush := func() {
		if stmt := strings.TrimSpace(current.String()); stmt != "" {
			statements = append(statements, stmt)
		}
		current.Reset()
	}

	for i := 0; i < len(runes); i++ {
		ch := runes[i]

		// 美元引用块内：只寻找结束标记
		if dollarTag != "" {
			if ch == '$' {
				if tag, ok := matchDollarTag(runes, i); ok && tag == dollarTag {
					current.WriteString(tag)
					i += len([]rune(tag)) - 1
					dollarTag = ""
					continue
				}
			}
			current.WriteRune(ch)
			continue
		}

		switch {
		// 行注释：整行原样保留
		case ch == '-' && i+1 < len(runes) && runes[i+1] == '-':
			for i < len(runes) && runes[i] != '\n' {
				current.WriteRune(runes[i])
				i++
			}
			if i < len(runes) {
				current.WriteRune('\n')
			}

		// 块注释
		case ch == '/' && i+1 < len(runes) && runes[i+1] == '*':
			current.WriteString("/*")
			i += 2
			for i < len(runes) && !(runes[i] == '*' && i+1 < len(runes) && runes[i+1] == '/') {
				current.WriteRune(runes[i])
				i++
			}
			current.WriteString("*/")
			i++

		// 单引号字符串（'' 表示一个字面量单引号）
		case ch == '\'':
			current.WriteRune(ch)
			i++
			for i < len(runes) {
				current.WriteRune(runes[i])
				if runes[i] == '\'' {
					if i+1 < len(runes) && runes[i+1] == '\'' {
						i++
						current.WriteRune(runes[i])
						i++
						continue
					}
					break
				}
				i++
			}

		// 美元引用开始
		case ch == '$':
			if tag, ok := matchDollarTag(runes, i); ok {
				current.WriteString(tag)
				i += len([]rune(tag)) - 1
				dollarTag = tag
			} else {
				current.WriteRune(ch)
			}

		case ch == ';':
			flush()

		default:
			current.WriteRune(ch)
		}
	}

	flush()
	return statements
}

// matchDollarTag 尝试在 runes[start] 处匹配美元引用标记（$$ 或 $tag$）。
// 标记体须为空或合法标识符（首字符非数字），否则视为普通 '$'（如 $1 占位符）。
func matchDollarTag(runes []rune, start int) (string, bool) {
	if start >= len(runes) || runes[start] != '$' {
		return "", false
	}
	for j := start + 1; j < len(runes); j++ {
		c := runes[j]
		if c == '$' {
			return string(runes[start : j+1]), true
		}
		isLetter := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		isDigit := c >= '0' && c <= '9'
		if !isLetter && !(isDigit && j > start+1) {
			return "", false
		}
	}
	return "", false
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
