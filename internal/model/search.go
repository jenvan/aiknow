package model

import "encoding/json"

// SearchParams 混合检索参数
type SearchParams struct {
	TopK         int     `json:"top_k"`
	CandidateK   int     `json:"candidate_k"`
	RRFK         int     `json:"rrf_k"`
	VectorWeight float64 `json:"vector_weight"`
	BM25Weight   float64 `json:"bm25_weight"`
	EFSearch     int     `json:"ef_search"`
}

// SearchChunk 检索分块结果
//
// 不含 embedding 字段：向量属内部数据，调用方只需 content 与得分。
// 曾经带过该字段并由 VectorSearch 填充，1024 维浮点数被原样序列化，
// 单条结果多出约 4KB，且 omitempty 对非空切片不生效，无法靠标签规避。
type SearchChunk struct {
	ID         string  `json:"id"`
	ArticleID  string  `json:"article_id"`
	ChunkIndex int     `json:"chunk_index"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
	RRFScore   float64 `json:"rrf_score,omitempty"`
	VectorRank int     `json:"vector_rank,omitempty"`
	BM25Rank   int     `json:"bm25_rank,omitempty"`
}

// SearchTimings 检索各阶段耗时（毫秒）
type SearchTimings struct {
	EmbeddingMs int `json:"embedding_ms"`
	VectorMs    int `json:"vector_ms"`
	BM25Ms      int `json:"bm25_ms"`
	RRFMs       int `json:"rrf_ms"`
	RerankMs    int `json:"rerank_ms"`
	TotalMs     int `json:"total_ms"`
}

// SearchResult 混合检索结果
type SearchResult struct {
	Query   string         `json:"query"`
	Results []SearchChunk  `json:"results"`
	Timings SearchTimings  `json:"timings"`
}

// KeywordSearchParams 关键词检索参数
type KeywordSearchParams struct {
	TopK           int     `json:"top_k"`
	TagWeight      float64 `json:"tag_weight"`
	CategoryWeight float64 `json:"category_weight"`
	TitleWeight    float64 `json:"title_weight"`
	SummaryWeight  float64 `json:"summary_weight"`
	ContentWeight  float64 `json:"content_weight"`
}

// KeywordSearchResult 关键词检索结果
type KeywordSearchResult struct {
	Total    int       `json:"total"`
	Articles []Article `json:"articles"`
}

// SearchDebugResult 检索调试结果（分阶段信息）
type SearchDebugResult struct {
	SearchParams SearchParams         `json:"search_params"`
	VectorResult *SearchResult        `json:"vector_result,omitempty"`
	BM25Result   *SearchResult        `json:"bm25_result,omitempty"`
	RerankResult *SearchResult        `json:"rerank_result,omitempty"`
	Keywords     *KeywordSearchResult `json:"keyword_result,omitempty"`
}

// SearchRequest 混合检索请求
type SearchRequest struct {
	Query  string            `json:"query" validate:"required"`
	Params SearchParams      `json:"params"`
}

// KeywordSearchRequest 关键词检索请求
type KeywordSearchRequest struct {
	Keywords []string          `json:"keywords" validate:"required,min=1"`
	Params   KeywordSearchParams `json:"params"`
}

// MCPInitializeRequest MCP initialize 请求
type MCPInitializeRequest struct {
	ClientInfo   MCPClientInfo `json:"clientInfo"`
	Protocol     string        `json:"protocol"`
	Capabilities MCPClientCap  `json:"capabilities"`
}

// MCPClientInfo 客户端信息
type MCPClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPClientCap 客户端能力
type MCPClientCap struct{}

// MCPInitializeResponse MCP initialize 响应
type MCPInitializeResponse struct {
	Protocol     string        `json:"protocol"`
	ServerInfo   MCPServerInfo `json:"serverInfo"`
	Capabilities MCPServerCap  `json:"capabilities"`
}

// MCPServerInfo 服务端信息
type MCPServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPServerCap 服务端能力
type MCPServerCap struct{}

// MCPTOOL MCP 工具定义
type MCPTOOL struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	Scope       string                 `json:"scope"`
}

// MCPCapabilities MCP 服务端能力声明
type MCPCapabilities struct {
	Tools map[string]interface{} `json:"tools"`
}

// MCPContentItem MCP 工具结果的内容项
type MCPContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// MCPTOOLResult MCP 工具调用结果
type MCPTOOLResult struct {
	Content []MCPContentItem `json:"content"`
	IsError bool             `json:"isError,omitempty"`
}

// MCPJSONRPCRequest JSON-RPC 2.0 请求
type MCPJSONRPCRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
	ID      *json.RawMessage       `json:"id,omitempty"`
}

// MCPJSONRPCResponse JSON-RPC 2.0 响应
type MCPJSONRPCResponse struct {
	JSONRPC string             `json:"jsonrpc"`
	Result  interface{}        `json:"result,omitempty"`
	Error   *MCPError          `json:"error,omitempty"`
	ID      *json.RawMessage   `json:"id,omitempty"`
}

// MCPError JSON-RPC 错误
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCPCallToolRequest tools/call 请求参数
type MCPCallToolRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}
