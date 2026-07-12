package agentteam

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/agentloop"
)

type tool struct {
	backend    Backend
	scope      Scope
	definition agent.ToolDefinition
	execute    func(context.Context, map[string]json.RawMessage) (any, error)
}

func Tools(backend Backend, scope Scope) map[string]agentloop.Tool {
	if backend == nil || scope.AgentID == "" {
		return nil
	}
	result := map[string]agentloop.Tool{}
	if scope.CanSpawn {
		result["spawn_agent"] = newSpawnTool(backend, scope)
	}
	result["list_agents"] = newListTool(backend, scope)
	result["send_agent_message"] = newSendTool(backend, scope)
	result["wait_agents"] = newWaitTool(backend, scope)
	result["interrupt_agent"] = newInterruptTool(backend, scope)
	return result
}

func (t *tool) Definition() agent.ToolDefinition { return t.definition }

func (t *tool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	input, err := decodeObject(raw)
	if err != nil {
		return "", err
	}
	result, err := t.execute(ctx, input)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func definition(name, description, schema string) agent.ToolDefinition {
	return agent.ToolDefinition{Name: name, Description: description, Parameters: json.RawMessage(schema)}
}

func newSpawnTool(backend Backend, scope Scope) *tool {
	profiles := make([]string, 0, len(scope.Profiles))
	for name, description := range scope.Profiles {
		profiles = append(profiles, name+": "+description)
	}
	sort.Strings(profiles)
	description := "Spawn a persistent child agent for an independent task. The child inherits this conversation's folder sandbox and cannot gain tools the parent lacks. Set mode to model-router to start the configured controller, which can decompose the request and choose allowlisted worker models."
	if len(profiles) > 0 {
		description += " Profiles: " + strings.Join(profiles, "; ")
	}
	allowedModels := normalizedStrings(scope.AllowedModels)
	if len(allowedModels) > 0 {
		description += " The model must be selected from: " + strings.Join(allowedModels, ", ") + "."
	}
	allowedThinking := normalizedStrings(scope.AllowedThinking)
	if scope.SpawnPolicy != nil {
		description = "Spawn one bounded worker subtask. Choose a distinct name, focused task, allowed model, and appropriate thinking level. The worker profile, role, and tool permissions are fixed by the parent request. Call this tool again for additional independent subtasks."
		if len(allowedModels) > 0 {
			description += " Allowed models: " + strings.Join(allowedModels, ", ") + "."
		}
		if len(allowedThinking) > 0 {
			description += " Thinking levels: " + strings.Join(allowedThinking, ", ") + "."
		}
	}
	return &tool{backend: backend, scope: scope,
		definition: definition("spawn_agent", description, spawnSchema(allowedModels, allowedThinking, scope.RequireModel, scope.SpawnPolicy != nil)),
		execute: func(ctx context.Context, input map[string]json.RawMessage) (any, error) {
			if scope.SpawnPolicy != nil {
				var args struct {
					Name     string `json:"name"`
					Task     string `json:"task"`
					Model    string `json:"model"`
					Thinking string `json:"thinking"`
				}
				if err := remarshal(input, &args); err != nil {
					return nil, err
				}
				request := *scope.SpawnPolicy
				request.ParentID = scope.AgentID
				request.Name, request.Task = args.Name, args.Task
				request.Model, request.Thinking = args.Model, args.Thinking
				request.Tools = cloneOptionalStrings(scope.SpawnPolicy.Tools)
				return backend.SpawnAgent(ctx, request)
			}
			var request SpawnRequest
			if err := remarshal(input, &request); err != nil {
				return nil, err
			}
			request.ParentID = scope.AgentID
			return backend.SpawnAgent(ctx, request)
		},
	}
}

func spawnSchema(allowedModels, allowedThinking []string, requireModel, routedWorker bool) string {
	model := map[string]any{"type": "string"}
	if len(allowedModels) > 0 {
		model["enum"] = allowedModels
	}
	required := []string{"name", "task"}
	if requireModel {
		required = append(required, "model")
	}
	if routedWorker {
		thinking := map[string]any{"type": "string"}
		if len(allowedThinking) > 0 {
			thinking["enum"] = allowedThinking
		}
		required = []string{"name", "task", "model", "thinking"}
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"}, "task": map[string]any{"type": "string"},
				"model": model, "thinking": thinking,
			},
			"required":             required,
			"additionalProperties": false,
		}
		data, _ := json.Marshal(schema)
		return string(data)
	}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":     map[string]any{"type": "string"},
			"mode":     map[string]any{"type": "string", "enum": []string{SpawnModeDirect, SpawnModeModelRouter}},
			"profile":  map[string]any{"type": "string"},
			"role":     map[string]any{"type": "string"},
			"task":     map[string]any{"type": "string"},
			"model":    model,
			"thinking": map[string]any{"type": "string"},
			"tools":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required":             required,
		"additionalProperties": false,
	}
	data, _ := json.Marshal(schema)
	return string(data)
}

func normalizedStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func cloneOptionalStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string{}, values...)
}

func newListTool(backend Backend, scope Scope) *tool {
	return &tool{backend: backend, scope: scope,
		definition: definition("list_agents", "List the parent, children, and sibling agents in this conversation team with their current status.", `{"type":"object","additionalProperties":false}`),
		execute: func(ctx context.Context, _ map[string]json.RawMessage) (any, error) {
			return backend.ListAgents(ctx, scope.AgentID)
		},
	}
}

func newSendTool(backend Backend, scope Scope) *tool {
	return &tool{backend: backend, scope: scope,
		definition: definition("send_agent_message", "Send a durable message to the parent, a child, or a sibling agent. Wake starts an idle recipient or steers a running one.", `{"type":"object","properties":{"agent_id":{"type":"string"},"message":{"type":"string"},"wake":{"type":"boolean","default":true}},"required":["agent_id","message"],"additionalProperties":false}`),
		execute: func(ctx context.Context, input map[string]json.RawMessage) (any, error) {
			var args struct {
				AgentID string `json:"agent_id"`
				Message string `json:"message"`
				Wake    *bool  `json:"wake"`
			}
			if err := remarshal(input, &args); err != nil {
				return nil, err
			}
			wake := args.Wake == nil || *args.Wake
			return backend.SendAgentMessage(ctx, scope.AgentID, args.AgentID, args.Message, wake)
		},
	}
}

func newWaitTool(backend Backend, scope Scope) *tool {
	return &tool{backend: backend, scope: scope,
		definition: definition("wait_agents", "Wait briefly for selected child agents to settle or send messages. Omit agent_ids to watch the whole team.", `{"type":"object","properties":{"agent_ids":{"type":"array","items":{"type":"string"}},"timeout_ms":{"type":"integer","minimum":100,"maximum":60000,"default":30000}},"additionalProperties":false}`),
		execute: func(ctx context.Context, input map[string]json.RawMessage) (any, error) {
			var args struct {
				AgentIDs  []string `json:"agent_ids"`
				TimeoutMS int      `json:"timeout_ms"`
			}
			if err := remarshal(input, &args); err != nil {
				return nil, err
			}
			if args.TimeoutMS == 0 {
				args.TimeoutMS = 30000
			}
			if args.TimeoutMS < 100 || args.TimeoutMS > 60000 {
				return nil, errors.New("agentteam: timeout_ms must be between 100 and 60000")
			}
			return backend.WaitAgents(ctx, scope.AgentID, args.AgentIDs, time.Duration(args.TimeoutMS)*time.Millisecond)
		},
	}
}

func newInterruptTool(backend Backend, scope Scope) *tool {
	return &tool{backend: backend, scope: scope,
		definition: definition("interrupt_agent", "Cancel a running child or sibling agent without deleting its persistent conversation.", `{"type":"object","properties":{"agent_id":{"type":"string"}},"required":["agent_id"],"additionalProperties":false}`),
		execute: func(ctx context.Context, input map[string]json.RawMessage) (any, error) {
			var args struct {
				AgentID string `json:"agent_id"`
			}
			if err := remarshal(input, &args); err != nil {
				return nil, err
			}
			err := backend.InterruptAgent(ctx, scope.AgentID, args.AgentID)
			return map[string]bool{"interrupted": err == nil}, err
		},
	}
}

func decodeObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var value map[string]json.RawMessage
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("agentteam: decode input: %w", err)
	}
	if value == nil {
		return nil, errors.New("agentteam: input must be an object")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("agentteam: input must contain one object")
	}
	return value, nil
}

func remarshal(input map[string]json.RawMessage, destination any) error {
	data, _ := json.Marshal(input)
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(destination)
}
