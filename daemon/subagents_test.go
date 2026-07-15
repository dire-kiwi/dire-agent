package daemon_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/agentloop"
	"github.com/dire-kiwi/dire-agent/agentteam"
	"github.com/dire-kiwi/dire-agent/capability"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/daemon"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

func TestSubagentPersistsMetadataInSeparateSQLiteAndStaysOutOfTopLevelLists(t *testing.T) {
	fixture := newSubagentFixture(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
	}, nil)

	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		Name: "parent", CWD: fixture.root, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "reader", Profile: "general", Role: "repository reader",
		Task: "read the input file",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, child.ID, "completed")

	storedChild, err := fixture.manager.Thread(fixture.ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedChild.ParentID != project.ID || storedChild.RootID != project.ID || storedChild.Depth != 1 {
		t.Fatalf("child ancestry = parent %q root %q depth %d", storedChild.ParentID, storedChild.RootID, storedChild.Depth)
	}
	if project.CreatedAt.IsZero() || storedChild.CreatedAt.IsZero() {
		t.Fatalf("creation timestamps were not preserved: parent=%v child=%v", project.CreatedAt, storedChild.CreatedAt)
	}
	if storedChild.AgentName != "reader" || storedChild.AgentRole != "repository reader" || storedChild.AgentProfile != "general" {
		t.Fatalf("child agent metadata = %#v", storedChild)
	}
	if storedChild.CWD != project.CWD || storedChild.ResourceKind() != threadstore.KindProject {
		t.Fatalf("child project scope = kind %q cwd %q, parent cwd %q", storedChild.ResourceKind(), storedChild.CWD, project.CWD)
	}
	if len(storedChild.AgentTools) != 1 || storedChild.AgentTools[0] != "read" || len(storedChild.Tools) != 1 || storedChild.Tools[0] != "read" {
		t.Fatalf("child tools = agent %v local %v", storedChild.AgentTools, storedChild.Tools)
	}

	parentPath := filepath.Join(fixture.store.Directory(), project.ID+".db")
	childPath := filepath.Join(fixture.store.Directory(), child.ID+".db")
	if parentPath == childPath {
		t.Fatal("parent and child resolved to the same SQLite path")
	}
	for _, path := range []string{parentPath, childPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("SQLite file %s: %v", path, err)
		}
	}
	parentDB, err := fixture.store.Open(fixture.ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer parentDB.Close()
	childDB, err := fixture.store.Open(fixture.ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer childDB.Close()
	parentState, err := parentDB.LoadState(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	childState, err := childDB.LoadState(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if parentState.SessionID == "" || childState.SessionID == "" || parentState.SessionID == childState.SessionID {
		t.Fatalf("provider sessions are not separated: parent=%q child=%q", parentState.SessionID, childState.SessionID)
	}
	childMessages, err := childDB.Messages(fixture.ctx, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !containsStoredMessage(childMessages, "user", "read the input file") || !containsStoredMessage(childMessages, "assistant", "stored value") {
		t.Fatalf("child transcript = %#v", childMessages)
	}
	parentMessages, err := parentDB.Messages(fixture.ctx, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if containsStoredMessage(parentMessages, "user", "read the input file") {
		t.Fatalf("child task leaked into parent transcript: %#v", parentMessages)
	}

	allStored, err := fixture.store.List(fixture.ctx)
	if err != nil || len(allStored) != 2 {
		t.Fatalf("raw store resources = %#v, %v", allStored, err)
	}
	projects, err := fixture.manager.ListProjects(fixture.ctx)
	if err != nil || len(projects) != 1 || projects[0].ID != project.ID {
		t.Fatalf("top-level projects = %#v, %v", projects, err)
	}
	conversations, err := fixture.manager.ListConversations(fixture.ctx)
	if err != nil || len(conversations) != 1 || conversations[0].ID != project.ID {
		t.Fatalf("top-level conversations = %#v, %v", conversations, err)
	}
	agents, err := fixture.manager.ListAgents(fixture.ctx, project.ID)
	if err != nil || len(agents) != 2 || !containsAgent(agents, project.ID) || !containsAgent(agents, child.ID) {
		t.Fatalf("team agents = %#v, %v", agents, err)
	}

	if err := fixture.manager.Close(); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{}
	reopened, err := daemon.NewManager(daemon.ManagerConfig{
		Store: fixture.store, Provider: provider, DefaultCWD: fixture.root,
		DefaultModel: "fake-model", Settings: fixture.settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restored, err := reopened.Thread(fixture.ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ParentID != project.ID || restored.RootID != project.ID || restored.Status != "completed" || provider.restored.Load() != 1 {
		t.Fatalf("restored child/provider state = %#v / %d", restored, provider.restored.Load())
	}
}

func TestSubagentDepthAndToolNarrowing(t *testing.T) {
	fixture := newSubagentFixture(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
		config.Global.Subagents.MaxDepth = 2
		config.Global.Subagents.Profiles = map[string]configuration.AgentProfile{
			"delegate": {
				Description: "A delegating read-only agent.", Thinking: configuration.ThinkingLow,
				Tools: []string{"read"}, CanSpawn: true,
			},
		}
	}, nil)
	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", Tools: []string{"read", "grep"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "escalation", Profile: "delegate", Task: "try to widen tools", Tools: []string{"grep"},
	}); err == nil || !strings.Contains(err.Error(), "not granted") {
		t.Fatalf("tool widening error = %v", err)
	}
	child, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "level-one", Profile: "delegate", Task: "first level", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	childThread, err := fixture.manager.Thread(fixture.ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if childThread.Depth != 1 || childThread.RootID != project.ID || len(childThread.AgentTools) != 1 || childThread.AgentTools[0] != "read" {
		t.Fatalf("level-one metadata = %#v", childThread)
	}
	grandchild, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: child.ID, Name: "level-two", Profile: "delegate", Task: "second level",
	})
	if err != nil {
		t.Fatal(err)
	}
	grandchildThread, err := fixture.manager.Thread(fixture.ctx, grandchild.ID)
	if err != nil {
		t.Fatal(err)
	}
	if grandchildThread.ParentID != child.ID || grandchildThread.RootID != project.ID || grandchildThread.Depth != 2 {
		t.Fatalf("level-two ancestry = %#v", grandchildThread)
	}
	if len(grandchildThread.AgentTools) != 1 || grandchildThread.AgentTools[0] != "read" {
		t.Fatalf("level-two tools widened: %v", grandchildThread.AgentTools)
	}
	if _, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: grandchild.ID, Name: "too-deep", Profile: "delegate", Task: "third level",
	}); err == nil || !strings.Contains(err.Error(), "depth limit 2") {
		t.Fatalf("depth-limit error = %v", err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, child.ID, "completed")
	waitForAgentStatus(t, fixture.ctx, fixture.manager, grandchild.ID, "completed")
	removed := []string{}
	if _, err := fixture.manager.UpdateSettings(fixture.ctx, project.ID, daemon.SettingsUpdate{Tools: &removed}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: child.ID, Name: "revoked-tool", Profile: "delegate", Task: "must remain narrowed", Tools: []string{"read"},
	}); err == nil || !strings.Contains(err.Error(), "not granted") {
		t.Fatalf("ancestor tool revocation was not propagated: %v", err)
	}
}

func TestSubagentConcurrencyWaitAndInterrupt(t *testing.T) {
	resolver := newBlockingReadResolver()
	fixture := newSubagentFixture(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
		config.Global.Subagents.MaxConcurrent = 1
		config.Global.Subagents.Profiles = map[string]configuration.AgentProfile{
			"blocked": {Description: "A blocked reader.", Tools: []string{"read"}},
		}
	}, resolver)
	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "blocked", Profile: "blocked", Task: "remain running",
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-resolver.started:
	case <-fixture.ctx.Done():
		t.Fatal("child never entered the blocking tool")
	}

	waited, err := fixture.manager.WaitAgents(fixture.ctx, project.ID, []string{child.ID}, 40*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !waited.TimedOut || len(waited.Agents) != 1 || waited.Agents[0].Status != "running" {
		t.Fatalf("wait timeout result = %#v", waited)
	}
	if _, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "over-limit", Profile: "blocked", Task: "must not start",
	}); err == nil || !strings.Contains(err.Error(), "maximum 1 concurrent") {
		t.Fatalf("concurrency-limit error = %v", err)
	}
	if err := fixture.manager.InterruptAgent(fixture.ctx, project.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	result, err := fixture.manager.WaitAgents(fixture.ctx, project.ID, []string{child.ID}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result.TimedOut || len(result.Agents) != 1 || result.Agents[0].Status != "interrupted" {
		t.Fatalf("interrupt wait result = %#v", result)
	}
	waitForCompletionEvent(t, fixture.ctx, fixture.manager, child.ID, "interrupted")
}

func TestSubagentMessagingRoutesParentChildAndRejectsSiblings(t *testing.T) {
	fixture := newSubagentFixture(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
		config.Global.Subagents.AllowSiblingMessages = false
	}, nil)
	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "first", Profile: "general", Task: "first task",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "second", Profile: "general", Task: "second task",
	})
	if err != nil {
		t.Fatal(err)
	}

	down, err := fixture.manager.SendAgentMessage(fixture.ctx, project.ID, first.ID, "message from parent", false)
	if err != nil {
		t.Fatal(err)
	}
	firstWait, err := fixture.manager.WaitAgents(fixture.ctx, first.ID, nil, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstWait.Messages) != 1 || firstWait.Messages[0].ID != down.ID || firstWait.Messages[0].FromID != project.ID {
		t.Fatalf("parent-to-child mailbox = %#v", firstWait)
	}
	up, err := fixture.manager.SendAgentMessage(fixture.ctx, first.ID, project.ID, "message from child", false)
	if err != nil {
		t.Fatal(err)
	}
	parentWait, err := fixture.manager.WaitAgents(fixture.ctx, project.ID, []string{first.ID, second.ID}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(parentWait.Messages) != 1 || parentWait.Messages[0].ID != up.ID || parentWait.Messages[0].FromID != first.ID {
		t.Fatalf("child-to-parent mailbox = %#v", parentWait)
	}
	if _, err := fixture.manager.SendAgentMessage(fixture.ctx, first.ID, second.ID, "sibling message", false); err == nil || !strings.Contains(err.Error(), "sibling agent communication is disabled") {
		t.Fatalf("sibling route error = %v", err)
	}

	firstMessages, err := fixture.manager.Messages(fixture.ctx, first.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !containsStoredKind(firstMessages, "agent_message", "message from parent") {
		t.Fatalf("child durable messages = %#v", firstMessages)
	}
	parentMessages, err := fixture.manager.Messages(fixture.ctx, project.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !containsStoredKind(parentMessages, "agent_message", "message from child") {
		t.Fatalf("parent durable messages = %#v", parentMessages)
	}
}

func TestSubagentMessagingAllowsSiblingsWakesIdleRecipientAndRejectsInvalidTeams(t *testing.T) {
	fixture := newSubagentFixture(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
		config.Global.Subagents.AllowSiblingMessages = true
	}, nil)
	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "first-sibling", Profile: "general", Task: "first task",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "second-sibling", Profile: "general", Task: "second task",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, first.ID, "completed")
	waitForAgentStatus(t, fixture.ctx, fixture.manager, second.ID, "completed")

	delivered, err := fixture.manager.SendAgentMessage(fixture.ctx, first.ID, second.ID, "coordinate on the shared result", true)
	if err != nil {
		t.Fatal(err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, second.ID, "completed")
	waited, err := fixture.manager.WaitAgents(fixture.ctx, second.ID, nil, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(waited.Messages) != 1 || waited.Messages[0].ID != delivered.ID || waited.Messages[0].FromID != first.ID || waited.Messages[0].ToID != second.ID {
		t.Fatalf("allowed sibling mailbox = %#v", waited)
	}
	secondMessages, err := fixture.manager.Messages(fixture.ctx, second.ID, 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	if !containsStoredKind(secondMessages, "agent_message", "coordinate on the shared result") ||
		!containsStoredMessage(secondMessages, "user", "Message from first-sibling") {
		t.Fatalf("woken sibling transcript = %#v", secondMessages)
	}
	firstEvents, err := fixture.manager.Events(fixture.ctx, first.ID, 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	secondEvents, err := fixture.manager.Events(fixture.ctx, second.ID, 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	if !containsEventType(firstEvents, "agent_message_sent") || !containsEventType(secondEvents, "agent_message") {
		t.Fatalf("sibling message events missing: source=%#v target=%#v", firstEvents, secondEvents)
	}
	if _, err := fixture.manager.SendAgentMessage(fixture.ctx, first.ID, first.ID, "self message", false); err == nil || !strings.Contains(err.Error(), "cannot target itself") {
		t.Fatalf("self-message error = %v", err)
	}

	otherRoot := t.TempDir()
	other, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: otherRoot, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.manager.SendAgentMessage(fixture.ctx, first.ID, other.ID, "cross-team message", false); err == nil || !strings.Contains(err.Error(), "cross-team") {
		t.Fatalf("cross-team message error = %v", err)
	}
}

func TestSubagentMessageWakesRunningRecipientThroughSteeringQueue(t *testing.T) {
	resolver := newBlockingReadResolver()
	fixture := newSubagentFixture(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
		config.Global.Subagents.MaxConcurrent = 1
		config.Global.Subagents.Profiles = map[string]configuration.AgentProfile{
			"blocked": {Description: "A blocked reader.", Tools: []string{"read"}},
		}
	}, resolver)
	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "running-recipient", Profile: "blocked", Task: "remain running",
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-resolver.started:
	case <-fixture.ctx.Done():
		t.Fatal("recipient never entered the blocking tool")
	}
	message, err := fixture.manager.SendAgentMessage(fixture.ctx, project.ID, child.ID, "steer the active child", true)
	if err != nil {
		t.Fatal(err)
	}
	events, err := fixture.manager.Events(fixture.ctx, child.ID, 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	if !containsEventType(events, "queue_update") {
		t.Fatalf("running recipient did not queue steering: %#v", events)
	}
	resolver.Unblock()
	waitForAgentStatus(t, fixture.ctx, fixture.manager, child.ID, "completed")
	waited, err := fixture.manager.WaitAgents(fixture.ctx, child.ID, nil, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(waited.Messages) != 1 || waited.Messages[0].ID != message.ID {
		t.Fatalf("running recipient mailbox = %#v", waited)
	}
	messages, err := fixture.manager.Messages(fixture.ctx, child.ID, 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	if !containsStoredKind(messages, "agent_message", "steer the active child") {
		t.Fatalf("running recipient message was not durable: %#v", messages)
	}
}

func TestSubagentAutoReportsCompletionToParent(t *testing.T) {
	fixture := newSubagentFixture(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = true
	}, nil)
	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "reporter", Profile: "general", Task: "produce a report",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, child.ID, "completed")
	message := waitForAgentMessage(t, fixture.ctx, fixture.manager, project.ID, child.ID)
	if message.FromID != child.ID || message.ToID != project.ID {
		t.Fatalf("auto-report route = %#v", message)
	}
	if !strings.Contains(message.Content, child.ID) || !strings.Contains(message.Content, "completed") || !strings.Contains(message.Content, "tool returned") {
		t.Fatalf("auto-report content = %q", message.Content)
	}
	events, err := fixture.manager.Events(fixture.ctx, project.ID, 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	if !containsCompletionEvent(events, "completed") {
		t.Fatalf("parent events missing child completion: %#v", events)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, project.ID, "idle")
}

func TestModelRouterCreatesControllerAndEnforcesWorkerPolicy(t *testing.T) {
	fixture := newSubagentFixture(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
		config.Global.Subagents.MaxDepth = 2
		config.Global.Subagents.ModelRouting = configuration.SubagentModelRoutingSettings{
			ControllerModel:    "gpt-5.6-luna",
			ControllerThinking: configuration.ThinkingXHigh,
			Prompt:             "Use low thinking for searches and medium thinking for implementation.",
			AllowedModels:      []string{"gpt-5.6-luna"},
		}
	}, nil)
	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		Name: "parent", CWD: fixture.root, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	controller, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "implement-feature", Mode: agentteam.SpawnModeModelRouter,
		Profile: "general", Role: "implementer", Task: "read input.txt and implement the feature",
		Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	controllerThread, err := fixture.manager.Thread(fixture.ctx, controller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if controllerThread.Model != "gpt-5.6-luna" || controllerThread.ThinkingLevel != "xhigh" || controllerThread.AgentProfile != configuration.ModelRouterControllerProfile || controllerThread.Depth != 1 {
		t.Fatalf("controller metadata = %#v", controllerThread)
	}
	if controllerThread.AgentName != "implement-feature-router" || len(controllerThread.Tools) != 0 {
		t.Fatalf("controller identity/tools = name %q tools %v", controllerThread.AgentName, controllerThread.Tools)
	}
	if len(controllerThread.AgentTools) != 1 || controllerThread.AgentTools[0] != "read" {
		t.Fatalf("controller did not preserve worker permission envelope: %v", controllerThread.AgentTools)
	}
	if controllerThread.ModelRouterPolicy == nil || controllerThread.ModelRouterPolicy.Profile != "general" || controllerThread.ModelRouterPolicy.Role != "implementer" || len(controllerThread.ModelRouterPolicy.Tools) != 1 {
		t.Fatalf("controller worker policy = %#v", controllerThread.ModelRouterPolicy)
	}
	for _, expected := range []string{
		"gpt-5.6-luna", "low thinking for searches", "independent subtasks",
	} {
		if !strings.Contains(controllerThread.Instructions, expected) {
			t.Fatalf("controller instructions missing %q: %s", expected, controllerThread.Instructions)
		}
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, controller.ID, "completed")
	messages, err := fixture.manager.Messages(fixture.ctx, controller.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !containsStoredMessage(messages, "user", "read input.txt and implement the feature") {
		t.Fatalf("controller task did not preserve original work: %#v", messages)
	}

	if _, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: controller.ID, Name: "worker", Profile: "general", Task: "do work", Thinking: "low",
	}); err == nil || !strings.Contains(err.Error(), "must choose a worker model") {
		t.Fatalf("missing routed model error = %v", err)
	}
	if _, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: controller.ID, Name: "worker", Profile: "general", Task: "do work", Model: "unapproved-model", Thinking: "low",
	}); err == nil || !strings.Contains(err.Error(), "is not allowed") {
		t.Fatalf("disallowed routed model error = %v", err)
	}
	if _, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: controller.ID, Name: "worker", Task: "do work", Model: "gpt-5.6-luna",
	}); err == nil || !strings.Contains(err.Error(), "must choose a worker thinking level") {
		t.Fatalf("missing routed thinking error = %v", err)
	}
	worker, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: controller.ID, Name: "search-low", Profile: "review", Role: "attacker",
		Task: "read input.txt", Model: "gpt-5.6-luna", Thinking: "low", Tools: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	workerThread, err := fixture.manager.Thread(fixture.ctx, worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	if workerThread.ParentID != controller.ID || workerThread.RootID != project.ID || workerThread.Depth != 2 || workerThread.Model != "gpt-5.6-luna" {
		t.Fatalf("worker metadata = %#v", workerThread)
	}
	if workerThread.AgentName != "search-low" || workerThread.AgentProfile != "general" || workerThread.AgentRole != "implementer" || workerThread.ThinkingLevel != "low" {
		t.Fatalf("worker policy was not enforced: %#v", workerThread)
	}
	if len(workerThread.Tools) != 1 || workerThread.Tools[0] != "read" {
		t.Fatalf("worker tools = %v", workerThread.Tools)
	}
	second, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: controller.ID, Name: "implement-medium", Task: "implement another task",
		Model: "gpt-5.6-luna", Thinking: "medium",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondThread, err := fixture.manager.Thread(fixture.ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if secondThread.AgentName != "implement-medium" || secondThread.ThinkingLevel != "medium" || secondThread.Model != "gpt-5.6-luna" || secondThread.ParentID != controller.ID {
		t.Fatalf("second routed worker = %#v", secondThread)
	}
}

func TestModelRouterRejectsInvalidModeAndInsufficientDepth(t *testing.T) {
	fixture := newSubagentFixture(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
		config.Global.Subagents.MaxDepth = 1
	}, nil)
	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "bad-mode", Mode: "automatic", Task: "work",
	}); err == nil || !strings.Contains(err.Error(), "invalid subagent spawn mode") {
		t.Fatalf("invalid mode error = %v", err)
	}
	if _, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "preselected", Mode: agentteam.SpawnModeModelRouter, Task: "work", Model: "worker-fast",
	}); err == nil || !strings.Contains(err.Error(), "request model must be empty") {
		t.Fatalf("preselected routed model error = %v", err)
	}
	if _, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "router", Mode: agentteam.SpawnModeModelRouter, Task: "work",
	}); err == nil || !strings.Contains(err.Error(), "needs two levels") {
		t.Fatalf("router depth error = %v", err)
	}
}

