package daemon

import (
	"reflect"
	"testing"

	"github.com/dire-kiwi/dire-agent/agentteam"
)

func TestSpawnRequestFromCommandIncludesModeAndCompatibilityFallbacks(t *testing.T) {
	command := Command{
		Name:      "router",
		Mode:      agentteam.SpawnModeModelRouter,
		Profile:   "review",
		AgentRole: "controller",
		Message:   "choose a model and delegate",
		Model:     "controller-model",
		Level:     "high",
		Tools:     []string{"read", "spawn_agent"},
	}
	want := agentteam.SpawnRequest{
		ParentID: "parent_1",
		Name:     "router",
		Mode:     agentteam.SpawnModeModelRouter,
		Profile:  "review",
		Role:     "controller",
		Task:     "choose a model and delegate",
		Model:    "controller-model",
		Thinking: "high",
		Tools:    []string{"read", "spawn_agent"},
	}
	if got := spawnRequestFromCommand(command, "parent_1"); !reflect.DeepEqual(got, want) {
		t.Fatalf("spawnRequestFromCommand() = %#v, want %#v", got, want)
	}

	command.ParentID = "explicit_parent"
	command.AgentName = "explicit_name"
	command.Task = "explicit task"
	got := spawnRequestFromCommand(command, "fallback_parent")
	if got.ParentID != "explicit_parent" || got.Name != "explicit_name" || got.Task != "explicit task" {
		t.Fatalf("explicit fields did not take precedence: %#v", got)
	}
}
