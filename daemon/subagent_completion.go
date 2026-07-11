package daemon

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type agentCompletion struct {
	Agent  any    `json:"agent"`
	Status string `json:"status"`
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (m *Manager) reportAgentCompletion(runtime *threadRuntime, result string, runErr error) {
	child := runtime.snapshotThread()
	status := child.Status
	completion := agentCompletion{Agent: agentFromThread(child), Status: status, Result: boundedAgentResult(result)}
	if runErr != nil {
		completion.Error = runErr.Error()
	}
	m.emit(context.Background(), runtime, "agent_completed", completion)
	parentRuntime, err := m.getRuntime(context.Background(), child.ParentID)
	if err == nil {
		m.emit(context.Background(), parentRuntime, "agent_completed", completion)
	}
	settings, settingsErr := m.runtimeSettings(context.Background(), teamRootID(child))
	if settingsErr != nil || !settings.Subagents.AutoReport || child.ParentID == "" {
		m.notifyTeam(teamRootID(child))
		return
	}
	summary := fmt.Sprintf("Agent %s (%s) %s.", firstNonEmpty(child.AgentName, child.ID), child.ID, status)
	if completion.Result != "" {
		summary += "\n\n" + completion.Result
	}
	if completion.Error != "" && !errors.Is(runErr, context.Canceled) {
		summary += "\n\nError: " + completion.Error
	}
	if _, err := m.SendAgentMessage(context.Background(), child.ID, child.ParentID, summary, true); err != nil {
		m.emit(context.Background(), runtime, "agent_report_error", map[string]string{"error": err.Error()})
	}
	m.notifyTeam(teamRootID(child))
}

func boundedAgentResult(value string) string {
	value = strings.TrimSpace(value)
	const limit = 32 << 10
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n[agent result truncated]"
}
