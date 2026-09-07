package ssrf

import (
	"testing"
)

func TestCheckURL_Allowed(t *testing.T) {
	c := NewChecker()

	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"http allowed", "http://example.com", true},
		{"https allowed", "https://example.com", true},
		{"http with port", "http://example.com:80", true},
		{"https with port", "https://example.com:443", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := c.CheckURL(tt.url)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Allowed != tt.want {
				t.Fatalf("want %v, got %v", tt.want, result.Allowed)
			}
		})
	}
}

func TestCheckURL_Blocked(t *testing.T) {
	c := NewChecker()

	tests := []struct {
		name string
		url  string
	}{
		{"ftp protocol", "ftp://example.com"},
		{"private IP", "http://192.168.1.1"},
		{"localhost", "http://127.0.0.1"},
		{"invalid port", "http://example.com:9999"},
		{"aws metadata", "http://169.254.169.254/latest/meta-data"},
		{"private 10.x", "http://10.0.0.1"},
		{"private 172.16.x", "http://172.16.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := c.CheckURL(tt.url)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Allowed {
				t.Fatalf("expected blocked, but got allowed")
			}
		})
	}
}

func TestSafeError(t *testing.T) {
	c := NewChecker()

	// Allowed URL should not error
	result, _ := c.CheckURL("https://example.com")
	if SafeError(result) != nil {
		t.Fatal("expected no error for allowed URL")
	}

	// Blocked URL should error with safe message
	result, _ = c.CheckURL("http://192.168.1.1")
	err := SafeError(result)
	if err == nil {
		t.Fatal("expected error for blocked URL")
	}
	if err.Error() != "url blocked by SSRF protection" {
		t.Fatalf("expected safe error message, got: %s", err.Error())
	}
}

func TestIsBlocked(t *testing.T) {
	c := NewChecker()

	result, _ := c.CheckURL("https://example.com")
	if IsBlocked(result) {
		t.Fatal("expected allowed URL not to be blocked")
	}

	result, _ = c.CheckURL("http://127.0.0.1")
	if !IsBlocked(result) {
		t.Fatal("expected blocked URL to be blocked")
	}
}

func TestIsPrivateIPError(t *testing.T) {
	c := NewChecker()

	// Non-private blocked (wrong protocol)
	result, _ := c.CheckURL("ftp://example.com")
	if IsPrivateIPError(result) {
		t.Fatal("expected non-private error not to be flagged as private")
	}

	// Private IP blocked
	result, _ = c.CheckURL("http://192.168.1.1")
	if !IsPrivateIPError(result) {
		t.Fatal("expected private IP error to be flagged")
	}
}

func TestIsSSRFResult(t *testing.T) {
	if IsSSRFResult(nil) {
		t.Fatal("nil error should not be SSRF result")
	}

	if IsSSRFResult(TestError("something else")) {
		t.Fatal("unrelated error should not be SSRF result")
	}

	c := NewChecker()
	result, _ := c.CheckURL("http://192.168.1.1")
	err := SafeError(result)
	if !IsSSRFResult(err) {
		t.Fatal("SafeError should be detected as SSRF result")
	}
}

type TestError string

func (e TestError) Error() string { return string(e) }
