package chatui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m model) capabilityCommand(input parsedInput) tea.Cmd {
	return agentRequest(input.kind, func() (string, error) {
		if input.kind == "capability-commands" {
			commands, err := m.api.CapabilityCommands(m.ctx, m.thread.ID)
			if err != nil {
				return "", err
			}
			if len(commands) == 0 {
				return "No enabled extension commands.", nil
			}
			lines := []string{"Extension commands:"}
			for _, command := range commands {
				line := "/" + command.Name
				if command.Description != "" {
					line += " — " + command.Description
				}
				lines = append(lines, line)
			}
			return strings.Join(lines, "\n"), nil
		}
		name, arguments, _ := strings.Cut(strings.TrimSpace(input.argument), " ")
		result, err := m.api.ExecuteCapabilityCommand(m.ctx, m.thread.ID, name, strings.TrimSpace(arguments))
		if err != nil {
			return "", err
		}
		output := strings.TrimSpace(result.Output)
		if output == "" {
			output = "command completed"
		}
		if result.Prompt != "" {
			output += "\nModel prompt queued."
		}
		if result.IsError {
			return "", fmt.Errorf("extension command: %s", output)
		}
		return output, nil
	})
}
