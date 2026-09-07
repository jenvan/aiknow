package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/aiknow-org/aiknow/internal/model"
	"github.com/aiknow-org/aiknow/internal/service"
)

// Server MCP 服务端 Handler
type Server struct {
	mcpService  service.MCPService
	authService service.AuthService
}

// NewServer 创建 MCP 服务端
func NewServer(mcpService service.MCPService, authService service.AuthService) *Server {
	return &Server{
		mcpService:  mcpService,
		authService: authService,
	}
}

// Handle 处理 MCP 请求
func (s *Server) Handle(c *gin.Context) {
	// 验证 API Key
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing API key"})
		c.Abort()
		return
	}

	key, err := s.authService.ValidateKey(c.Request.Context(), authHeader)
	if err != nil || key == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
		c.Abort()
		return
	}

	// 读取请求体
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		c.Abort()
		return
	}

	// 解析 JSON-RPC 请求
	var req model.MCPJSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON-RPC request"})
		c.Abort()
		return
	}

	// 处理请求
	resp, err := s.handleRequest(c.Request.Context(), &req, key.Scope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 设置响应头
	c.Header("MCP-Protocol-Version", "2025-06-18")
	if resp.ID != nil {
		idStr, _ := json.Marshal(string(*resp.ID))
		c.Header("Mcp-Session-Id", string(idStr))
	}

	c.JSON(http.StatusOK, resp)
}

func (s *Server) handleRequest(ctx context.Context, req *model.MCPJSONRPCRequest, scope string) (*model.MCPJSONRPCResponse, error) {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(ctx, req)
	case "tools/list":
		return s.handleToolsList(ctx, req, scope)
	case "tools/call":
		return s.handleToolsCall(ctx, req, scope)
	default:
		return nil, fmt.Errorf("unknown method: %s", req.Method)
	}
}

func (s *Server) handleInitialize(ctx context.Context, req *model.MCPJSONRPCRequest) (*model.MCPJSONRPCResponse, error) {
	var initReq model.MCPInitializeRequest
	if req.Params != nil {
		paramsBytes, _ := json.Marshal(req.Params)
		json.Unmarshal(paramsBytes, &initReq)
	}

	resp, err := s.mcpService.Initialize(ctx, initReq)
	if err != nil {
		return nil, err
	}

	return &model.MCPJSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  resp,
	}, nil
}

func (s *Server) handleToolsList(ctx context.Context, req *model.MCPJSONRPCRequest, scope string) (*model.MCPJSONRPCResponse, error) {
	tools, err := s.mcpService.ListTools(ctx, scope)
	if err != nil {
		return nil, err
	}

	return &model.MCPJSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  tools,
	}, nil
}

func (s *Server) handleToolsCall(ctx context.Context, req *model.MCPJSONRPCRequest, scope string) (*model.MCPJSONRPCResponse, error) {
	var callReq model.MCPCallToolRequest
	if req.Params != nil {
		paramsBytes, _ := json.Marshal(req.Params)
		json.Unmarshal(paramsBytes, &callReq)
	}

	result, err := s.mcpService.CallTool(ctx, callReq.Name, callReq.Arguments, scope)
	if err != nil {
		return &model.MCPJSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &model.MCPError{
				Code:    -32000,
				Message: err.Error(),
			},
		}, nil
	}

	return &model.MCPJSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}, nil
}
