package daemon_test

import (
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/agentteam"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/daemon"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

func TestRunningSubagentRecoversAsInterruptedAfterManagerRestart(t *testing.T) {
	fixture := newSubagentFixture(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
	}, nil)
	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "restart-recovery", Profile: "general", Task: "finish once",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, child.ID, "completed")
	if err := fixture.manager.Close(); err != nil {
		t.Fatal(err)
	}

	childDB, err := fixture.store.Open(fixture.ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := childDB.UpdateThread(fixture.ctx, func(stored *threadstore.Thread) error {
		stored.Status = "running"
		return nil
	}); err != nil {
		_ = childDB.Close()
		t.Fatal(err)
	}
	if err := childDB.Close(); err != nil {
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
	waited, err := reopened.WaitAgents(fixture.ctx, project.ID, []string{child.ID}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if waited.TimedOut || len(waited.Agents) != 1 || waited.Agents[0].Status != "interrupted" {
		t.Fatalf("wait after recovery = %#v", waited)
	}
	recovered, err := reopened.Thread(fixture.ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "interrupted" {
		t.Fatalf("recovered subagent status = %q, want interrupted", recovered.Status)
	}
}
