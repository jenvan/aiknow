package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

// List 查询 MCP 日志列表
func (r *MCPLogRepositoryImpl) List(ctx context.Context, query model.MCPLogQuery) ([]model.MCPRequestLog, error) {
	// TODO: 实现完整查询
	return nil, fmt.Errorf("not implemented")
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
		if err == sql.ErrNoRows {
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
