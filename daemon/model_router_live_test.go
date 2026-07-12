//go:build live

package daemon_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/agentteam"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/daemon"
	"github.com/dire-kiwi/dire-agent/provider/codex"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

// TestLiveLunaModelRouterTwoThinkingLevels uses the current Codex CLI login
// and consumes real subscription allowance. One Luna/xhigh controller must
// create two distinct Luna workers at low and medium reasoning effort.
func TestLiveLunaModelRouterTwoThinkingLevels(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	root := t.TempDir()
	defaults := configuration.DefaultConfig(root)
	defaults.Global.Subagents.AutoReport = false
	defaults.Global.Subagents.MaxDepth = 2
	defaults.Global.Subagents.MaxConcurrent = 3
	defaults.Global.Subagents.ModelRouting = configuration.SubagentModelRoutingSettings{
		ControllerModel:    "gpt-5.6-luna",
		ControllerThinking: configuration.ThinkingXHigh,
		Prompt: "For this validation, use gpt-5.6-luna for every worker. " +
			"Use low thinking for a simple exact-response task and medium thinking for a separate exact-response task.",
		AllowedModels: []string{"gpt-5.6-luna"},
	}
	defaults.Global.Subagents.Profiles["general"] = configuration.AgentProfile{
		Description: "General worker with no local tools for live routing validation.",
		Tools:       []string{},
	}
	settings, err := configuration.NewStore(filepath.Join(root, "config.json"), defaults)
	if err != nil {
		t.Fatal(err)
	}
	store, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	provider, err := codex.New(ctx, codex.Config{})
	if err != nil {
		t.Fatalf("start direct Codex provider: %v", err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: provider, DefaultCWD: root,
		DefaultProvider: "codex", DefaultModel: "gpt-5.6-luna", Settings: settings,
	})
	if err != nil {
		_ = provider.Close()
		t.Fatal(err)
	}
	defer func() { _ = manager.Close() }()

	chat, err := manager.CreateChat(ctx, daemon.CreateChatOptions{
		Name: "live-luna-router", Model: "gpt-5.6-luna", ThinkingLevel: "low",
		Instructions: "This is a live routing validation. Follow exact output instructions and do not add commentary.",
	})
	if err != nil {
		t.Fatal(err)
	}
	controller, err := manager.SpawnAgent(ctx, agentteam.SpawnRequest{
		ParentID: chat.ID, Name: "live-luna-pair", Mode: agentteam.SpawnModeModelRouter,
		Profile: "general",
		Task: "Spawn exactly two workers and no others. " +
			"Worker one must be named luna-low, use gpt-5.6-luna with low thinking, and receive the task: Reply with exactly LUNA_LOW_OK and nothing else. " +
			"Worker two must be named luna-medium, use gpt-5.6-luna with medium thinking, and receive the task: Reply with exactly LUNA_MEDIUM_OK and nothing else.",
		Tools: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	controllerThread, err := manager.Thread(ctx, controller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if controllerThread.Model != "gpt-5.6-luna" || controllerThread.ThinkingLevel != "xhigh" {
		t.Fatalf("controller model/thinking = %s/%s", controllerThread.Model, controllerThread.ThinkingLevel)
	}

	workers := waitForLiveRoutedWorkers(t, ctx, manager, chat.ID, controller.ID, 2)
	low := workers["luna-low"]
	medium := workers["luna-medium"]
	if low.ID == "" || medium.ID == "" {
		t.Fatalf("controller workers = %#v", workers)
	}
	for _, expected := range []struct {
		agent    agentteam.Agent
		thinking string
		output   string
	}{
		{agent: low, thinking: "low", output: "LUNA_LOW_OK"},
		{agent: medium, thinking: "medium", output: "LUNA_MEDIUM_OK"},
	} {
		thread := waitForLiveAgentCompletion(t, ctx, manager, expected.agent.ID)
		if thread.Model != "gpt-5.6-luna" || thread.ThinkingLevel != expected.thinking {
			t.Fatalf("worker %s model/thinking = %s/%s", expected.agent.Name, thread.Model, thread.ThinkingLevel)
		}
		messages, messagesErr := manager.Messages(ctx, expected.agent.ID, 0, 200)
		if messagesErr != nil {
			t.Fatal(messagesErr)
		}
		if !hasLiveAssistantOutput(messages, expected.output) {
			t.Fatalf("worker %s did not return %s: %#v", expected.agent.Name, expected.output, messages)
		}
	}
	waitForLiveAgentCompletion(t, ctx, manager, controller.ID)

	all, err := manager.ListAgents(ctx, chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 { // root + controller + two workers
		t.Fatalf("live routed team size = %d, agents=%#v", len(all), all)
	}
}

func waitForLiveRoutedWorkers(t *testing.T, ctx context.Context, manager *daemon.Manager, rootID, controllerID string, count int) map[string]agentteam.Agent {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		agents, err := manager.ListAgents(ctx, rootID)
		if err != nil {
			t.Fatal(err)
		}
		workers := make(map[string]agentteam.Agent)
		for _, candidate := range agents {
			if candidate.ParentID == controllerID {
				workers[candidate.Name] = candidate
			}
		}
		if len(workers) >= count {
			return workers
		}
		select {
		case <-ctx.Done():
			t.Fatalf("controller created %d/%d workers: %v", len(workers), count, ctx.Err())
		case <-ticker.C:
		}
	}
}

func waitForLiveAgentCompletion(t *testing.T, ctx context.Context, manager *daemon.Manager, id string) threadstore.Thread {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		thread, err := manager.Thread(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		switch thread.Status {
		case "completed":
			return thread
		case "failed", "interrupted":
			t.Fatalf("worker %s ended with status %s", id, thread.Status)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("worker %s did not complete: %v", id, ctx.Err())
		case <-ticker.C:
		}
	}
}

func hasLiveAssistantOutput(messages []threadstore.Message, expected string) bool {
	for _, message := range messages {
		if message.Role == "assistant" && strings.TrimSpace(message.Content) == expected {
			return true
		}
	}
	return false
}
