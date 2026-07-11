package mcpclient

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Client owns initialized sessions and a cached tool catalog for MCP servers.
type Client struct {
	mu         sync.Mutex
	closed     bool
	servers    map[string]*serverRuntime
	options    Options
	factory    TransportFactory
	connector  Connector
	background context.Context
	cancel     context.CancelFunc
	refreshes  sync.WaitGroup
}

type serverRuntime struct {
	opMu       sync.Mutex
	mu         sync.RWMutex
	config     ServerConfig
	session    Session
	generation uint64
	status     ServerStatus
	tools      map[string]*discoveredTool
}

// New validates and copies configuration. It does not contact any server.
func New(configs []ServerConfig, options Options) (*Client, error) {
	options = normalizeOptions(options)
	background, cancel := context.WithCancel(context.Background())
	client := &Client{
		servers:    make(map[string]*serverRuntime, len(configs)),
		options:    options,
		factory:    options.TransportFactory,
		connector:  options.Connector,
		background: background,
		cancel:     cancel,
	}
	if client.factory == nil {
		client.factory = DefaultTransportFactory{}
	}
	if client.connector == nil {
		client.connector = sdkConnector{name: options.ClientName, version: options.ClientVersion}
	}
	for _, input := range configs {
		cfg := cloneConfig(input)
		if err := cfg.validate(); err != nil {
			cancel()
			return nil, &ConfigError{Server: cfg.Name, Message: err.Error()}
		}
		if _, exists := client.servers[cfg.Name]; exists {
			cancel()
			return nil, &ConfigError{Server: cfg.Name, Message: "duplicate server name"}
		}
		state := StateDisconnected
		if !cfg.Enabled {
			state = StateDisabled
		} else if !cfg.Trusted {
			state = StateUntrusted
		}
		client.servers[cfg.Name] = &serverRuntime{
			config: cfg,
			status: ServerStatus{Name: cfg.Name, Transport: cfg.Transport, Enabled: cfg.Enabled, Trusted: cfg.Trusted, State: state},
			tools:  make(map[string]*discoveredTool),
		}
	}
	return client, nil
}

