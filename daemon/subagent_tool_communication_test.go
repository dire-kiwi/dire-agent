package daemon_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/agentteam"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/daemon"
)

func TestSubagentToolMessageReachesAndWakesParent(t *testing.T) {
	provider := newToolCommunicationProvider()
	fixture := newSubagentFixtureWithProvider(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
	}, nil, provider)
	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider.parentID = project.ID
	child, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "tool-messenger", Profile: "general", Task: "send the verified finding",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, child.ID, "completed")
	waitForAgentStatus(t, fixture.ctx, fixture.manager, project.ID, "idle")

	select {
	case prompt := <-provider.parentPrompts:
		if !strings.Contains(prompt, "Message from tool-messenger") || !strings.Contains(prompt, "verified child finding") {
			t.Fatalf("parent wake prompt = %q", prompt)
		}
	case <-fixture.ctx.Done():
		t.Fatal("parent model did not consume the child message")
	}
	waited, err := fixture.manager.WaitAgents(fixture.ctx, project.ID, []string{child.ID}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(waited.Messages) != 1 || waited.Messages[0].FromID != child.ID || waited.Messages[0].ToID != project.ID || waited.Messages[0].Content != "verified child finding" {
		t.Fatalf("parent mailbox = %#v", waited)
	}
	if !provider.childSawSendTool.Load() || !provider.childSawSuccessfulResult.Load() {
		t.Fatalf("child tool execution: exposed=%v successful=%v", provider.childSawSendTool.Load(), provider.childSawSuccessfulResult.Load())
	}
}