func TestLunaXHighModelRouterAutomaticallySpawnsLowAndMediumTasks(t *testing.T) {
	provider := &modelRoutingProvider{}
	fixture := newSubagentFixtureWithProvider(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
		config.Global.Subagents.MaxConcurrent = 3
		config.Global.Subagents.ModelRouting = configuration.SubagentModelRoutingSettings{
			ControllerModel:    "gpt-5.6-luna",
			ControllerThinking: configuration.ThinkingXHigh,
			Prompt:             "Start a low-thinking lookup task and a medium-thinking synthesis task on Luna.",
			AllowedModels:      []string{"gpt-5.6-luna"},
		}
	}, nil, provider)
	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", ThinkingLevel: "low", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	controller, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "luna-pair", Mode: agentteam.SpawnModeModelRouter,
		Profile: "general", Task: "Run two different tasks: inspect the input, then synthesize a separate summary.", Tools: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	controllerThread, err := fixture.manager.Thread(fixture.ctx, controller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if controllerThread.Model != "gpt-5.6-luna" || controllerThread.ThinkingLevel != "xhigh" {
		t.Fatalf("controller model/thinking = %s/%s", controllerThread.Model, controllerThread.ThinkingLevel)
	}
	workers := map[string]agentteam.Agent{}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for len(workers) < 2 {
		agents, listErr := fixture.manager.ListAgents(fixture.ctx, project.ID)
		if listErr != nil {
			t.Fatal(listErr)
		}
		for _, candidate := range agents {
			if candidate.ParentID == controller.ID {
				workers[candidate.Name] = candidate
			}
		}
		if len(workers) == 2 {
			break
		}
		select {
		case <-fixture.ctx.Done():
			t.Fatal("routing controller never spawned its worker")
		case <-ticker.C:
		}
	}
	low, lowOK := workers["luna-low"]
	medium, mediumOK := workers["luna-medium"]
	if !lowOK || !mediumOK || low.Model != "gpt-5.6-luna" || medium.Model != "gpt-5.6-luna" {
		t.Fatalf("automatically routed workers = %#v", workers)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, low.ID, "completed")
	waitForAgentStatus(t, fixture.ctx, fixture.manager, medium.ID, "completed")
	waitForAgentStatus(t, fixture.ctx, fixture.manager, controller.ID, "completed")
	lowThread, err := fixture.manager.Thread(fixture.ctx, low.ID)
	if err != nil {
		t.Fatal(err)
	}
	mediumThread, err := fixture.manager.Thread(fixture.ctx, medium.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lowThread.ThinkingLevel != "low" || mediumThread.ThinkingLevel != "medium" {
		t.Fatalf("worker thinking = low:%q medium:%q", lowThread.ThinkingLevel, mediumThread.ThinkingLevel)
	}
	lowMessages, err := fixture.manager.Messages(fixture.ctx, low.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	mediumMessages, err := fixture.manager.Messages(fixture.ctx, medium.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !containsStoredMessage(lowMessages, "user", "inspect the first distinct task") ||
		!containsStoredMessage(mediumMessages, "user", "synthesize the second distinct task") {
		t.Fatalf("worker tasks were not distinct: low=%#v medium=%#v", lowMessages, mediumMessages)
	}
	schema := provider.spawnSchema()
	var definition struct {
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	if err := json.Unmarshal(schema, &definition); err != nil {
		t.Fatal(err)
	}
	if len(definition.Properties) != 4 || definition.Properties["model"] == nil || definition.Properties["thinking"] == nil || len(definition.Required) != 4 {
		t.Fatalf("controller spawn schema = %s", schema)
	}
	if effort := provider.effort("luna-pair-router"); effort != "xhigh" {
		t.Fatalf("controller reasoning effort = %q", effort)
	}
	if effort := provider.effort("luna-low"); effort != "low" {
		t.Fatalf("low worker reasoning effort = %q", effort)
	}
	if effort := provider.effort("luna-medium"); effort != "medium" {
		t.Fatalf("medium worker reasoning effort = %q", effort)
	}
}

type subagentFixture struct {
	ctx      context.Context
	root     string
	store    *threadstore.Store
	settings *configuration.Store
	manager  *daemon.Manager
}

func newSubagentFixture(t *testing.T, configure func(*configuration.Config), resolver capability.Resolver) *subagentFixture {
	return newSubagentFixtureWithProvider(t, configure, resolver, &fakeProvider{})
}

func newSubagentFixtureWithProvider(t *testing.T, configure func(*configuration.Config), resolver capability.Resolver, provider agent.StatefulProvider) *subagentFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "input.txt"), []byte("stored value"), 0o600); err != nil {
		t.Fatal(err)
	}
	defaults := configuration.DefaultConfig(root)
	defaults.Global.Model.Provider = "fake"
	defaults.Global.Model.ID = "fake-model"
	defaults.Global.Tools.Enabled = []string{"read", "grep", "find", "ls"}
	if configure != nil {
		configure(&defaults)
	}
	settings, err := configuration.NewStore(filepath.Join(root, "config.json"), defaults)
	if err != nil {
		t.Fatal(err)
	}
	store, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: provider, DefaultCWD: root,
		DefaultProvider: "fake", DefaultModel: "fake-model", Settings: settings, Capabilities: resolver,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if unblocker, ok := resolver.(interface{ Unblock() }); ok {
			unblocker.Unblock()
		}
		_ = manager.Close()
	})
	return &subagentFixture{ctx: ctx, root: root, store: store, settings: settings, manager: manager}
}

