package mcpserver_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/agentteam"
	"github.com/dire-kiwi/dire-agent/daemon"
	"github.com/dire-kiwi/dire-agent/mcpserver"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

func TestBridgeListsCreatesAndRunsChats(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	backend := &fakeDaemon{}
	bridge, err := mcpserver.New(backend)
	if err != nil {
		t.Fatal(err)
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := bridge.MCP().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	created, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "dire_agent_create_chat", Arguments: map[string]any{"name": "Desktop chat"},
	})
	if err != nil || created.IsError {
		t.Fatalf("create: result=%+v err=%v", created, err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "dire_agent_send_message",
		Arguments: map[string]any{"conversation_id": "chat_1", "message": "hello"},
	})
	if err != nil || result.IsError {
		t.Fatalf("send: result=%+v err=%v", result, err)
	}
	spawned, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "dire_agent_spawn_agent", Arguments: map[string]any{
			"parent_id": "chat_1", "name": "researcher", "task": "inspect the topic",
			"mode": agentteam.SpawnModeModelRouter,
		},
	})
	if err != nil || spawned.IsError {
		t.Fatalf("spawn: result=%+v err=%v", spawned, err)
	}
	listed, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "dire_agent_list_agents", Arguments: map[string]any{"conversation_id": "chat_1"},
	})
	if err != nil || listed.IsError {
		t.Fatalf("list agents: result=%+v err=%v", listed, err)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.prompt != "hello" || len(backend.messages) != 2 {
		t.Fatalf("backend = prompt %q messages %+v", backend.prompt, backend.messages)
	}
	if backend.spawnRequest.Mode != agentteam.SpawnModeModelRouter {
		t.Fatalf("spawn mode = %q, want %q", backend.spawnRequest.Mode, agentteam.SpawnModeModelRouter)
	}
}

func TestBridgeAgentCoordinationTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	backend := &fakeDaemon{agent: agentteam.Agent{ID: "agent_1", ParentID: "chat_1", RootID: "chat_1"}}
	bridge, err := mcpserver.New(backend)
	if err != nil {
		t.Fatal(err)
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := bridge.MCP().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	for _, request := range []struct {
		name      string
		arguments map[string]any
	}{
		{
			name: "dire_agent_send_agent_message",
			arguments: map[string]any{
				"from_id": "chat_1", "agent_id": "agent_1", "message": "stay idle", "wake": false,
			},
		},
		{
			name: "dire_agent_send_agent_message",
			arguments: map[string]any{
				"from_id": "agent_1", "agent_id": "chat_1", "message": "default wake",
			},
		},
		{
			name: "dire_agent_wait_agents",
			arguments: map[string]any{
				"caller_id": "chat_1", "agent_ids": []string{"agent_1", "agent_2"}, "timeout_ms": 2750,
			},
		},
		{
			name: "dire_agent_interrupt_agent",
			arguments: map[string]any{
				"caller_id": "chat_1", "agent_id": "agent_1",
			},
		},
		{
			name: "dire_agent_delete_agent",
			arguments: map[string]any{
				"caller_id": "agent_2", "agent_id": "agent_1",
			},
		},
	} {
		result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: request.name, Arguments: request.arguments})
		if callErr != nil || result.IsError {
			t.Fatalf("%s: result=%+v err=%v", request.name, result, callErr)
		}
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if len(backend.agentMessages) != 2 {
		t.Fatalf("agent message calls = %#v", backend.agentMessages)
	}
	if first := backend.agentMessages[0]; first.from != "chat_1" || first.to != "agent_1" || first.message != "stay idle" || first.wake {
		t.Fatalf("explicit wake=false message = %#v", first)
	}
	if second := backend.agentMessages[1]; second.from != "agent_1" || second.to != "chat_1" || second.message != "default wake" || !second.wake {
		t.Fatalf("default wake message = %#v", second)
	}
	if backend.waitCall.caller != "chat_1" || backend.waitCall.timeout != 2750*time.Millisecond ||
		len(backend.waitCall.agentIDs) != 2 || backend.waitCall.agentIDs[0] != "agent_1" || backend.waitCall.agentIDs[1] != "agent_2" {
		t.Fatalf("wait call = %#v", backend.waitCall)
	}
	if backend.interruptCall != (agentTargetCall{caller: "chat_1", agentID: "agent_1"}) {
		t.Fatalf("interrupt call = %#v", backend.interruptCall)
	}
	if backend.deleteCall != (agentTargetCall{caller: "agent_2", agentID: "agent_1"}) {
		t.Fatalf("delete call = %#v", backend.deleteCall)
	}
}

