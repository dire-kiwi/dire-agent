package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestInMemoryDiscoveryCallAndAgentAdapter(t *testing.T) {
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1.2.3"}, nil)
	server.AddTool(&mcp.Tool{
		Name:        "greet",
		Description: "Greets a person",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`),
	}, func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var arguments struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(request.Params.Arguments, &arguments); err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: "Hello " + arguments.Name}},
			StructuredContent: map[string]any{"greeted": arguments.Name},
		}, nil
	})
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()

	var factoryCalls atomic.Int32
	client := newInMemoryClient(t, clientTransport, &factoryCalls, 0)
	defer client.Close()
	if err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if factoryCalls.Load() != 1 {
		t.Fatalf("transport factory calls = %d, want 1", factoryCalls.Load())
	}
	statuses := client.ServerStatuses()
	if len(statuses) != 1 || statuses[0].State != StateReady || statuses[0].ToolCount != 1 {
		t.Fatalf("unexpected server status: %#v", statuses)
	}
	if statuses[0].ServerName != "fixture" || statuses[0].ServerVersion != "1.2.3" {
		t.Fatalf("missing server identity: %#v", statuses[0])
	}

	tools := client.AgentTools()
	tool := tools["mcp__memory__greet"]
	if tool == nil {
		t.Fatalf("agent tools = %#v", tools)
	}
	definition := tool.Definition()
	if definition.Name != "mcp__memory__greet" || definition.Description != "Greets a person" {
		t.Fatalf("unexpected definition: %#v", definition)
	}
	output, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"Ada"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "Hello Ada") || !strings.Contains(output, `"greeted":"Ada"`) {
		t.Fatalf("unexpected output %q", output)
	}
	toolStatuses := client.ToolStatuses()
	if len(toolStatuses) != 1 || toolStatuses[0].LastCalledAt.IsZero() || toolStatuses[0].LastError != "" {
		t.Fatalf("unexpected tool status: %#v", toolStatuses)
	}
}

func TestCallTimeoutCancelsInMemoryServer(t *testing.T) {
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "wait", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		})
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := newInMemoryClient(t, clientTransport, nil, 20*time.Millisecond)
	defer client.Close()
	if err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err = client.CallTool(context.Background(), "mcp__memory__wait", json.RawMessage(`{}`))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("CallTool error = %v, want deadline exceeded", err)
	}
	if client.ServerStatuses()[0].State != StateDegraded {
		t.Fatalf("status after timeout = %#v", client.ServerStatuses()[0])
	}
}

func newInMemoryClient(t *testing.T, transport mcp.Transport, calls *atomic.Int32, callTimeout time.Duration) *Client {
	t.Helper()
	client, err := New([]ServerConfig{{
		Name: "memory", Enabled: true, Trusted: true, Transport: TransportStdio, Command: "unused", CallTimeout: callTimeout,
	}}, Options{TransportFactory: TransportFactoryFunc(func(context.Context, ServerConfig) (mcp.Transport, error) {
		if calls != nil {
			calls.Add(1)
		}
		return transport, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
