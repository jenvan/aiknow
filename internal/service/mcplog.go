package service

import (
	"context"
	"time"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/repository"
)

// MCPLogServiceImpl MCP 日志服务实现
type MCPLogServiceImpl struct {
	mcpLogRepo repository.MCPLogRepository
}

// NewMCPLogService 创建 MCP 日志服务
func NewMCPLogService(mcpLogRepo repository.MCPLogRepository) *MCPLogServiceImpl {
	return &MCPLogServiceImpl{mcpLogRepo: mcpLogRepo}
}

// CreateLog 创建 MCP 调用日志
func (s *MCPLogServiceImpl) CreateLog(ctx context.Context, log *model.MCPRequestLog) error {
	if log.TraceID == "" {
		log.TraceID = generateTraceID()
	}
	log.CreatedAt = time.Now()
	return s.mcpLogRepo.Create(ctx, log)
}

// GetByTraceID 根据 trace_id 查询日志
func (s *MCPLogServiceImpl) GetByTraceID(ctx context.Context, traceID string) (*model.MCPRequestLog, error) {
	return s.mcpLogRepo.GetByTraceID(ctx, traceID)
}

// ListLogs 查询日志列表
func (s *MCPLogServiceImpl) ListLogs(ctx context.Context, query model.MCPLogQuery) ([]model.MCPRequestLog, error) {
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	return s.mcpLogRepo.List(ctx, query)
}

// DeleteExpired 清理过期日志
func (s *MCPLogServiceImpl) DeleteExpired(ctx context.Context, before time.Time) (int, error) {
	return s.mcpLogRepo.DeleteExpired(ctx, before)
}
