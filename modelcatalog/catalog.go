// Package modelcatalog contains provider-neutral metadata for models exposed by
// the daemon. Values are kept here so the provider, configuration, and clients
// cannot silently drift apart.
package modelcatalog

import "strings"

const (
	GPT56ContextWindow int64 = 372_000
	GPT54ContextWindow int64 = 272_000
)

// ContextWindow returns the usable context window for a known model ID. The
// GPT-5.6 values mirror the model catalog distributed by the Codex CLI.
func ContextWindow(model string) int64 {
	model = strings.ToLower(strings.TrimSpace(model))
	for _, prefix := range []string{"openai.", "openai/"} {
		model = strings.TrimPrefix(model, prefix)
	}
	if strings.HasPrefix(model, "gpt-5.6") {
		return GPT56ContextWindow
	}
	if (strings.HasPrefix(model, "gpt-5.5") || strings.HasPrefix(model, "gpt-5.4")) &&
		!strings.Contains(model, "mini") && !strings.Contains(model, "nano") {
		return GPT54ContextWindow
	}
	return 0
}
