package mcpclient

import "time"

// State is a safe, user-facing summary of a server lifecycle.
type State string

const (
	StateDisabled     State = "disabled"
	StateUntrusted    State = "untrusted"
	StateDisconnected State = "disconnected"
	StateConnecting   State = "connecting"
	StateReady        State = "ready"
	StateDegraded     State = "degraded"
	StateError        State = "error"
	StateClosed       State = "closed"
)

// ServerStatus intentionally excludes commands, environment, endpoints, and
// HTTP headers so credentials cannot escape through health APIs.
type ServerStatus struct {
	Name              string        `json:"name"`
	Transport         TransportKind `json:"transport"`
	Enabled           bool          `json:"enabled"`
	Trusted           bool          `json:"trusted"`
	State             State         `json:"state"`
	ToolCount         int           `json:"tool_count"`
	SupportsResources bool          `json:"supports_resources,omitempty"`
	SupportsPrompts   bool          `json:"supports_prompts,omitempty"`
	ProtocolVersion   string        `json:"protocol_version,omitempty"`
	ServerName        string        `json:"server_name,omitempty"`
	ServerVersion     string        `json:"server_version,omitempty"`
	LastError         string        `json:"last_error,omitempty"`
	ConnectedAt       time.Time     `json:"connected_at,omitempty"`
	ToolsRefreshedAt  time.Time     `json:"tools_refreshed_at,omitempty"`
}

// ToolStatus reports discovery and call health without including arguments or
// results from calls.
type ToolStatus struct {
	Server       string    `json:"server"`
	Name         string    `json:"name"`
	ModelName    string    `json:"model_name"`
	Description  string    `json:"description,omitempty"`
	Available    bool      `json:"available"`
	LastError    string    `json:"last_error,omitempty"`
	LastCalledAt time.Time `json:"last_called_at,omitempty"`
}
