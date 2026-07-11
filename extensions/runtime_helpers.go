package extensions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"
)

func validateTool(tool ToolSpec) error {
	if tool.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	if len(tool.InputSchema) == 0 {
		tool.InputSchema = json.RawMessage(`{"type":"object"}`)
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
		return fmt.Errorf("tool %q has invalid input schema: %w", tool.Name, err)
	}
	if kind, _ := schema["type"].(string); kind != "object" {
		return fmt.Errorf("tool %q input schema must have type object", tool.Name)
	}
	return nil
}

func cloneTool(tool ToolSpec) ToolSpec {
	tool.InputSchema = append(json.RawMessage(nil), tool.InputSchema...)
	if len(tool.InputSchema) == 0 {
		tool.InputSchema = json.RawMessage(`{"type":"object"}`)
	}
	return tool
}

func truncate(value string, maximum int) string {
	if maximum <= 0 || len(value) <= maximum {
		return value
	}
	suffix := "\n[output truncated]"
	limit := maximum - len(suffix)
	if limit <= 0 {
		return suffix[:maximum]
	}
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit] + suffix
}

func withTimeout(ctx context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, duration)
}
