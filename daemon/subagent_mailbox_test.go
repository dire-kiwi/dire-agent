package daemon_test

import (
	"sync"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/agentteam"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/daemon"
)

func TestUnreadAgentMessageSurvivesRestartAndIsDeliveredOnce(t *testing.T) {
	fixture := newSubagentFixture(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
	}, nil)
	parent, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: parent.ID, Name: "mailbox-reader", Profile: "general", Task: "finish before delivery",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, child.ID, "completed")
	sent, err := fixture.manager.SendAgentMessage(fixture.ctx, parent.ID, child.ID, "persist across restart", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.manager.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := daemon.NewManager(daemon.ManagerConfig{
		Store: fixture.store, Provider: &fakeProvider{}, DefaultCWD: fixture.root,
		DefaultProvider: "fake", DefaultModel: "fake-model", Settings: fixture.settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	first, err := reopened.WaitAgents(fixture.ctx, child.ID, nil, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Messages) != 1 || first.Messages[0].ID != sent.ID || first.Messages[0].Content != "persist across restart" {
		t.Fatalf("first mailbox delivery = %#v", first)
	}
	second, err := reopened.WaitAgents(fixture.ctx, child.ID, nil, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Messages) != 0 {
		t.Fatalf("message was delivered more than once: %#v", second.Messages)
	}
}

func TestConcurrentAgentWaitersDeliverMailboxMessageOnlyOnce(t *testing.T) {
	fixture := newSubagentFixture(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
	}, nil)
	parent, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: parent.ID, Name: "concurrent-mailbox", Profile: "general", Task: "finish before concurrent waits",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, child.ID, "completed")
	if _, err := fixture.manager.SendAgentMessage(fixture.ctx, parent.ID, child.ID, "deliver to one waiter", false); err != nil {
		t.Fatal(err)
	}

	type outcome struct {
		result agentteam.WaitResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	var waiters sync.WaitGroup
	for range 2 {
		waiters.Add(1)
		go func() {
			defer waiters.Done()
			<-start
			result, waitErr := fixture.manager.WaitAgents(fixture.ctx, child.ID, nil, time.Second)
			outcomes <- outcome{result: result, err: waitErr}
		}()
	}
	close(start)
	waiters.Wait()
	close(outcomes)
	totalMessages := 0
	for current := range outcomes {
		if current.err != nil {
			t.Fatal(current.err)
		}
		totalMessages += len(current.result.Messages)
	}
	if totalMessages != 1 {
		t.Fatalf("concurrent waiters received %d copies, want exactly one", totalMessages)
	}
}
