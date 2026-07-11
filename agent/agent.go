// Package agent defines provider-neutral building blocks for conversational AI
// agents. Provider implementations live in separate packages.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Provider opens conversational sessions backed by an AI service.
//
// Implementations may own background processes or network connections, so a
// Provider must be closed when it is no longer needed.
type Provider interface {
	OpenSession(context.Context, SessionOptions) (Session, error)
	io.Closer
}

// SessionOptions contains the portable subset of session configuration. A
// provider may apply additional safe defaults of its own.
type SessionOptions struct {
	Model            string
	WorkingDirectory string
	Instructions     string
}

// Session is a stateful conversation with a provider.
type Session interface {
	ID() string
	Run(context.Context, string) (Result, error)
}

// ToolDefinition describes a function tool exposed to a model.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall is a model request to execute a named tool.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolResult is returned to the model after executing a ToolCall.
type ToolResult struct {
	CallID  string `json:"call_id"`
	Output  string `json:"output"`
	IsError bool   `json:"is_error,omitempty"`
}

// ImageInput is an in-memory image attached to a user turn. Keeping the bytes
// provider-neutral lets transports persist uploads inside their own sandbox
// while providers choose the wire representation required by their API.
type ImageInput struct {
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mime_type"`
	Data     []byte `json:"-"`
}

// ModelEvent represents incremental output produced while a model step runs.
type ModelEvent struct {
	Type  string          `json:"type"`
	Delta string          `json:"delta,omitempty"`
	Text  string          `json:"text,omitempty"`
	Item  json.RawMessage `json:"item,omitempty"`
}

// Usage is provider-neutral token accounting for one or more model calls.
// CacheReadTokens and CacheWriteTokens are subsets of input-side activity;
// TotalTokens is the provider-reported input-plus-output total, while
// ContextTokens describes the most recent model context rather than a sum.
type Usage struct {
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	ContextTokens    int64 `json:"context_tokens"`
	ContextWindow    int64 `json:"context_window"`
}

// StepRequest contains new inputs for one model invocation.
type StepRequest struct {
	UserMessages    []string
	Images          []ImageInput
	ToolResults     []ToolResult
	Tools           []ToolDefinition
	ReasoningEffort string
	OnEvent         func(ModelEvent)
}

// StepResult is one model invocation. Tool calls indicate that the agentic
// loop should execute tools and invoke Step again with their results.
type StepResult struct {
	Result
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// StepSession is implemented by sessions that support agentic tool loops.
type StepSession interface {
	Session
	Step(context.Context, StepRequest) (StepResult, error)
}

// SessionState is opaque provider-owned state suitable for persistence.
type SessionState struct {
	ID       string          `json:"id"`
	Provider string          `json:"provider"`
	Data     json.RawMessage `json:"data"`
}

// StatefulSession can snapshot its conversation for later resumption.
type StatefulSession interface {
	Session
	State() (SessionState, error)
}

// StatefulProvider can restore a provider session from persisted state.
type StatefulProvider interface {
	Provider
	OpenSessionFromState(context.Context, SessionOptions, SessionState) (Session, error)
}

// Result is the provider-neutral result of one agent turn.
type Result struct {
	Text      string
	Provider  string
	SessionID string
	TurnID    string
	Usage     Usage `json:"usage"`
}

// Agent gives a small, provider-independent facade over a Session.
type Agent struct {
	session Session
}

// New opens a session and returns an agent that can run multiple conversational
// turns against it.
func New(ctx context.Context, provider Provider, options SessionOptions) (*Agent, error) {
	if provider == nil {
		return nil, fmt.Errorf("agent: provider is nil")
	}

	session, err := provider.OpenSession(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("agent: open session: %w", err)
	}
	if session == nil {
		return nil, fmt.Errorf("agent: provider returned a nil session")
	}

	return &Agent{session: session}, nil
}

// ID returns the provider's identifier for the current session.
func (a *Agent) ID() string {
	if a == nil || a.session == nil {
		return ""
	}
	return a.session.ID()
}

// Run sends one prompt to the agent. The same Agent can be reused for
// follow-up turns.
func (a *Agent) Run(ctx context.Context, prompt string) (Result, error) {
	if a == nil || a.session == nil {
		return Result{}, fmt.Errorf("agent: not initialized")
	}
	if strings.TrimSpace(prompt) == "" {
		return Result{}, fmt.Errorf("agent: prompt is empty")
	}

	result, err := a.session.Run(ctx, prompt)
	if err != nil {
		return Result{}, fmt.Errorf("agent: run turn: %w", err)
	}
	return result, nil
}
