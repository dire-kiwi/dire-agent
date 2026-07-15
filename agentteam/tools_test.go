package agentteam_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/agentteam"
)

func TestToolsSpawnAndMessageWithCallerIdentity(t *testing.T) {
	backend := &fakeBackend{}
	tools := agentteam.Tools(backend, agentteam.Scope{
		AgentID: "project_root", CanSpawn: true,
		Profiles: map[string]string{"explore": "Read-only exploration."},
	})
	if len(tools) != 5 {
		t.Fatalf("tools = %v", tools)
	}
	output, err := tools["spawn_agent"].Execute(context.Background(), json.RawMessage(`{"name":"searcher","profile":"explore","task":"find the code"}`))
	if err != nil || backend.spawn.ParentID != "project_root" || backend.spawn.Name != "searcher" || !json.Valid([]byte(output)) {
		t.Fatalf("spawn = %q request=%+v err=%v", output, backend.spawn, err)
	}
	if _, err := tools["send_agent_message"].Execute(context.Background(), json.RawMessage(`{"agent_id":"agent_1","message":"status?"}`)); err != nil {
		t.Fatal(err)
	}
	if backend.from != "project_root" || backend.to != "agent_1" || !backend.wake {
		t.Fatalf("message routing = %s -> %s wake=%v", backend.from, backend.to, backend.wake)
	}
}

func TestChildWithoutSpawnPermissionCannotCreateGrandchildren(t *testing.T) {
	tools := agentteam.Tools(&fakeBackend{}, agentteam.Scope{AgentID: "agent_1", CanSpawn: false})
	if tools["spawn_agent"] != nil || tools["list_agents"] == nil || tools["send_agent_message"] == nil {
		t.Fatalf("tools = %v", tools)
	}
}

func TestRoutedSpawnToolConstrainsModelThinkingAndPermissions(t *testing.T) {
	backend := &fakeBackend{}
	policy := &agentteam.SpawnRequest{
		Profile: "review", Role: "reviewer", Mode: agentteam.SpawnModeDirect, Tools: []string{"read"},
	}
	tools := agentteam.Tools(backend, agentteam.Scope{
		AgentID: "agent_router", CanSpawn: true,
		AllowedModels: []string{"worker-large", "worker-fast"}, AllowedThinking: []string{"medium", "low"},
		RequireModel: true, SpawnPolicy: policy,
	})
	definition := tools["spawn_agent"].Definition()
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(definition.Parameters, &schema); err != nil {
		t.Fatal(err)
	}
	models := schema.Properties["model"].Enum
	if len(models) != 2 || models[0] != "worker-fast" || models[1] != "worker-large" {
		t.Fatalf("model enum = %v", models)
	}
	if !contains(schema.Required, "model") {
		t.Fatalf("required fields = %v", schema.Required)
	}
	thinking := schema.Properties["thinking"].Enum
	if len(thinking) != 2 || thinking[0] != "low" || thinking[1] != "medium" {
		t.Fatalf("thinking enum = %v", thinking)
	}
	if len(schema.Properties) != 4 || !contains(schema.Required, "thinking") {
		t.Fatalf("router schema = properties %#v required %v", schema.Properties, schema.Required)
	}
	_, err := tools["spawn_agent"].Execute(context.Background(), json.RawMessage(`{"name":"search","task":"find the parser","model":"worker-fast","thinking":"low"}`))
	if err != nil || backend.spawn.Model != "worker-fast" || backend.spawn.Name != "search" || backend.spawn.Task != "find the parser" || backend.spawn.Thinking != "low" {
		t.Fatalf("spawn request = %+v, err = %v", backend.spawn, err)
	}
	if backend.spawn.Profile != "review" || backend.spawn.Role != "reviewer" || len(backend.spawn.Tools) != 1 || backend.spawn.Tools[0] != "read" {
		t.Fatalf("routed permissions were not fixed: %+v", backend.spawn)
	}
	if _, err := tools["spawn_agent"].Execute(context.Background(), json.RawMessage(`{"name":"bad","task":"work","model":"worker-fast","thinking":"low","profile":"general"}`)); err == nil {
		t.Fatal("router spawn tool accepted a profile override")
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

type fakeBackend struct {
	spawn agentteam.SpawnRequest
	from  string
	to    string
	wake  bool
}

func (f *fakeBackend) SpawnAgent(_ context.Context, request agentteam.SpawnRequest) (agentteam.Agent, error) {
	f.spawn = request
	return agentteam.Agent{ID: "agent_1", ParentID: request.ParentID, Name: request.Name, Status: "running"}, nil
}
func (*fakeBackend) ListAgents(context.Context, string) ([]agentteam.Agent, error) {
	return []agentteam.Agent{{ID: "agent_1", Status: "idle"}}, nil
}
func (f *fakeBackend) SendAgentMessage(_ context.Context, from, to, _ string, wake bool) (agentteam.Message, error) {
	f.from, f.to, f.wake = from, to, wake
	return agentteam.Message{ID: "message_1", FromID: from, ToID: to}, nil
}
func (*fakeBackend) WaitAgents(context.Context, string, []string, time.Duration) (agentteam.WaitResult, error) {
	return agentteam.WaitResult{}, nil
}
func (*fakeBackend) InterruptAgent(context.Context, string, string) error { return nil }