type modelRoutingProvider struct {
	mu           sync.Mutex
	next         int
	routerSchema json.RawMessage
	efforts      map[string]string
}

func (p *modelRoutingProvider) OpenSession(_ context.Context, options agent.SessionOptions) (agent.Session, error) {
	p.mu.Lock()
	p.next++
	id := fmt.Sprintf("routing-%d", p.next)
	if p.efforts == nil {
		p.efforts = make(map[string]string)
	}
	p.mu.Unlock()
	name := subagentNameFromInstructions(options.Instructions)
	if strings.Contains(options.Instructions, "You are a model-routing controller") {
		return &modelRoutingSession{id: id, name: name, provider: p}, nil
	}
	if name != "" {
		return &modelRoutingWorkerSession{id: id, name: name, provider: p}, nil
	}
	return &fakeSession{id: id}, nil
}

func (p *modelRoutingProvider) OpenSessionFromState(_ context.Context, _ agent.SessionOptions, state agent.SessionState) (agent.Session, error) {
	if state.Provider == "routing" {
		return &modelRoutingSession{id: state.ID, provider: p, spawned: true}, nil
	}
	if state.Provider == "routing-worker" {
		return &modelRoutingWorkerSession{id: state.ID, provider: p}, nil
	}
	return &fakeSession{id: state.ID}, nil
}

