package model

// SearchParams 混合检索参数
type SearchParams struct {
	TopK         int     `json:"top_k"`
	CandidateK   int     `json:"candidate_k"`
	RRFK         int     `json:"rrf_k"`
	VectorWeight float64 `json:"vector_weight"`
	BM25Weight   float64 `json:"bm25_weight"`
	EFSearch     int     `json:"ef_search"`
}

// SearchResult 混合检索结果
type SearchResult struct {
	Total   int       `json:"total"`
	Articles []Article `json:"articles"`
}

// KeywordSearchParams 关键词检索参数
type KeywordSearchParams struct {
	TopK            int     `json:"top_k"`
	TagWeight       float64 `json:"tag_weight"`
	CategoryWeight  float64 `json:"category_weight"`
	TitleWeight     float64 `json:"title_weight"`
	SummaryWeight   float64 `json:"summary_weight"`
	ContentWeight   float64 `json:"content_weight"`
}

// KeywordSearchResult 关键词检索结果
type KeywordSearchResult struct {
	Total    int       `json:"total"`
	Articles []Article `json:"articles"`
}

// SearchDebugResult 检索调试结果（分阶段信息）
type SearchDebugResult struct {
	SearchParams SearchParams       `json:"search_params"`
	VectorResult *SearchResult      `json:"vector_result,omitempty"`
	BM25Result   *SearchResult      `json:"bm25_result,omitempty"`
	RerankResult *SearchResult      `json:"rerank_result,omitempty"`
	Keywords     *KeywordSearchResult `json:"keyword_result,omitempty"`
}

// MCPInitializeRequest MCP initialize 请求
type MCPInitializeRequest struct {
	ClientInfo  MCPClientInfo  `json:"clientInfo"`
	Protocol    string         `json:"protocol"`
	Capabilities MCPClientCap `json:"capabilities"`
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
	Protocol     string            `json:"protocol"`
	ServerInfo   MCPServerInfo     `json:"serverInfo"`
	Capabilities MCPServerCap     `json:"capabilities"`
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
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	Scope       string          `json:"scope"`
}

// MCPTOOLResult MCP 工具调用结果
type MCPTOOLResult struct {
	Content []map[string]interface{} `json:"content"`
	IsError bool                     `json:"isError,omitempty"`
}
