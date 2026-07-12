package client

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/dire-kiwi/dire-agent/agentteam"
	"github.com/dire-kiwi/dire-agent/daemon"
)

func TestSpawnAgentCommandIncludesMode(t *testing.T) {
	request := agentteam.SpawnRequest{
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
	want := daemon.Command{
		Type:           "spawn_agent",
		ConversationID: "parent_1",
		ParentID:       "parent_1",
		AgentName:      "router",
		AgentRole:      "controller",
		Profile:        "review",
		Task:           "choose a model and delegate",
		Model:          "controller-model",
		Level:          "high",
		Mode:           agentteam.SpawnModeModelRouter,
		Tools:          []string{"read", "spawn_agent"},
	}
	if got := spawnAgentCommand(request); !reflect.DeepEqual(got, want) {
		t.Fatalf("spawnAgentCommand() = %#v, want %#v", got, want)
	}
}

func TestSpawnAgentCommandPreservesExplicitEmptyToolsOnWire(t *testing.T) {
	command := spawnAgentCommand(agentteam.SpawnRequest{
		ParentID: "parent_1", Name: "no-tools", Task: "stay sandboxed", Tools: []string{},
	})
	data, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	var decoded daemon.Command
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Tools == nil || len(decoded.Tools) != 0 {
		t.Fatalf("explicit empty tools changed across wire round trip: json=%s tools=%#v", data, decoded.Tools)
	}
}
