package chatui

import (
	"fmt"
	"strings"
)

type parsedInput struct {
	kind     string
	argument string
	err      error
}

type slashCommandDefinition struct {
	name            string
	description     string
	acceptsArgument bool
}

var slashCommandCatalog = []slashCommandDefinition{
	{name: "steer", description: "guide the active run", acceptsArgument: true},
	{name: "follow-up", description: "queue the next turn", acceptsArgument: true},
	{name: "abort", description: "cancel the active run"},
	{name: "agents", description: "list the agent tree"},
	{name: "spawn", description: "spawn a child agent", acceptsArgument: true},
	{name: "message", description: "message a child agent", acceptsArgument: true},
	{name: "wait", description: "wait for child agents", acceptsArgument: true},
	{name: "interrupt", description: "interrupt a child agent", acceptsArgument: true},
	{name: "delete-agent", description: "delete an idle child", acceptsArgument: true},
	{name: "commands", description: "list extension commands"},
	{name: "model", description: "show or change the model", acceptsArgument: true},
	{name: "thinking", description: "show or change reasoning", acceptsArgument: true},
	{name: "name", description: "show or rename the conversation", acceptsArgument: true},
	{name: "folders", description: "show sandbox folders"},
	{name: "folder-add", description: "include an absolute folder", acceptsArgument: true},
	{name: "folder-remove", description: "remove an included folder", acceptsArgument: true},
	{name: "status", description: "show state and token usage"},
	{name: "clear", description: "clear the local transcript"},
	{name: "help", description: "show command help"},
	{name: "quit", description: "exit the client"},
}

func slashCommandSuggestions(input string) []slashCommandDefinition {
	value := strings.TrimLeft(input, " \t")
	if !strings.HasPrefix(value, "/") || strings.Contains(value, "\n") || strings.ContainsAny(value[1:], " \t") {
		return nil
	}
	query := strings.ToLower(strings.TrimPrefix(value, "/"))
	matches := make([]slashCommandDefinition, 0, len(slashCommandCatalog))
	for _, command := range slashCommandCatalog {
		if strings.HasPrefix(command.name, query) {
			matches = append(matches, command)
		}
	}
	if len(matches) == 1 && matches[0].name == query {
		return nil
	}
	return matches
}

func completeSlashCommand(command slashCommandDefinition) string {
	value := "/" + command.name
	if command.acceptsArgument {
		value += " "
	}
	return value
}

func parseInput(input string) parsedInput {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return parsedInput{kind: "prompt", argument: input}
	}
	fields := strings.Fields(input)
	name := strings.TrimPrefix(strings.ToLower(fields[0]), "/")
	argument := strings.TrimSpace(strings.TrimPrefix(input, fields[0]))
	if strings.HasPrefix(name, "skill:") {
		// Explicit Agent Skills syntax is a model prompt, not a local UI
		// command. The daemon expands trusted skill instructions per run.
		return parsedInput{kind: "prompt", argument: input}
	}
	if strings.HasPrefix(name, "ext:") {
		return parsedInput{kind: "capability-command", argument: strings.TrimSpace(strings.TrimPrefix(input, "/"))}
	}
	requireArgument := func(kind string) parsedInput {
		if argument == "" {
			return parsedInput{err: fmt.Errorf("/%s requires text", kind)}
		}
		return parsedInput{kind: kind, argument: argument}
	}
	switch name {
	case "steer":
		return requireArgument("steer")
	case "follow-up", "followup", "follow_up", "queue":
		return requireArgument("follow-up")
	case "abort", "stop":
		return parsedInput{kind: "abort"}
	case "agents":
		return parsedInput{kind: "agents"}
	case "spawn":
		return requireArgument("spawn")
	case "message", "msg":
		return requireArgument("message")
	case "wait":
		return parsedInput{kind: "wait", argument: argument}
	case "interrupt":
		return requireArgument("interrupt")
	case "delete-agent":
		return requireArgument("delete-agent")
	case "commands":
		return parsedInput{kind: "capability-commands"}
	case "model":
		return parsedInput{kind: "model", argument: argument}
	case "thinking", "think":
		return parsedInput{kind: "thinking", argument: strings.ToLower(argument)}
	case "name", "rename":
		return parsedInput{kind: "name", argument: argument}
	case "folders":
		return parsedInput{kind: "folders"}
	case "folder-add":
		return requireArgument("folder-add")
	case "folder-remove":
		return requireArgument("folder-remove")
	case "status", "project", "thread":
		return parsedInput{kind: "status"}
	case "clear":
		return parsedInput{kind: "clear"}
	case "help", "?":
		return parsedInput{kind: "help"}
	case "quit", "exit", "q":
		return parsedInput{kind: "quit"}
	default:
		return parsedInput{err: fmt.Errorf("unknown command /%s; type /help", name)}
	}
}

const helpText = `Commands:
/steer TEXT       inject guidance into the active run
/follow-up TEXT   queue the next turn (alias: /followup)
/abort            cancel the active run
/agents           list this conversation's agent tree
/spawn NAME TASK  spawn a child; use NAME [PROFILE] [--mode model-router] -- TASK for options
/message ID TEXT  send and wake an agent (alias: /msg)
/wait [ID ...]    wait up to 30 seconds for child agents
/interrupt ID     cancel a running agent
/delete-agent ID  delete an idle leaf agent
/commands         list extension-provided slash commands
/ext:ID:CMD ARGS  execute an extension command
/model [MODEL]    show or change the model while idle
/thinking [LEVEL] show or change reasoning: none|minimal|low|medium|high|xhigh|max
/name [NAME]      show or rename the conversation
/folders          show the main and additional sandbox folders
/folder-add PATH  include an absolute folder without changing the main folder
/folder-remove PATH
                  remove an included sandbox folder
/status           show conversation scope, state, settings, and usage
/clear            clear the local transcript view
/help             show this help
/quit             exit the client (the daemon keeps running)`
