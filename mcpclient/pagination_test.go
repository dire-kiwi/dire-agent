package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolDiscoveryFollowsPagination(t *testing.T) {
	session := &fakeSession{pages: map[string]*mcp.ListToolsResult{
		"": {
			Tools:      []*mcp.Tool{testTool("first")},
			NextCursor: "page-2",
		},
		"page-2": {Tools: []*mcp.Tool{testTool("second")}},
	}}
	client := newFakeClient(t, session)
	defer client.Close()
	if err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := session.requestedCursors(); !reflect.DeepEqual(got, []string{"", "page-2"}) {
		t.Fatalf("requested cursors = %#v", got)
	}
	tools := client.AgentTools()
	if tools["mcp__pages__first"] == nil || tools["mcp__pages__second"] == nil || len(tools) != 2 {
		t.Fatalf("unexpected tools: %#v", tools)
	}
}

func TestRepeatedPaginationCursorKeepsOldCatalog(t *testing.T) {
	session := &fakeSession{pages: map[string]*mcp.ListToolsResult{
		"":      {Tools: []*mcp.Tool{testTool("first")}, NextCursor: "again"},
		"again": {NextCursor: "again"},
	}}
	client := newFakeClient(t, session)
	defer client.Close()
	err := client.Connect(context.Background())
	if err == nil || !stringsContain(err.Error(), "repeated a pagination cursor") {
		t.Fatalf("Connect error = %v", err)
	}
	if len(client.AgentTools()) != 0 {
		t.Fatal("partial catalog was installed")
	}
	if client.ServerStatuses()[0].State != StateDegraded {
		t.Fatalf("unexpected status: %#v", client.ServerStatuses()[0])
	}
}

func TestRefreshRejectsInvalidSchema(t *testing.T) {
	session := &fakeSession{pages: map[string]*mcp.ListToolsResult{"": {Tools: []*mcp.Tool{{
		Name: "bad", InputSchema: []string{"not", "an", "object"},
	}}}}}
	client := newFakeClient(t, session)
	defer client.Close()
	err := client.Connect(context.Background())
	if err == nil || !stringsContain(err.Error(), "schema is not a JSON object") {
		t.Fatalf("Connect error = %v", err)
	}
}

type fakeSession struct {
	mu      sync.Mutex
	pages   map[string]*mcp.ListToolsResult
	cursors []string
	closed  bool
}

func (s *fakeSession) ListTools(_ context.Context, params *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursors = append(s.cursors, params.Cursor)
	result, ok := s.pages[params.Cursor]
	if !ok {
		return nil, errors.New("unexpected cursor")
	}
	return result, nil
}

func (s *fakeSession) CallTool(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
}

func (s *fakeSession) InitializeResult() *mcp.InitializeResult {
	return &mcp.InitializeResult{ProtocolVersion: "test", ServerInfo: &mcp.Implementation{Name: "fake", Version: "1"}}
}

func (s *fakeSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *fakeSession) requestedCursors() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.cursors...)
}

func newFakeClient(t *testing.T, session Session) *Client {
	t.Helper()
	clientTransport, _ := mcp.NewInMemoryTransports()
	client, err := New([]ServerConfig{{Name: "pages", Enabled: true, Trusted: true, Transport: TransportStdio, Command: "unused"}}, Options{
		TransportFactory: TransportFactoryFunc(func(context.Context, ServerConfig) (mcp.Transport, error) {
			return clientTransport, nil
		}),
		Connector: ConnectorFunc(func(context.Context, mcp.Transport, ConnectOptions) (Session, error) {
			return session, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func testTool(name string) *mcp.Tool {
	return &mcp.Tool{Name: name, InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func stringsContain(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