func (*modelRoutingProvider) Close() error { return nil }

func (p *modelRoutingProvider) spawnSchema() json.RawMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append(json.RawMessage(nil), p.routerSchema...)
}

func (p *modelRoutingProvider) recordEffort(name, effort string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.efforts == nil {
		p.efforts = make(map[string]string)
	}
	p.efforts[name] = effort
}

func (p *modelRoutingProvider) effort(name string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.efforts[name]
}

func subagentNameFromInstructions(instructions string) string {
	const marker = `You are subagent "`
	start := strings.Index(instructions, marker)
	if start < 0 {
		return ""
	}
	rest := instructions[start+len(marker):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return rest[:end]
}

type modelRoutingSession struct {
	id       string
	name     string
	provider *modelRoutingProvider
	mu       sync.Mutex
	spawned  bool
}

func (s *modelRoutingSession) ID() string { return s.id }

func (s *modelRoutingSession) Run(ctx context.Context, prompt string) (agent.Result, error) {
	step, err := s.Step(ctx, agent.StepRequest{UserMessages: []string{prompt}})
	return step.Result, err
}

func (s *modelRoutingSession) Step(_ context.Context, request agent.StepRequest) (agent.StepResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.provider.recordEffort(s.name, request.ReasoningEffort)
	for _, tool := range request.Tools {
		if tool.Name == "spawn_agent" {
			s.provider.mu.Lock()
			s.provider.routerSchema = append(json.RawMessage(nil), tool.Parameters...)
			s.provider.mu.Unlock()
			break
		}
	}
	if len(request.ToolResults) != 0 || s.spawned {
		return agent.StepResult{Result: agent.Result{Text: "routing complete", SessionID: s.id, TurnID: "router-final"}}, nil
	}
	s.spawned = true
	return agent.StepResult{
		Result: agent.Result{SessionID: s.id, TurnID: "router-spawn"},
		ToolCalls: []agent.ToolCall{
			{ID: "call-low", Name: "spawn_agent", Arguments: json.RawMessage(`{"name":"luna-low","task":"inspect the first distinct task","model":"gpt-5.6-luna","thinking":"low"}`)},
			{ID: "call-medium", Name: "spawn_agent", Arguments: json.RawMessage(`{"name":"luna-medium","task":"synthesize the second distinct task","model":"gpt-5.6-luna","thinking":"medium"}`)},
		},
	}, nil
}

func (s *modelRoutingSession) State() (agent.SessionState, error) {
	return agent.SessionState{ID: s.id, Provider: "routing", Data: json.RawMessage(`{}`)}, nil
}

type modelRoutingWorkerSession struct {
	id       string
	name     string
	provider *modelRoutingProvider
}

func (s *modelRoutingWorkerSession) ID() string { return s.id }

func (s *modelRoutingWorkerSession) Run(ctx context.Context, prompt string) (agent.Result, error) {
	step, err := s.Step(ctx, agent.StepRequest{UserMessages: []string{prompt}})
	return step.Result, err
}

func (s *modelRoutingWorkerSession) Step(_ context.Context, request agent.StepRequest) (agent.StepResult, error) {
	s.provider.recordEffort(s.name, request.ReasoningEffort)
	return agent.StepResult{Result: agent.Result{
		Text: "completed " + s.name, SessionID: s.id, TurnID: "worker-final",
	}}, nil
}

func (s *modelRoutingWorkerSession) State() (agent.SessionState, error) {
	return agent.SessionState{ID: s.id, Provider: "routing-worker", Data: json.RawMessage(`{}`)}, nil
}

type blockingReadResolver struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func newBlockingReadResolver() *blockingReadResolver {
	return &blockingReadResolver{started: make(chan struct{}), release: make(chan struct{})}
}

func (r *blockingReadResolver) Resolve(_ context.Context, scope capability.Scope) (capability.Snapshot, error) {
	tools := make(map[string]agentloop.Tool)
	for _, name := range scope.Builtins {
		if name == "read" {
			tools[name] = blockingReadTool{resolver: r}
		}
	}
	return capability.Snapshot{Tools: tools, Commands: map[string]capability.Command{}}, nil
}

func (r *blockingReadResolver) Close() error {
	r.Unblock()
	return nil
}

func (r *blockingReadResolver) Unblock() {
	r.releaseOnce.Do(func() { close(r.release) })
}

type blockingReadTool struct{ resolver *blockingReadResolver }

func (t blockingReadTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name: "read", Description: "Block until interrupted.",
		Parameters: json.RawMessage(`{"type":"object","additionalProperties":true}`),
	}
}

