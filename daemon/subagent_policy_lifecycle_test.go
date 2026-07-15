package daemon_test

import (
	"strings"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/agentteam"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/daemon"
)

func TestSubagentMaxChildrenCountsExistingChildren(t *testing.T) {
	fixture := newSubagentFixture(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
		config.Global.Subagents.MaxChildren = 1
	}, nil)
	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "only-child", Profile: "general", Task: "first task",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "extra-child", Profile: "general", Task: "second task",
	}); err == nil || !strings.Contains(err.Error(), "maximum 1 children") {
		t.Fatalf("max-children error = %v", err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, first.ID, "completed")
}

func TestSubagentProfileWithoutCanSpawnRejectsGrandchild(t *testing.T) {
	fixture := newSubagentFixture(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
		config.Global.Subagents.Profiles = map[string]configuration.AgentProfile{
			"leaf": {Description: "A non-delegating worker.", CanSpawn: false},
		}
	}, nil)
	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "leaf", Profile: "leaf", Task: "bounded leaf task",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: leaf.ID, Name: "forbidden-child", Profile: "leaf", Task: "must not start",
	}); err == nil || !strings.Contains(err.Error(), "profile cannot spawn children") {
		t.Fatalf("CanSpawn=false error = %v", err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, leaf.ID, "completed")
}

func TestWaitAgentsWithNilIDsSelectsDescendantsAndDrainsMailboxOnce(t *testing.T) {
	fixture := newSubagentFixture(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
		config.Global.Subagents.Profiles = map[string]configuration.AgentProfile{
			"delegate": {Description: "A delegating worker.", CanSpawn: true},
		}
	}, nil)
	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "delegate-parent", Profile: "delegate", Task: "parent task",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, parent.ID, "completed")
	descendant, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: parent.ID, Name: "descendant", Profile: "delegate", Task: "nested task",
	})
	if err != nil {
		t.Fatal(err)
	}
	sibling, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "root-sibling", Profile: "delegate", Task: "sibling task",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, descendant.ID, "completed")
	waitForAgentStatus(t, fixture.ctx, fixture.manager, sibling.ID, "completed")

	delivered, err := fixture.manager.SendAgentMessage(fixture.ctx, project.ID, parent.ID, "one mailbox delivery", false)
	if err != nil {
		t.Fatal(err)
	}
	first, err := fixture.manager.WaitAgents(fixture.ctx, parent.ID, nil, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if first.TimedOut || len(first.Agents) != 1 || first.Agents[0].ID != descendant.ID {
		t.Fatalf("nil-ID descendant selection = %#v", first)
	}
	if len(first.Messages) != 1 || first.Messages[0].ID != delivered.ID {
		t.Fatalf("first mailbox drain = %#v", first.Messages)
	}
	second, err := fixture.manager.WaitAgents(fixture.ctx, parent.ID, nil, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Agents) != 1 || second.Agents[0].ID != descendant.ID || len(second.Messages) != 0 {
		t.Fatalf("second mailbox drain = %#v", second)
	}
}

func TestInterruptFreesSubagentConcurrencySlot(t *testing.T) {
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
	first, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "blocking-first", Profile: "blocked", Task: "hold the slot",
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-resolver.started:
	case <-fixture.ctx.Done():
		t.Fatal("first child never entered the blocking tool")
	}
	if _, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "blocked-by-limit", Profile: "blocked", Task: "must wait",
	}); err == nil || !strings.Contains(err.Error(), "maximum 1 concurrent") {
		t.Fatalf("concurrency-limit error = %v", err)
	}
	if err := fixture.manager.InterruptAgent(fixture.ctx, project.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, first.ID, "interrupted")

	replacement, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "replacement", Profile: "blocked", Task: "use the freed slot",
	})
	if err != nil {
		t.Fatalf("spawn after interrupt: %v", err)
	}
	if replacement.Status != "running" {
		t.Fatalf("replacement status = %q", replacement.Status)
	}
	if err := fixture.manager.InterruptAgent(fixture.ctx, project.ID, replacement.ID); err != nil {
		t.Fatal(err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, replacement.ID, "interrupted")
}

func TestSubagentCannotInterruptParentOrTeamRoot(t *testing.T) {
	fixture := newSubagentFixture(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
		config.Global.Subagents.MaxDepth = 2
		config.Global.Subagents.Profiles = map[string]configuration.AgentProfile{
			"delegate": {Description: "A delegating worker.", CanSpawn: true},
		}
	}, nil)
	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "interrupt-parent", Profile: "delegate", Task: "parent task",
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: parent.ID, Name: "interrupt-child", Profile: "delegate", Task: "child task",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, parent.ID, "completed")
	waitForAgentStatus(t, fixture.ctx, fixture.manager, child.ID, "completed")
	for _, targetID := range []string{parent.ID, project.ID} {
		if err := fixture.manager.InterruptAgent(fixture.ctx, child.ID, targetID); err == nil || !strings.Contains(err.Error(), "cannot interrupt an ancestor") {
			t.Fatalf("interrupt ancestor %s error = %v", targetID, err)
		}
	}
}
