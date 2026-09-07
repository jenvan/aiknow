package model

// SearchDebugRequest 检索调试请求
type SearchDebugRequest struct {
	Query   string              `json:"query"`
	Params  SearchParams        `json:"params"`
}

// KeywordSearchDebugRequest 关键词检索调试请求
type KeywordSearchDebugRequest struct {
	Keywords []string            `json:"keywords"`
	Params   KeywordSearchParams `json:"params"`
}