func (t blockingReadTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	t.resolver.startedOnce.Do(func() { close(t.resolver.started) })
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-t.resolver.release:
		return "released", nil
	}
}

func waitForAgentStatus(t *testing.T, ctx context.Context, manager *daemon.Manager, id string, wanted ...string) threadstore.Thread {
	t.Helper()
	wantedSet := make(map[string]bool, len(wanted))
	for _, status := range wanted {
		wantedSet[status] = true
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		resource, err := manager.Thread(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if wantedSet[resource.Status] {
			return resource
		}
		select {
		case <-ctx.Done():
			t.Fatalf("agent %s status remained %q; wanted %v", id, resource.Status, wanted)
		case <-ticker.C:
		}
	}
}

func waitForAgentMessage(t *testing.T, ctx context.Context, manager *daemon.Manager, targetID, fromID string) agentteam.Message {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		messages, err := manager.Messages(ctx, targetID, 0, 500)
		if err != nil {
			t.Fatal(err)
		}
		for _, stored := range messages {
			if stored.Kind != "agent_message" {
				continue
			}
			var message agentteam.Message
			if json.Unmarshal(stored.Data, &message) == nil && message.FromID == fromID {
				return message
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("no agent message from %s reached %s", fromID, targetID)
		case <-ticker.C:
		}
	}
}

func waitForCompletionEvent(t *testing.T, ctx context.Context, manager *daemon.Manager, id, status string) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		events, err := manager.Events(ctx, id, 0, 500)
		if err != nil {
			t.Fatal(err)
		}
		if containsCompletionEvent(events, status) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("conversation %s never emitted agent_completed with status %q", id, status)
		case <-ticker.C:
		}
	}
}

func containsStoredMessage(messages []threadstore.Message, role, content string) bool {
	for _, message := range messages {
		if message.Role == role && strings.Contains(message.Content, content) {
			return true
		}
	}
	return false
}

func containsStoredKind(messages []threadstore.Message, kind, content string) bool {
	for _, message := range messages {
		if message.Kind == kind && strings.Contains(message.Content, content) {
			return true
		}
	}
	return false
}

func containsAgent(agents []agentteam.Agent, id string) bool {
	for _, candidate := range agents {
		if candidate.ID == id {
			return true
		}
	}
	return false
}

func containsCompletionEvent(events []threadstore.Event, status string) bool {
	for _, event := range events {
		if event.Type != "agent_completed" {
			continue
		}
		var completion struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(event.Data, &completion) == nil && completion.Status == status {
			return true
		}
	}
	return false
}

func containsEventType(events []threadstore.Event, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