// Connect initializes every enabled and trusted server. Successful servers
// remain usable if another server fails.
func (c *Client) Connect(ctx context.Context) error {
	var failures []error
	for _, name := range c.serverNames() {
		runtime := c.servers[name]
		if !runtime.config.Enabled || !runtime.config.Trusted {
			continue
		}
		if err := c.ConnectServer(ctx, name); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// ConnectServer initializes one server and refreshes its complete tool list.
func (c *Client) ConnectServer(ctx context.Context, name string) error {
	runtime, err := c.getServer(name)
	if err != nil {
		return err
	}
	runtime.opMu.Lock()
	defer runtime.opMu.Unlock()
	if !runtime.config.Enabled {
		return fmt.Errorf("%w: %s", ErrDisabled, name)
	}
	if !runtime.config.Trusted {
		return fmt.Errorf("%w: %s", ErrUntrusted, name)
	}
	if c.isClosed() {
		return ErrClosed
	}
	runtime.setState(StateConnecting, "")
	transport, err := c.factory.NewTransport(ctx, cloneConfig(runtime.config))
	if err != nil {
		return c.connectionFailure(runtime, "creating transport", err)
	}
	connectCtx, cancel := context.WithTimeout(ctx, c.timeout(runtime.config.ConnectTimeout, c.options.ConnectTimeout))
	defer cancel()
	session, err := c.connector.Connect(connectCtx, transport, ConnectOptions{
		ToolListChanged: func() { c.scheduleRefresh(name) },
	})
	if err != nil {
		return c.connectionFailure(runtime, "connecting", err)
	}
	old, generation, installed := c.installSession(runtime, session)
	if !installed {
		_ = session.Close()
		return ErrClosed
	}
	if old != nil {
		_ = old.Close()
	}
	if err := c.refreshSession(ctx, runtime, session, generation); err != nil {
		return err
	}
	return nil
}

// RefreshServer fetches every page of the current tool list.
func (c *Client) RefreshServer(ctx context.Context, name string) error {
	if c.isClosed() {
		return ErrClosed
	}
	runtime, err := c.getServer(name)
	if err != nil {
		return err
	}
	runtime.mu.RLock()
	session := runtime.session
	generation := runtime.generation
	runtime.mu.RUnlock()
	if session == nil {
		return fmt.Errorf("%w: %s", ErrNotConnected, name)
	}
	return c.refreshSession(ctx, runtime, session, generation)
}

func (c *Client) refreshSession(ctx context.Context, runtime *serverRuntime, session Session, generation uint64) error {
	if initialized := session.InitializeResult(); initialized != nil && initialized.Capabilities != nil && initialized.Capabilities.Tools == nil {
		if !runtime.installTools(generation, map[string]*discoveredTool{}) {
			return fmt.Errorf("%w: session changed while listing tools", ErrNotConnected)
		}
		return nil
	}
	listCtx, cancel := context.WithTimeout(ctx, c.timeout(runtime.config.ListTimeout, c.options.ListTimeout))
	defer cancel()
	tools, err := listAllTools(listCtx, session)
	if err != nil {
		safe := safeError(runtime.config, "listing tools", err)
		runtime.setState(StateDegraded, safe.Error())
		return safe
	}
	discovered := make(map[string]*discoveredTool, len(tools))
	for _, tool := range tools {
		item, itemErr := newDiscoveredTool(runtime.config.Name, tool)
		if itemErr != nil {
			safe := safeError(runtime.config, "reading tool metadata", itemErr)
			runtime.setState(StateDegraded, safe.Error())
			return safe
		}
		if _, duplicate := discovered[item.modelName]; duplicate {
			err := fmt.Errorf("duplicate tool name %q", item.remoteName)
			safe := safeError(runtime.config, "reading tool metadata", err)
			runtime.setState(StateDegraded, safe.Error())
			return safe
		}
		discovered[item.modelName] = item
	}
	if !runtime.installTools(generation, discovered) {
		return fmt.Errorf("%w: session changed while listing tools", ErrNotConnected)
	}
	return nil
}

func listAllTools(ctx context.Context, session Session) ([]*mcp.Tool, error) {
	var tools []*mcp.Tool
	cursor := ""
	seen := map[string]bool{}
	for page := 0; page < 1000; page++ {
		result, err := session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, errors.New("tools/list returned no result")
		}
		tools = append(tools, result.Tools...)
		if result.NextCursor == "" {
			return tools, nil
		}
		if seen[result.NextCursor] {
			return nil, errors.New("tools/list repeated a pagination cursor")
		}
		seen[result.NextCursor] = true
		cursor = result.NextCursor
	}
	return nil, errors.New("tools/list exceeded 1000 pages")
}

func (c *Client) connectionFailure(runtime *serverRuntime, operation string, err error) error {
	safe := safeError(runtime.config, operation, err)
	runtime.setState(StateError, safe.Error())
	return safe
}

func (c *Client) scheduleRefresh(name string) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.refreshes.Add(1)
	c.mu.Unlock()
	go func() {
		defer c.refreshes.Done()
		_ = c.RefreshServer(c.background, name)
	}()
}

func (c *Client) timeout(server, fallback time.Duration) time.Duration {
	if server > 0 {
		return server
	}
	return fallback
}

func (c *Client) getServer(name string) (*serverRuntime, error) {
	runtime := c.servers[name]
	if runtime == nil {
		return nil, fmt.Errorf("MCP server %q is not configured", name)
	}
	return runtime, nil
}

func (c *Client) serverNames() []string {
	names := make([]string, 0, len(c.servers))
	for name := range c.servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *Client) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *Client) installSession(runtime *serverRuntime, session Session) (Session, uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, 0, false
	}
	old, generation := runtime.installSession(session)
	return old, generation, true
}
