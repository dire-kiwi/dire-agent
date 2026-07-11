package mcpclient

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Session is the subset of an initialized MCP client session used by Client.
// It also makes protocol behavior independently testable.
type Session interface {
	ListTools(context.Context, *mcp.ListToolsParams) (*mcp.ListToolsResult, error)
	CallTool(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error)
	InitializeResult() *mcp.InitializeResult
	Close() error
}

// ConnectOptions contains callbacks for server-initiated notifications.
type ConnectOptions struct {
	ToolListChanged func()
}

// Connector initializes an MCP session over a fresh transport.
type Connector interface {
	Connect(context.Context, mcp.Transport, ConnectOptions) (Session, error)
}

// ConnectorFunc adapts a function into a Connector.
type ConnectorFunc func(context.Context, mcp.Transport, ConnectOptions) (Session, error)

func (f ConnectorFunc) Connect(ctx context.Context, transport mcp.Transport, options ConnectOptions) (Session, error) {
	return f(ctx, transport, options)
}

type sdkConnector struct {
	name    string
	version string
}

func (c sdkConnector) Connect(ctx context.Context, transport mcp.Transport, options ConnectOptions) (Session, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: c.name, Version: c.version}, &mcp.ClientOptions{
		// Do not advertise roots, sampling, or elicitation. Tools, prompts, and
		// resources are client-initiated and need no additional client capability.
		Capabilities: &mcp.ClientCapabilities{},
		ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) {
			if options.ToolListChanged != nil {
				options.ToolListChanged()
			}
		},
	})
	return client.Connect(ctx, transport, nil)
}
