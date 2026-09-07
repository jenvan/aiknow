package service

import (
	"context"
	"fmt"
)

// authServiceImpl 认证服务
type authServiceImpl struct{}

// NewAuthService 创建认证服务
func NewAuthService() *authServiceImpl {
	return &authServiceImpl{}
}

// Login 登录
func (s *authServiceImpl) Login(ctx context.Context, username, password string) (string, error) {
	return "", fmt.Errorf("auth service not implemented")
}

// Logout 登出
func (s *authServiceImpl) Logout(ctx context.Context, sessionID string) error {
	return fmt.Errorf("auth service not implemented")
}

// GetSession 获取 Session
func (s *authServiceImpl) GetSession(ctx context.Context, sessionID string) (string, error) {
	return "", fmt.Errorf("auth service not implemented")
}
