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
			html:     `<p>&lt;script&gt; &amp; &quot;quotes&quot;</p>`,
			expected: "<script> & \"quotes\"",
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

func TestClean_RemoveNoise(t *testing.T) {
	c := NewCleaner()
	html := `<html><body><nav>skip nav</nav><article><p>Main content</p></article><footer>skip footer</footer></body></html>`
	result := c.Clean(html)
	if result != "Main content" {
		t.Fatalf("want 'Main content', got %q", result)
	}
}

func TestClean_BlockSpacing(t *testing.T) {
	c := NewCleaner()
	html := `<h1>Title</h1><p>Paragraph one.</p><p>Paragraph two.</p>`
	result := c.Clean(html)
	expected := "Title\nParagraph one.\nParagraph two."
	if result != expected {
		t.Fatalf("want %q, got %q", expected, result)
	}
}
