package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOpenTrustGateDoesNotConnect(t *testing.T) {
	called := false
	connector := ConnectorFunc(func(context.Context, ProcessSpec, Limits) (Connection, error) {
		called = true
		return nil, errors.New("unexpected")
	})
	base := LaunchConfig{ID: "demo", Enabled: true, Trust: TrustPrompt, Process: ProcessSpec{Command: "adapter"}}
	if _, err := Open(context.Background(), base, OpenOptions{Connector: connector}); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("error = %v", err)
	}
	base.Enabled = false
	base.Trust = TrustTrusted
	if _, err := Open(context.Background(), base, OpenOptions{Connector: connector}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("error = %v", err)
	}
	if called {
		t.Fatal("connector called before trust gate")
	}
}

func TestClientToolsAndBoundedResults(t *testing.T) {
	connection := newFakeConnection()
	client := openFake(t, connection, Limits{MaxOutputBytes: 40})
	defer client.Close(context.Background())

	tools := client.AgentTools()
	tool, ok := tools["ext__demo_plugin__echo"]
	if !ok {
		t.Fatalf("tools = %#v", tools)
	}
	definition := tool.Definition()
	if definition.Name != "ext__demo_plugin__echo" || !json.Valid(definition.Parameters) {
		t.Fatalf("definition = %#v", definition)
	}
	output, err := tool.Execute(context.Background(), json.RawMessage(`{"value":"hello"}`))
	if err != nil || output != "hello" {
		t.Fatalf("execute = %q, %v", output, err)
	}
	connection.setLongOutput(strings.Repeat("x", 100))
	output, err = tool.Execute(context.Background(), json.RawMessage(`{"value":"ignored"}`))
	if err != nil || len(output) > 40 || !strings.Contains(output, "truncated") {
		t.Fatalf("bounded output = %q, %v", output, err)
	}
	connection.setLongOutput("")
	output, err = tool.Execute(context.Background(), json.RawMessage(`{"fail":true}`))
	if output != "requested failure" || !errors.Is(err, ErrToolReported) {
		t.Fatalf("tool failure = %q, %v", output, err)
	}
	if got := client.Stderr(); got != "fake stderr" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestClientTimeoutAndClose(t *testing.T) {
	connection := newFakeConnection()
	client := openFake(t, connection, Limits{CallTimeout: 20 * time.Millisecond})
	connection.setBlocked(true)
	_, err := client.CallTool(context.Background(), "echo", json.RawMessage(`{}`))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}
	connection.setBlocked(false)
	if err := client.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !connection.wasClosed() || !connection.saw("shutdown") {
		t.Fatalf("close state = %#v", connection)
	}
	if _, err := client.CallTool(context.Background(), "echo", nil); !errors.Is(err, ErrClosed) {
		t.Fatalf("post-close error = %v", err)
	}
}

func TestClientRejectsOversizedArguments(t *testing.T) {
	connection := newFakeConnection()
	client := openFake(t, connection, Limits{MaxMessageBytes: 32})
	defer client.Close(context.Background())
	_, err := client.CallTool(context.Background(), "echo", json.RawMessage(`{"value":"this request is intentionally much too long"}`))
	if err == nil || !strings.Contains(err.Error(), "exceed") {
		t.Fatalf("error = %v", err)
	}
}

func TestRefreshAndCallsAreRaceSafe(t *testing.T) {
	connection := newFakeConnection()
	client := openFake(t, connection, Limits{})
	defer client.Close(context.Background())
	var wait sync.WaitGroup
	for index := 0; index < 24; index++ {
		wait.Add(1)
		go func(refresh bool) {
			defer wait.Done()
			if refresh {
				if err := client.RefreshTools(context.Background()); err != nil {
					t.Errorf("refresh: %v", err)
				}
				_ = client.AgentTools()
				return
			}
			if _, err := client.CallTool(context.Background(), "echo", json.RawMessage(`{"value":"ok"}`)); err != nil {
				t.Errorf("call: %v", err)
			}
		}(index%2 == 0)
	}
	wait.Wait()
}