func TestRunningSubagentModelConsumesSteeredAgentMessage(t *testing.T) {
	provider := newSteeringCaptureProvider()
	fixture := newSubagentFixtureWithProvider(t, func(config *configuration.Config) {
		config.Global.Subagents.AutoReport = false
	}, nil, provider)
	project, err := fixture.manager.CreateProject(fixture.ctx, daemon.CreateProjectOptions{
		CWD: fixture.root, Model: "fake-model", Tools: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := fixture.manager.SpawnAgent(fixture.ctx, agentteam.SpawnRequest{
		ParentID: project.ID, Name: "steering-consumer", Profile: "general", Task: "wait for guidance", Tools: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.started:
	case <-fixture.ctx.Done():
		t.Fatal("child model did not enter its running step")
	}
	if _, err := fixture.manager.SendAgentMessage(fixture.ctx, project.ID, child.ID, "consume this while running", true); err != nil {
		t.Fatal(err)
	}
	provider.Release()
	select {
	case prompt := <-provider.received:
		if !strings.Contains(prompt, "Message from") || !strings.Contains(prompt, "consume this while running") {
			t.Fatalf("steered model prompt = %q", prompt)
		}
	case <-fixture.ctx.Done():
		t.Fatal("running child model never consumed the agent message")
	}
	waitForAgentStatus(t, fixture.ctx, fixture.manager, child.ID, "completed")
}

type toolCommunicationProvider struct {
	next                     atomic.Int64
	parentID                 string
	parentPrompts            chan string
	childSawSendTool         atomic.Bool
	childSawSuccessfulResult atomic.Bool
}

func newToolCommunicationProvider() *toolCommunicationProvider {
	return &toolCommunicationProvider{parentPrompts: make(chan string, 4)}
}

func (p *toolCommunicationProvider) OpenSession(_ context.Context, options agent.SessionOptions) (agent.Session, error) {
	kind := "parent"
	if strings.Contains(options.Instructions, `You are subagent "tool-messenger"`) {
		kind = "child"
	}
	return &toolCommunicationSession{id: fmt.Sprintf("tool-comm-%d", p.next.Add(1)), kind: kind, provider: p}, nil
}

func (p *toolCommunicationProvider) OpenSessionFromState(_ context.Context, _ agent.SessionOptions, state agent.SessionState) (agent.Session, error) {
	var saved struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(state.Data, &saved)
	return &toolCommunicationSession{id: state.ID, kind: saved.Kind, provider: p}, nil
}

func (*toolCommunicationProvider) Close() error { return nil }

type toolCommunicationSession struct {
	id       string
	kind     string
	provider *toolCommunicationProvider
	mu       sync.Mutex
	called   bool
}

func (s *toolCommunicationSession) ID() string { return s.id }

func (s *toolCommunicationSession) Run(ctx context.Context, prompt string) (agent.Result, error) {
	step, err := s.Step(ctx, agent.StepRequest{UserMessages: []string{prompt}})
	return step.Result, err
}

func (s *toolCommunicationSession) Step(_ context.Context, request agent.StepRequest) (agent.StepResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.kind != "child" {
		for _, prompt := range request.UserMessages {
			s.provider.parentPrompts <- prompt
		}
		return agent.StepResult{Result: agent.Result{Text: "parent received message", SessionID: s.id, TurnID: "parent-final"}}, nil
	}
	if len(request.ToolResults) != 0 {
		if !request.ToolResults[0].IsError && strings.Contains(request.ToolResults[0].Output, "verified child finding") {
			s.provider.childSawSuccessfulResult.Store(true)
		}
		return agent.StepResult{Result: agent.Result{Text: "message sent", SessionID: s.id, TurnID: "child-final"}}, nil
	}
	for _, tool := range request.Tools {
		if tool.Name == "send_agent_message" {
			s.provider.childSawSendTool.Store(true)
		}
	}
	s.called = true
	arguments, _ := json.Marshal(map[string]any{
		"agent_id": s.provider.parentID,
		"message":  "verified child finding",
		"wake":     true,
	})
	return agent.StepResult{
		Result:    agent.Result{SessionID: s.id, TurnID: "child-send"},
		ToolCalls: []agent.ToolCall{{ID: "send-parent", Name: "send_agent_message", Arguments: arguments}},
	}, nil
}

func (s *toolCommunicationSession) State() (agent.SessionState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := json.Marshal(map[string]string{"kind": s.kind})
	return agent.SessionState{ID: s.id, Provider: "tool-communication", Data: data}, nil
}

type steeringCaptureProvider struct {
	next        atomic.Int64
	started     chan struct{}
	release     chan struct{}
	received    chan string
	startedOnce sync.Once
	releaseOnce sync.Once
}

func newSteeringCaptureProvider() *steeringCaptureProvider {
	return &steeringCaptureProvider{
		started: make(chan struct{}), release: make(chan struct{}), received: make(chan string, 2),
	}
}

func (p *steeringCaptureProvider) OpenSession(_ context.Context, options agent.SessionOptions) (agent.Session, error) {
	kind := "parent"
	if strings.Contains(options.Instructions, `You are subagent "steering-consumer"`) {
		kind = "child"
	}
	return &steeringCaptureSession{id: fmt.Sprintf("steering-%d", p.next.Add(1)), kind: kind, provider: p}, nil
}

func (p *steeringCaptureProvider) OpenSessionFromState(_ context.Context, _ agent.SessionOptions, state agent.SessionState) (agent.Session, error) {
	var saved struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(state.Data, &saved)
	return &steeringCaptureSession{id: state.ID, kind: saved.Kind, provider: p}, nil
}

func (p *steeringCaptureProvider) Release() {
	p.releaseOnce.Do(func() { close(p.release) })
}

func (p *steeringCaptureProvider) Close() error {
	p.Release()
	return nil
}

type steeringCaptureSession struct {
	id       string
	kind     string
	provider *steeringCaptureProvider
	step     atomic.Int64
}

func (s *steeringCaptureSession) ID() string { return s.id }

func (s *steeringCaptureSession) Run(ctx context.Context, prompt string) (agent.Result, error) {
	step, err := s.Step(ctx, agent.StepRequest{UserMessages: []string{prompt}})
	return step.Result, err
}

func (s *steeringCaptureSession) Step(ctx context.Context, request agent.StepRequest) (agent.StepResult, error) {
	if s.kind != "child" {
		return agent.StepResult{Result: agent.Result{Text: "parent idle", SessionID: s.id, TurnID: "parent"}}, nil
	}
	if s.step.Add(1) == 1 {
		s.provider.startedOnce.Do(func() { close(s.provider.started) })
		select {
		case <-ctx.Done():
			return agent.StepResult{}, ctx.Err()
		case <-s.provider.release:
		}
		return agent.StepResult{Result: agent.Result{Text: "ready for steering", SessionID: s.id, TurnID: "initial"}}, nil
	}
	for _, prompt := range request.UserMessages {
		s.provider.received <- prompt
	}
	return agent.StepResult{Result: agent.Result{Text: "steering consumed", SessionID: s.id, TurnID: "steered"}}, nil
}

func (s *steeringCaptureSession) State() (agent.SessionState, error) {
	data, _ := json.Marshal(map[string]string{"kind": s.kind})
	return agent.SessionState{ID: s.id, Provider: "steering-capture", Data: data}, nil
}
