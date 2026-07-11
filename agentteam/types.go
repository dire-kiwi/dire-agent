// Package agentteam provides model tools for persistent parent/child agents.
package agentteam

import (
	"context"
	"time"
)

type SpawnRequest struct {
	ParentID string   `json:"parent_id"`
	Name     string   `json:"name"`
	Profile  string   `json:"profile,omitempty"`
	Role     string   `json:"role,omitempty"`
	Task     string   `json:"task"`
	Model    string   `json:"model,omitempty"`
	Thinking string   `json:"thinking,omitempty"`
	Tools    []string `json:"tools,omitempty"`
}

type Agent struct {
	ID        string    `json:"id"`
	ParentID  string    `json:"parent_id"`
	RootID    string    `json:"root_id"`
	Name      string    `json:"name"`
	Role      string    `json:"role,omitempty"`
	Profile   string    `json:"profile,omitempty"`
	Depth     int       `json:"depth"`
	Status    string    `json:"status"`
	Model     string    `json:"model,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type Message struct {
	ID        string    `json:"id"`
	FromID    string    `json:"from_id"`
	ToID      string    `json:"to_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type WaitResult struct {
	Agents   []Agent   `json:"agents"`
	Messages []Message `json:"messages,omitempty"`
	TimedOut bool      `json:"timed_out,omitempty"`
}

type Backend interface {
	SpawnAgent(context.Context, SpawnRequest) (Agent, error)
	ListAgents(context.Context, string) ([]Agent, error)
	SendAgentMessage(context.Context, string, string, string, bool) (Message, error)
	WaitAgents(context.Context, string, []string, time.Duration) (WaitResult, error)
	InterruptAgent(context.Context, string, string) error
}

type Scope struct {
	AgentID  string
	CanSpawn bool
	Profiles map[string]string
}
