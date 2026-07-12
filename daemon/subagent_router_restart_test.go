package daemon_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dire-kiwi/dire-agent/agentteam"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/daemon"
)

func TestRestoredModelRouterRetainsSchemaAndWorkerPermissionPolicy(t *testing.T) {
	initialProvider := &modelRoutingProvider{}
	fixture := newSubagentFixtureWithProvider(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
		config.Global.Subagents.MaxDepth = 2
		config.Global.Subagents.MaxConcurrent = 3
		config.Global.Subagents.ModelRouting = configuration.SubagentModelRoutingSettings{
			ControllerModel: "gpt-5.6-luna", ControllerThinking: configuration.ThinkingXHigh,
			Prompt: "Route bounded work.", AllowedModels: []string{"gpt-5.6-luna"},
		}
	}, nil, initialProvider)
	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	controller, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "restart-router", Mode: agentteam.SpawnModeModelRouter,
		Profile: "general", Role: "persisted-role", Task: "delegate before and after restart", Tools: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, controller.ID, "completed")
	if err := fixture.manager.Close(); err != nil {
		t.Fatal(err)
	}

	restoredProvider := &modelRoutingProvider{}
	reopened, err := daemon.NewManager(daemon.ManagerConfig{
		Store: fixture.store, Provider: restoredProvider, DefaultCWD: fixture.root,
		DefaultProvider: "fake", DefaultModel: "fake-model", Settings: fixture.settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if _, err := reopened.SendAgentMessage(fixture.ctx, project.ID, controller.ID, "verify restored routing tools", true); err != nil {
		t.Fatal(err)
	}
	waitForAgentStatus(t, fixture.ctx, reopened, controller.ID, "completed")

	var schema struct {
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	if err := json.Unmarshal(restoredProvider.spawnSchema(), &schema); err != nil {
		t.Fatal(err)
	}
	if len(schema.Properties) != 4 || len(schema.Required) != 4 || schema.Properties["model"] == nil || schema.Properties["thinking"] == nil {
		t.Fatalf("restored controller spawn schema = %#v", schema)
	}
	if _, err := reopened.SpawnAgent(context.Background(), agentteam.SpawnRequest{
		ParentID: controller.ID, Name: "forbidden-model", Task: "must fail", Model: "not-allowed", Thinking: "low",
	}); err == nil {
		t.Fatal("restored controller accepted a model outside its allowlist")
	}
	worker, err := reopened.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: controller.ID, Name: "restored-worker", Profile: "review", Role: "attacker",
		Task: "run with persisted restrictions", Model: "gpt-5.6-luna", Thinking: "medium", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := reopened.Thread(fixture.ctx, worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AgentProfile != "general" || stored.AgentRole != "persisted-role" || len(stored.Tools) != 0 || len(stored.AgentTools) != 0 {
		t.Fatalf("restored worker permission policy widened: %#v", stored)
	}
}
