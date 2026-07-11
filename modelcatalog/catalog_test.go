package modelcatalog

import "testing"

func TestContextWindow(t *testing.T) {
	tests := map[string]int64{
		"gpt-5.6":              GPT56ContextWindow,
		"gpt-5.6-sol":          GPT56ContextWindow,
		"openai/gpt-5.6-terra": GPT56ContextWindow,
		"openai.gpt-5.6-luna":  GPT56ContextWindow,
		"gpt-5.5":              GPT54ContextWindow,
		"gpt-5.4-2026-03-05":   GPT54ContextWindow,
		"openai.gpt-5.4-pro":   GPT54ContextWindow,
		"gpt-5.4-mini":         0,
		"gpt-5.4-nano":         0,
		"unknown":              0,
	}
	for model, want := range tests {
		if got := ContextWindow(model); got != want {
			t.Errorf("ContextWindow(%q) = %d, want %d", model, got, want)
		}
	}
}
