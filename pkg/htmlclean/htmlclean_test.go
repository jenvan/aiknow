package htmlclean

import (
	"testing"
)

func TestClean(t *testing.T) {
	c := NewCleaner()

	html := `<p>Hello <b>World</b></p>`
	expected := "Hello World"
	result := c.Clean(html)

	if result != expected {
		t.Fatalf("want %q, got %q", expected, result)
	}
}

func TestClean_MoreCases(t *testing.T) {
	c := NewCleaner()

	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name:     "simple tags",
			html:     `<div>Hello World</div>`,
			expected: "Hello World",
		},
		{
			name:     "nested tags",
			html:     `<p><span>Hello</span> <strong>World</strong></p>`,
			expected: "Hello World",
		},
		{
			name:     "entities",
			html:     `&lt;script&gt; &amp; &quot;quotes&quot;`,
			expected: "script & \"quotes\"",
		},
		{
			name:     "multiple spaces",
			html:     `<p>  Hello    World  </p>`,
			expected: "Hello World",
		},
		{
			name:     "empty",
			html:     ``,
			expected: "",
		},
		{
			name:     "no tags",
			html:     `plain text`,
			expected: "plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Clean(tt.html)
			if result != tt.expected {
				t.Fatalf("want %q, got %q", tt.expected, result)
			}
		})
	}
}