type agentMessageCall struct {
	from    string
	to      string
	message string
	wake    bool
}

type waitAgentsCall struct {
	caller   string
	agentIDs []string
	timeout  time.Duration
}

type agentTargetCall struct {
	caller  string
	agentID string
}

type fakeDaemon struct {
	mu            sync.Mutex
	chat          threadstore.Chat
	prompt        string
	messages      []threadstore.Message
	agent         agentteam.Agent
	spawnRequest  agentteam.SpawnRequest
	agentMessages []agentMessageCall
	waitCall      waitAgentsCall
	interruptCall agentTargetCall
	deleteCall    agentTargetCall
}

func (f *fakeDaemon) ListProjects(context.Context) ([]threadstore.Project, error) { return nil, nil }
func (f *fakeDaemon) ListChats(context.Context) ([]threadstore.Chat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.chat.ID == "" {
		return nil, nil
	}
	return []threadstore.Chat{f.chat}, nil
}
func (f *fakeDaemon) ListConversations(ctx context.Context) ([]threadstore.Conversation, error) {
	return f.ListChats(ctx)
}
func (f *fakeDaemon) CreateProject(context.Context, daemon.CreateProjectOptions) (threadstore.Project, error) {
	return threadstore.Project{ID: "project_1", Kind: threadstore.KindProject}, nil
}
func (f *fakeDaemon) CreateChat(_ context.Context, options daemon.CreateChatOptions) (threadstore.Chat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chat = threadstore.Chat{ID: "chat_1", Kind: threadstore.KindChat, Name: options.Name, Model: "gpt-5.6"}
	return f.chat, nil
}
func (f *fakeDaemon) Conversation(context.Context, string) (threadstore.Conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.chat, nil
}
func (f *fakeDaemon) State(context.Context, string) (daemon.RuntimeState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return daemon.RuntimeState{Kind: threadstore.KindChat, Conversation: f.chat, Chat: f.chat, Usage: agent.Usage{}}, nil
}
func (f *fakeDaemon) Messages(context.Context, string, int64, int) ([]threadstore.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]threadstore.Message(nil), f.messages...), nil
}
func (f *fakeDaemon) Prompt(_ context.Context, _ string, message, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prompt = message
	f.messages = []threadstore.Message{{Role: "user", Content: message}, {Role: "assistant", Content: "done"}}
	return nil
}
func (f *fakeDaemon) WaitForSettled(context.Context, string) (daemon.WireEvent, error) {
	return daemon.WireEvent{Type: "agent_settled", ConversationID: "chat_1"}, nil
}
func (f *fakeDaemon) Abort(context.Context, string) error { return nil }
func (f *fakeDaemon) SpawnAgent(_ context.Context, request agentteam.SpawnRequest) (agentteam.Agent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spawnRequest = request
	f.agent = agentteam.Agent{ID: "agent_1", ParentID: request.ParentID, RootID: request.ParentID, Name: request.Name, Status: "running"}
	return f.agent, nil
}
func (f *fakeDaemon) ListAgents(context.Context, string) ([]agentteam.Agent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.agent.ID == "" {
		return nil, nil
	}
	return []agentteam.Agent{f.agent}, nil
}
func (f *fakeDaemon) SendAgentMessage(_ context.Context, from, to, message string, wake bool) (agentteam.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.agentMessages = append(f.agentMessages, agentMessageCall{from: from, to: to, message: message, wake: wake})
	return agentteam.Message{ID: "agentmsg_1", FromID: from, ToID: to, Content: message}, nil
}
func (f *fakeDaemon) WaitAgents(_ context.Context, caller string, agentIDs []string, timeout time.Duration) (agentteam.WaitResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.waitCall = waitAgentsCall{caller: caller, agentIDs: append([]string(nil), agentIDs...), timeout: timeout}
	return agentteam.WaitResult{Agents: []agentteam.Agent{f.agent}}, nil
}
func (f *fakeDaemon) InterruptAgent(_ context.Context, caller, agentID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.interruptCall = agentTargetCall{caller: caller, agentID: agentID}
	return nil
}
func (f *fakeDaemon) DeleteAgent(_ context.Context, caller, agentID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCall = agentTargetCall{caller: caller, agentID: agentID}
	return nil
}
