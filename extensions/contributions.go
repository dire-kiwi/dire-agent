package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

type CommandSpec struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	ArgumentHint string `json:"argument_hint,omitempty"`
}

type CommandResult struct {
	Output  string `json:"output,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
	IsError bool   `json:"is_error,omitempty"`
}

type PromptFragment struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Priority int    `json:"priority,omitempty"`
}

type HookEvent string

const (
	HookBeforePrompt HookEvent = "before_prompt"
	HookAfterModel   HookEvent = "after_model"
	HookBeforeTool   HookEvent = "before_tool"
	HookAfterTool    HookEvent = "after_tool"
)

type HookSpec struct {
	ID       string    `json:"id"`
	Event    HookEvent `json:"event"`
	Priority int       `json:"priority,omitempty"`
}

type HookPayload struct {
	Prompt    string          `json:"prompt,omitempty"`
	ModelText string          `json:"model_text,omitempty"`
	ToolName  string          `json:"tool_name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Output    string          `json:"output,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type HookResult struct {
	Veto      bool            `json:"veto,omitempty"`
	Message   string          `json:"message,omitempty"`
	Prompt    *string         `json:"prompt,omitempty"`
	ModelText *string         `json:"model_text,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Output    *string         `json:"output,omitempty"`
	IsError   *bool           `json:"is_error,omitempty"`
}

type UIContribution struct {
	ID     string          `json:"id"`
	Kind   string          `json:"kind"`
	Title  string          `json:"title,omitempty"`
	Schema json.RawMessage `json:"schema,omitempty"`
}

type ToolRenderer struct {
	Tool  string `json:"tool"`
	Style string `json:"style,omitempty"`
	Label string `json:"label,omitempty"`
}

type Registration struct {
	Commands        []CommandSpec    `json:"commands,omitempty"`
	PromptFragments []PromptFragment `json:"prompt_fragments,omitempty"`
	Hooks           []HookSpec       `json:"hooks,omitempty"`
	SettingsSchema  json.RawMessage  `json:"settings_schema,omitempty"`
	UI              []UIContribution `json:"ui,omitempty"`
	ToolRenderers   []ToolRenderer   `json:"tool_renderers,omitempty"`
}

type executeCommandParams struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

type invokeHookParams struct {
	HookID  string      `json:"hook_id"`
	Event   HookEvent   `json:"event"`
	Payload HookPayload `json:"payload"`
}

func (c *Client) Registration() Registration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneRegistration(c.registration)
}

func (c *Client) ListCommands() []CommandSpec {
	registration := c.Registration()
	return registration.Commands
}

func (c *Client) ExecuteCommand(ctx context.Context, name, arguments string) (CommandResult, error) {
	if c.closed.Load() {
		return CommandResult{}, ErrClosed
	}
	if !c.hasCommand(name) {
		return CommandResult{}, fmt.Errorf("extensions: unknown command %q", name)
	}
	requestCtx, cancel := withTimeout(ctx, c.limits.CallTimeout)
	defer cancel()
	var result CommandResult
	err := c.connection.Call(requestCtx, "execute_command", executeCommandParams{Name: name, Arguments: arguments}, &result)
	result.Output = truncate(result.Output, c.limits.MaxOutputBytes)
	result.Prompt = truncate(result.Prompt, c.limits.MaxPromptBytes)
	return result, err
}

func (c *Client) InvokeHooks(ctx context.Context, event HookEvent, payload HookPayload) (HookPayload, *HookResult, error) {
	if !validHookEvent(event) {
		return payload, nil, fmt.Errorf("extensions: unknown hook event %q", event)
	}
	for _, hook := range c.hooksFor(event) {
		requestCtx, cancel := withTimeout(ctx, c.limits.CallTimeout)
		var result HookResult
		err := c.connection.Call(requestCtx, "invoke_hook", invokeHookParams{HookID: hook.ID, Event: event, Payload: payload}, &result)
		cancel()
		if err != nil {
			return payload, nil, err
		}
		applyHookResult(&payload, result, c.limits)
		if result.Veto {
			result.Message = truncate(result.Message, c.limits.MaxOutputBytes)
			return payload, &result, nil
		}
	}
	return payload, nil, nil
}

func (c *Client) hooksFor(event HookEvent) []HookSpec {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var result []HookSpec
	for _, hook := range c.registration.Hooks {
		if hook.Event == event {
			result = append(result, hook)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Priority < result[j].Priority })
	return result
}

func (c *Client) hasCommand(name string) bool {
	for _, command := range c.ListCommands() {
		if command.Name == name {
			return true
		}
	}
	return false
}

func validHookEvent(event HookEvent) bool {
	return event == HookBeforePrompt || event == HookAfterModel || event == HookBeforeTool || event == HookAfterTool
}

func applyHookResult(payload *HookPayload, result HookResult, limits Limits) {
	if result.Prompt != nil {
		payload.Prompt = truncate(*result.Prompt, limits.MaxPromptBytes)
	}
	if result.ModelText != nil {
		payload.ModelText = truncate(*result.ModelText, limits.MaxOutputBytes)
	}
	if len(result.Arguments) > 0 && json.Valid(result.Arguments) && len(result.Arguments) <= limits.MaxMessageBytes {
		payload.Arguments = append(json.RawMessage(nil), result.Arguments...)
	}
	if result.Output != nil {
		payload.Output = truncate(*result.Output, limits.MaxOutputBytes)
	}
	if result.IsError != nil {
		payload.IsError = *result.IsError
	}
}

var errInvalidRegistration = errors.New("extensions: invalid registration")
