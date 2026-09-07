package model

import "time"

// MCPRequestLog MCP 请求日志
type MCPRequestLog struct {
	ID                string                 `json:"id" db:"id"`
	TraceID           string                 `json:"trace_id" db:"trace_id"`
	Method            string                 `json:"method" db:"method"`
	ToolName          *string                `json:"tool_name,omitempty" db:"tool_name"`
	RequestBody       map[string]interface{} `json:"request_body" db:"request_body"`
	ResponseBody      *map[string]interface{} `json:"response_body,omitempty" db:"response_body"`
	ResponseHeaders   map[string]string      `json:"response_headers,omitempty" db:"response_headers"`
	ErrorMessage      *string                `json:"error_message,omitempty" db:"error_message"`
	QueryTimeMS       *int                   `json:"query_time_ms,omitempty" db:"query_time_ms"`
	APIKeyID          *string                `json:"api_key_id,omitempty" db:"api_key_id"`
	CreatedAt         time.Time              `json:"created_at" db:"created_at"`
}

// MCPLogQuery MCP 日志查询参数
type MCPLogQuery struct {
	Page      int        `form:"page" default:"1"`
	PageSize  int        `form:"page_size" default:"20"`
	TraceID   *string    `form:"trace_id,omitempty"`
	Method    *string    `form:"method,omitempty"`
	ToolName  *string    `form:"tool_name,omitempty"`
	Keyword   *string    `form:"keyword,omitempty"` // 全文检索
	DateFrom  *time.Time `form:"date_from,omitempty"`
	DateTo    *time.Time `form:"date_to,omitempty"`
	Status    *string    `form:"status,omitempty"` // success | error
}
