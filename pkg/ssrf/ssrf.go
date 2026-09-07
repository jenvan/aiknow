package ssrf

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// SSRFCheckResult SSRF 检查结果
type SSRFCheckResult struct {
	Allowed   bool
	Reason    string
	IsPrivate bool
}

// Checker SSRF 检查器
type Checker struct {
	allowedPorts []int
	blockedIPs   []*net.IPNet
}

// NewChecker 创建 SSRF 检查器
func NewChecker() *Checker {
	c := &Checker{
		allowedPorts: []int{80, 443, 8080, 8443},
		blockedIPs:   make([]*net.IPNet, 0),
	}

	// 添加私有 IP 段
	c.addBlockedRange("10.0.0.0/8")
	c.addBlockedRange("172.16.0.0/12")
	c.addBlockedRange("192.168.0.0/16")
	c.addBlockedRange("127.0.0.0/8")
	c.addBlockedRange("169.254.169.254/32") // AWS metadata
	c.addBlockedRange("0.0.0.0/8")
	c.addBlockedRange("100.64.0.0/10")

	return c
}

func (c *Checker) addBlockedRange(cidr string) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return
	}
	c.blockedIPs = append(c.blockedIPs, ipNet)
}

// CheckURL 检查 URL 是否安全
func (c *Checker) CheckURL(rawURL string) (*SSRFCheckResult, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return &SSRFCheckResult{Allowed: false, Reason: "invalid URL"}, nil
	}

	// 只允许 http 和 https
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return &SSRFCheckResult{Allowed: false, Reason: fmt.Sprintf("protocol %s not allowed", parsed.Scheme)}, nil
	}

	host := parsed.Hostname()
	port := parsed.Port()

	// 检查端口
	if port != "" {
		portNum, err := strconv.Atoi(port)
		if err != nil {
			return &SSRFCheckResult{Allowed: false, Reason: "invalid port"}, nil
		}

		allowed := false
		for _, p := range c.allowedPorts {
			if p == portNum {
				allowed = true
				break
			}
		}
		if !allowed {
			return &SSRFCheckResult{Allowed: false, Reason: fmt.Sprintf("port %s not allowed", port)}, nil
		}
	}

	// 检查 host 是否为空
	if host == "" {
		return &SSRFCheckResult{Allowed: false, Reason: "empty host"}, nil
	}

	// 解析 IP
	ip := net.ParseIP(host)
	if ip == nil {
		// DNS 解析
		addrs, err := net.LookupIP(host)
		if err != nil {
			return &SSRFCheckResult{Allowed: false, Reason: fmt.Sprintf("DNS resolution failed: %v", err)}, nil
		}

		for _, resolvedIP := range addrs {
			if c.isPrivateIP(resolvedIP) {
				return &SSRFCheckResult{
					Allowed:   false,
					Reason:    fmt.Sprintf("resolved to private IP: %s", resolvedIP),
					IsPrivate: true,
				}, nil
			}
		}
	} else {
		if c.isPrivateIP(ip) {
			return &SSRFCheckResult{
				Allowed:   false,
				Reason:    fmt.Sprintf("private IP address: %s", ip),
				IsPrivate: true,
			}, nil
		}
	}

	return &SSRFCheckResult{Allowed: true}, nil
}

func (c *Checker) isPrivateIP(ip net.IP) bool {
	for _, range_ := range c.blockedIPs {
		if range_.Contains(ip) {
			return true
		}
	}
	return false
}

// SafeError 返回对调用方友好的错误信息（不暴露内部拦截原因）
func SafeError(result *SSRFCheckResult) error {
	if result.Allowed {
		return nil
	}
	return fmt.Errorf("url blocked by SSRF protection")
}

// IsBlocked 检查 URL 是否被拦截
func IsBlocked(result *SSRFCheckResult) bool {
	return !result.Allowed
}

// IsPrivateIPError 判断是否为内网 IP 拦截（用于日志分类）
func IsPrivateIPError(result *SSRFCheckResult) bool {
	return !result.Allowed && result.IsPrivate
}

// String 实现 error 接口
func (r *SSRFCheckResult) Error() string {
	return r.Reason
}

// IsSSRFResult 判断错误是否来自 SSRF 检查
func IsSSRFResult(err error) bool {
	if err == nil {
		return false
	}
	// 检查是否为 SafeError 包装的错误
	return strings.Contains(err.Error(), "url blocked by SSRF protection")
}