func TestOpenRejectsInvalidToolSchemaAndCleansUp(t *testing.T) {
	connection := newFakeConnection()
	connection.tools = []ToolSpec{{Name: "bad", InputSchema: json.RawMessage(`{"type":"string"}`)}}
	_, err := Open(context.Background(), trustedFakeConfig(), OpenOptions{Connector: staticConnector(connection)})
	if err == nil || !strings.Contains(err.Error(), "type object") {
		t.Fatalf("error = %v", err)
	}
	if !connection.wasClosed() {
		t.Fatal("connection was not cleaned up")
	}
}

type fakeConnection struct {
	mu           sync.Mutex
	calls        []string
	tools        []ToolSpec
	longOutput   string
	blocked      bool
	closed       bool
	registration Registration
}

func newFakeConnection() *fakeConnection {
	return &fakeConnection{tools: []ToolSpec{{
		Name: "echo", Description: "Echo a value.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}}}`),
	}}}
}

func (f *fakeConnection) Call(ctx context.Context, method string, params any, result any) error {
	f.mu.Lock()
	f.calls = append(f.calls, method)
	blocked := f.blocked && method == "call_tool"
	tools := append([]ToolSpec(nil), f.tools...)
	longOutput := f.longOutput
	f.mu.Unlock()
	if blocked {
		<-ctx.Done()
		return ctx.Err()
	}
	switch method {
	case "initialize":
		return assignJSON(result, initializeResult{ProtocolVersion: ProtocolVersion, Server: PeerInfo{Name: "fake"}, Registration: f.registration})
	case "list_tools":
		return assignJSON(result, listToolsResult{Tools: tools})
	case "call_tool":
		contents, _ := json.Marshal(params)
		var call struct {
			Arguments struct {
				Value string `json:"value"`
				Fail  bool   `json:"fail"`
			} `json:"arguments"`
		}
		_ = json.Unmarshal(contents, &call)
		response := ToolResult{Output: call.Arguments.Value}
		if longOutput != "" {
			response.Output = longOutput
		}
		if call.Arguments.Fail {
			response = ToolResult{Output: "requested failure", IsError: true}
		}
		return assignJSON(result, response)
	case "shutdown":
		return assignJSON(result, struct{}{})
	case "execute_command":
		return assignJSON(result, CommandResult{Output: "command complete", Prompt: "run command"})
	case "invoke_hook":
		contents, _ := json.Marshal(params)
		var call invokeHookParams
		_ = json.Unmarshal(contents, &call)
		if call.HookID == "prefix" {
			value := "checked: " + call.Payload.Prompt
			return assignJSON(result, HookResult{Prompt: &value})
		}
		return assignJSON(result, HookResult{Veto: true, Message: "blocked"})
	case "get_status":
		return assignJSON(result, Status{Level: "ready", Message: "extension ready"})
	default:
		return &RPCError{Code: -32601, Message: "unknown method"}
	}
}

func (f *fakeConnection) Stderr() string { return "fake stderr" }
func (f *fakeConnection) Close(context.Context) error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return nil
}
func (f *fakeConnection) setLongOutput(value string) {
	f.mu.Lock()
	f.longOutput = value
	f.mu.Unlock()
}
func (f *fakeConnection) setBlocked(value bool) { f.mu.Lock(); f.blocked = value; f.mu.Unlock() }
func (f *fakeConnection) wasClosed() bool       { f.mu.Lock(); defer f.mu.Unlock(); return f.closed }
func (f *fakeConnection) saw(method string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, call := range f.calls {
		if call == method {
			return true
		}
	}
	return false
}

func openFake(t *testing.T, connection Connection, limits Limits) *Client {
	t.Helper()
	client, err := Open(context.Background(), trustedFakeConfig(), OpenOptions{Connector: staticConnector(connection), Limits: limits})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func trustedFakeConfig() LaunchConfig {
	return LaunchConfig{ID: "Demo Plugin", Enabled: true, Trust: TrustTrusted, Process: ProcessSpec{Command: "unused"}}
}

func staticConnector(connection Connection) Connector {
	return ConnectorFunc(func(context.Context, ProcessSpec, Limits) (Connection, error) { return connection, nil })
}

func assignJSON(target, value any) error {
	contents, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(contents, target)
}
