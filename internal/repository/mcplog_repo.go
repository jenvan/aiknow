package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/google/uuid"
)

// MCPLogRepositoryImpl MCP 日志仓库实现
type MCPLogRepositoryImpl struct {
	db *PostgresDB
}

// NewMCPLogRepository 创建 MCP 日志仓库
func NewMCPLogRepository(db *PostgresDB) *MCPLogRepositoryImpl {
	return &MCPLogRepositoryImpl{db: db}
}

// Create 创建 MCP 日志
func (r *MCPLogRepositoryImpl) Create(ctx context.Context, log *model.MCPRequestLog) error {
	if log.ID == "" {
		log.ID = uuid.New().String()
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}

	requestBodyBytes, _ := json.Marshal(log.RequestBody)
	var responseBodyBytes []byte
	if log.ResponseBody != nil {
		responseBodyBytes, _ = json.Marshal(*log.ResponseBody)
	}

	headersBytes, _ := json.Marshal(log.ResponseHeaders)

	_, err := r.db.pool.Exec(ctx, `
		INSERT INTO mcp_request_logs (
			id, trace_id, method, tool_name, request_body, response_body,
			response_headers, error_message, query_time_ms, api_key_id, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, log.ID, log.TraceID, log.Method, log.ToolName, requestBodyBytes, responseBodyBytes,
		headersBytes, log.ErrorMessage, log.QueryTimeMS, log.APIKeyID, log.CreatedAt)

	if err != nil {
		return fmt.Errorf("create mcp log: %w", err)
	}

	return nil
}

// List 查询 MCP 日志列表。
//
// 表中无 status 列，成功/失败由 error_message 是否为空推导，
// 故 query.Status 映射为对该列的 IS NULL / IS NOT NULL 判断。
func (r *MCPLogRepositoryImpl) List(ctx context.Context, query model.MCPLogQuery) ([]model.MCPRequestLog, error) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		query.PageSize = 20
	}

	conditions := []string{"1=1"}
	args := []interface{}{}
	argIdx := 1

	add := func(cond string, val interface{}) {
		conditions = append(conditions, fmt.Sprintf(cond, argIdx))
		args = append(args, val)
		argIdx++
	}

	if query.TraceID != nil && *query.TraceID != "" {
		add("trace_id = $%d", *query.TraceID)
	}
	if query.Method != nil && *query.Method != "" {
		add("method = $%d", *query.Method)
	}
	if query.ToolName != nil && *query.ToolName != "" {
		add("tool_name = $%d", *query.ToolName)
	}
	if query.DateFrom != nil {
		add("created_at >= $%d", *query.DateFrom)
	}
	if query.DateTo != nil {
		add("created_at <= $%d", *query.DateTo)
	}
	if query.Status != nil {
		switch *query.Status {
		case "error":
			conditions = append(conditions, "error_message IS NOT NULL")
		case "success":
			conditions = append(conditions, "error_message IS NULL")
		}
	}
	if query.Keyword != nil && *query.Keyword != "" {
		// 请求与响应体为 JSONB，转 text 后做子串匹配即可满足调试期检索需要
		conditions = append(conditions, fmt.Sprintf(
			"(request_body::text ILIKE $%d OR response_body::text ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+*query.Keyword+"%")
		argIdx++
	}

	offset := (query.Page - 1) * query.PageSize

	rows, err := r.db.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, trace_id, method, tool_name, request_body, response_body,
		       response_headers, error_message, query_time_ms, api_key_id, created_at
		FROM mcp_request_logs
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(conditions, " AND "), argIdx, argIdx+1),
		append(args, query.PageSize, offset)...)
	if err != nil {
		return nil, fmt.Errorf("list mcp logs: %w", err)
	}
	defer rows.Close()

	logs := make([]model.MCPRequestLog, 0)
	for rows.Next() {
		var log model.MCPRequestLog
		var requestBodyBytes, responseBodyBytes, headersBytes []byte

		if err := rows.Scan(&log.ID, &log.TraceID, &log.Method, &log.ToolName,
			&requestBodyBytes, &responseBodyBytes, &headersBytes,
			&log.ErrorMessage, &log.QueryTimeMS, &log.APIKeyID, &log.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan mcp log: %w", err)
		}

		// JSON 解析失败不应中断整页查询，留空即可
		_ = json.Unmarshal(requestBodyBytes, &log.RequestBody)
		if responseBodyBytes != nil {
			var body map[string]interface{}
			if json.Unmarshal(responseBodyBytes, &body) == nil {
				log.ResponseBody = &body
			}
		}
		_ = json.Unmarshal(headersBytes, &log.ResponseHeaders)

		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mcp logs: %w", err)
	}

	return logs, nil
}

// GetByTraceID 根据 trace_id 查询
func (r *MCPLogRepositoryImpl) GetByTraceID(ctx context.Context, traceID string) (*model.MCPRequestLog, error) {
	log := &model.MCPRequestLog{}
	var requestBodyBytes, responseBodyBytes, headersBytes []byte

	err := r.db.pool.QueryRow(ctx, `
		SELECT id, trace_id, method, tool_name, request_body, response_body,
		       response_headers, error_message, query_time_ms, api_key_id, created_at
		FROM mcp_request_logs WHERE trace_id = $1
	`, traceID).Scan(&log.ID, &log.TraceID, &log.Method, &log.ToolName,
		&requestBodyBytes, &responseBodyBytes, &headersBytes,
		&log.ErrorMessage, &log.QueryTimeMS, &log.APIKeyID, &log.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get mcp log: %w", err)
	}

	json.Unmarshal(requestBodyBytes, &log.RequestBody)
	if responseBodyBytes != nil {
		var body map[string]interface{}
		json.Unmarshal(responseBodyBytes, &body)
		log.ResponseBody = &body
	}
	json.Unmarshal(headersBytes, &log.ResponseHeaders)

	return log, nil
}

// DeleteExpired 删除过期日志
func (r *MCPLogRepositoryImpl) DeleteExpired(ctx context.Context, before time.Time) (int, error) {
	result, err := r.db.pool.Exec(ctx, `
		DELETE FROM mcp_request_logs WHERE created_at < $1
	`, before)

	if err != nil {
		return 0, fmt.Errorf("delete expired logs: %w", err)
	}

	return int(result.RowsAffected()), nil
}
