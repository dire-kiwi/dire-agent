package mcpclient

import (
	"errors"
	"sort"
	"time"
)

func (runtime *serverRuntime) setState(state State, message string) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	runtime.status.State = state
	runtime.status.LastError = message
}

func (runtime *serverRuntime) installSession(session Session) (Session, uint64) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	old := runtime.session
	runtime.session = session
	runtime.generation++
	runtime.tools = make(map[string]*discoveredTool)
	runtime.status.State = StateReady
	runtime.status.LastError = ""
	runtime.status.ToolCount = 0
	runtime.status.ConnectedAt = time.Now().UTC()
	if initialized := session.InitializeResult(); initialized != nil {
		runtime.status.ProtocolVersion = initialized.ProtocolVersion
		if initialized.Capabilities != nil {
			runtime.status.SupportsResources = initialized.Capabilities.Resources != nil
			runtime.status.SupportsPrompts = initialized.Capabilities.Prompts != nil
		}
		if initialized.ServerInfo != nil {
			runtime.status.ServerName = initialized.ServerInfo.Name
			runtime.status.ServerVersion = initialized.ServerInfo.Version
		}
	}
	return old, runtime.generation
}

func (runtime *serverRuntime) installTools(generation uint64, tools map[string]*discoveredTool) bool {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.session == nil || runtime.generation != generation {
		return false
	}
	runtime.tools = tools
	runtime.status.State = StateReady
	runtime.status.LastError = ""
	runtime.status.ToolCount = len(tools)
	runtime.status.ToolsRefreshedAt = time.Now().UTC()
	return true
}

// ServerStatuses returns a name-sorted, secret-free status snapshot.
func (c *Client) ServerStatuses() []ServerStatus {
	statuses := make([]ServerStatus, 0, len(c.servers))
	for _, name := range c.serverNames() {
		runtime := c.servers[name]
		runtime.mu.RLock()
		statuses = append(statuses, runtime.status)
		runtime.mu.RUnlock()
	}
	return statuses
}

// ToolStatuses returns a model-name-sorted health snapshot.
func (c *Client) ToolStatuses() []ToolStatus {
	var statuses []ToolStatus
	for _, name := range c.serverNames() {
		runtime := c.servers[name]
		runtime.mu.RLock()
		for _, tool := range runtime.tools {
			statuses = append(statuses, tool.status)
		}
		runtime.mu.RUnlock()
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].ModelName < statuses[j].ModelName })
	return statuses
}

// Close cancels background refreshes and gracefully closes every session.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.cancel()
	c.mu.Unlock()

	var sessions []Session
	for _, runtime := range c.servers {
		runtime.mu.Lock()
		if runtime.session != nil {
			sessions = append(sessions, runtime.session)
			runtime.session = nil
		}
		runtime.generation++
		runtime.status.State = StateClosed
		for _, tool := range runtime.tools {
			tool.status.Available = false
		}
		runtime.mu.Unlock()
	}
	var failures []error
	for _, session := range sessions {
		if err := session.Close(); err != nil {
			failures = append(failures, err)
		}
	}
	c.refreshes.Wait()
	return errors.Join(failures...)
}
