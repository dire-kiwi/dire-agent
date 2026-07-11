package mcpclient

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFlattenResultHandlesContentKindsAndBounds(t *testing.T) {
	result := flattenResult(&mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "plain text"},
			&mcp.ImageContent{MIMEType: "image/png", Data: []byte("encoded")},
			&mcp.ResourceLink{Name: "manual", URI: "file:///manual.md"},
		},
		StructuredContent: map[string]any{"large": strings.Repeat("x", 100)},
	}, 200, 30)
	for _, fragment := range []string{"plain text", "[image: image/png", "manual", "Structured content:"} {
		if !strings.Contains(result.Output, fragment) {
			t.Fatalf("output %q does not contain %q", result.Output, fragment)
		}
	}
	if !result.Truncated {
		t.Fatal("structured truncation was not reported")
	}
}

func TestFlattenResultMarksLogicalError(t *testing.T) {
	result := flattenResult(&mcp.CallToolResult{IsError: true}, 100, 50)
	if !result.IsError || result.Output != ErrToolFailed.Error() {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestTruncateUTF8KeepsValidText(t *testing.T) {
	got, truncated := truncateUTF8("ab🙂cd", 4)
	if !truncated || got != "a…" || len(got) > 4 {
		t.Fatalf("truncateUTF8 = %q, %v", got, truncated)
	}
}
