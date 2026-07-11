package mcpclient

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Result is the provider-neutral form of an MCP tool result.
type Result struct {
	Output    string `json:"output"`
	IsError   bool   `json:"is_error,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

func flattenResult(result *mcp.CallToolResult, maxBytes, maxStructuredBytes int) Result {
	parts := make([]string, 0, len(result.Content)+1)
	for _, content := range result.Content {
		part := flattenContent(content)
		if part != "" {
			parts = append(parts, part)
		}
	}
	truncated := false
	if result.StructuredContent != nil {
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			parts = append(parts, "[structured content could not be encoded]")
		} else {
			structured, cut := truncateUTF8(string(encoded), maxStructuredBytes)
			truncated = truncated || cut
			parts = append(parts, "Structured content:\n"+structured)
		}
	}
	output := strings.Join(parts, "\n")
	if result.IsError && output == "" {
		output = ErrToolFailed.Error()
	}
	output, cut := truncateUTF8(output, maxBytes)
	return Result{Output: output, IsError: result.IsError, Truncated: truncated || cut}
}

func flattenContent(content mcp.Content) string {
	switch value := content.(type) {
	case *mcp.TextContent:
		if value == nil {
			return "[empty MCP content]"
		}
		return value.Text
	case *mcp.ImageContent:
		if value == nil {
			return "[empty MCP content]"
		}
		return fmt.Sprintf("[image: %s, %d encoded bytes]", value.MIMEType, len(value.Data))
	case *mcp.AudioContent:
		if value == nil {
			return "[empty MCP content]"
		}
		return fmt.Sprintf("[audio: %s, %d encoded bytes]", value.MIMEType, len(value.Data))
	case *mcp.ResourceLink:
		if value == nil {
			return "[empty MCP content]"
		}
		label := value.Name
		if label == "" {
			label = value.Title
		}
		if label == "" {
			return "[resource: " + value.URI + "]"
		}
		return fmt.Sprintf("[resource: %s (%s)]", label, value.URI)
	case nil:
		return "[empty MCP content]"
	default:
		encoded, err := content.MarshalJSON()
		if err != nil {
			return "[MCP content could not be encoded]"
		}
		return string(encoded)
	}
}
