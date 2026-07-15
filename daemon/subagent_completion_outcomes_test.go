package daemon_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/agentteam"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/daemon"
)

func TestSubagentAutoReportsFailedOutcome(t *testing.T) {
	fixture := newSubagentFixtureWithProvider(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = true
	}, nil, &failedChildProvider{})
	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", Tools: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "failing-worker", Profile: "general", Task: "fail deterministically",
		Tools: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, child.ID, "failed")
	report := waitForAgentMessage(t, fixture.ctx, fixture.manager, project.ID, child.ID)
	if !strings.Contains(report.Content, "failed") || !strings.Contains(report.Content, "Error: worker exploded") {
		t.Fatalf("failed completion report = %q", report.Content)
	}
	waitForCompletionEvent(t, fixture.ctx, fixture.manager, project.ID, "failed")
	waitForAgentStatus(t, fixture.ctx, fixture.manager, project.ID, "idle")
}

func TestSubagentAutoReportsInterruptedOutcomeWithoutCancellationError(t *testing.T) {
	resolver := newBlockingReadResolver()
	fixture := newSubagentFixture(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = true
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
		ParentID: project.ID, Name: "cancelled-worker", Profile: "blocked", Task: "block until cancelled",
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-resolver.started:
	case <-fixture.ctx.Done():
		t.Fatal("child never entered blocking tool")
	}
	if err := fixture.manager.InterruptAgent(fixture.ctx, project.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, child.ID, "interrupted")
	report := waitForAgentMessage(t, fixture.ctx, fixture.manager, project.ID, child.ID)
	if !strings.Contains(report.Content, "interrupted") || strings.Contains(report.Content, "Error: context canceled") {
		t.Fatalf("interrupted completion report = %q", report.Content)
	}
	waitForCompletionEvent(t, fixture.ctx, fixture.manager, project.ID, "interrupted")
	resolver.Unblock()
	waitForAgentStatus(t, fixture.ctx, fixture.manager, project.ID, "idle")
}

type failedChildProvider struct{ next atomic.Int64 }

func (p *failedChildProvider) OpenSession(_ context.Context, options agent.SessionOptions) (agent.Session, error) {
	kind := "parent"
	if strings.Contains(options.Instructions, `You are subagent "failing-worker"`) {
		kind = "child"
	}
	return &failedChildSession{id: fmt.Sprintf("failed-child-%d", p.next.Add(1)), kind: kind}, nil
}

func (p *failedChildProvider) OpenSessionFromState(_ context.Context, _ agent.SessionOptions, state agent.SessionState) (agent.Session, error) {
	var saved struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(state.Data, &saved)
	return &failedChildSession{id: state.ID, kind: saved.Kind}, nil
}

func (*failedChildProvider) Close() error { return nil }

type failedChildSession struct {
	id   string
	kind string
}

func (s *failedChildSession) ID() string { return s.id }

func (s *failedChildSession) Run(ctx context.Context, prompt string) (agent.Result, error) {
	step, err := s.Step(ctx, agent.StepRequest{UserMessages: []string{prompt}})
	return step.Result, err
}

func (s *failedChildSession) Step(_ context.Context, _ agent.StepRequest) (agent.StepResult, error) {
	if s.kind == "child" {
		return agent.StepResult{}, errors.New("worker exploded")
	}
	return agent.StepResult{Result: agent.Result{Text: "failure report consumed", SessionID: s.id, TurnID: "parent-final"}}, nil
}

func (s *failedChildSession) State() (agent.SessionState, error) {
	data, _ := json.Marshal(map[string]string{"kind": s.kind})
	return agent.SessionState{ID: s.id, Provider: "failed-child", Data: data}, nil
}
