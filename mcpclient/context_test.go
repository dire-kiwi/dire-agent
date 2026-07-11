package mcpclient

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type contextSessionFake struct{}

func (*contextSessionFake) ListTools(context.Context, *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	panic("tools/list called for a resource/prompt-only server")
}
func (*contextSessionFake) CallTool(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{}, nil
}
func (*contextSessionFake) InitializeResult() *mcp.InitializeResult {
	return &mcp.InitializeResult{Capabilities: &mcp.ServerCapabilities{
		Resources: &mcp.ResourceCapabilities{}, Prompts: &mcp.PromptCapabilities{},
	}}
}
func (*contextSessionFake) Close() error { return nil }
func (*contextSessionFake) ListResources(_ context.Context, params *mcp.ListResourcesParams) (*mcp.ListResourcesResult, error) {
	if params.Cursor == "" {
		return &mcp.ListResourcesResult{NextCursor: "next", Resources: []*mcp.Resource{{URI: "memo://one", Name: "one", MIMEType: "text/plain"}}}, nil
	}
	return &mcp.ListResourcesResult{Resources: []*mcp.Resource{{URI: "memo://two", Name: "two"}}}, nil
}
func (*contextSessionFake) ReadResource(context.Context, *mcp.ReadResourceParams) (*mcp.ReadResourceResult, error) {
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{
		{URI: "memo://one", MIMEType: "text/plain", Text: "resource body"},
		{URI: "memo://blob", MIMEType: "application/octet-stream", Blob: []byte{1, 2, 3}},
	}}, nil
}
func (*contextSessionFake) ListPrompts(context.Context, *mcp.ListPromptsParams) (*mcp.ListPromptsResult, error) {
	return &mcp.ListPromptsResult{Prompts: []*mcp.Prompt{{
		Name: "review", Description: "Review a change.",
		Arguments: []*mcp.PromptArgument{{Name: "path", Required: true}},
	}}}, nil
}
func (*contextSessionFake) GetPrompt(_ context.Context, params *mcp.GetPromptParams) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{Description: "Rendered", Messages: []*mcp.PromptMessage{{
		Role: "user", Content: &mcp.TextContent{Text: "Review " + params.Arguments["path"]},
	}}}, nil
}

func TestMCPResourcesPromptsAndContextTools(t *testing.T) {
	client, err := New([]ServerConfig{{
		Name: "memory", Enabled: true, Trusted: true, Transport: TransportStdio, Command: "memory",
	}}, Options{
		TransportFactory: TransportFactoryFunc(func(context.Context, ServerConfig) (mcp.Transport, error) { return nil, nil }),
		Connector: ConnectorFunc(func(context.Context, mcp.Transport, ConnectOptions) (Session, error) {
			return &contextSessionFake{}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	resources, err := client.ListResources(context.Background(), "memory")
	if err != nil || len(resources) != 2 || resources[1].URI != "memo://two" {
		t.Fatalf("resources = %+v, err=%v", resources, err)
	}
	contents, err := client.ReadResource(context.Background(), "memory", "memo://one")
	if err != nil || contents[0].Text != "resource body" || contents[1].BlobBytes != 3 {
		t.Fatalf("contents = %+v, err=%v", contents, err)
	}
	prompts, err := client.ListPrompts(context.Background(), "memory")
	if err != nil || len(prompts) != 1 || !prompts[0].Arguments[0].Required {
		t.Fatalf("prompts = %+v, err=%v", prompts, err)
	}
	prompt, err := client.GetPrompt(context.Background(), "memory", "review", map[string]string{"path": "README.md"})
	if err != nil || !strings.Contains(prompt.Output, "Review README.md") {
		t.Fatalf("prompt = %+v, err=%v", prompt, err)
	}
	tools := client.ContextTools()
	if len(tools) != 4 {
		t.Fatalf("context tools = %v", tools)
	}
	output, err := tools[ContextToolName("memory", "read_resource")].Execute(context.Background(), json.RawMessage(`{"uri":"memo://one"}`))
	if err != nil || !strings.Contains(output, "resource body") {
		t.Fatalf("read tool output = %q, err=%v", output, err)
	}
}
