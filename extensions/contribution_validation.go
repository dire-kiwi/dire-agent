package extensions

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func validateRegistration(registration Registration, limits Limits) error {
	if len(registration.Commands) > limits.MaxCommands || len(registration.Hooks) > limits.MaxHooks || len(registration.UI) > limits.MaxUIContributions {
		return fmt.Errorf("%w: contribution count exceeds configured limit", errInvalidRegistration)
	}
	seenCommands := map[string]bool{}
	for _, command := range registration.Commands {
		if strings.TrimSpace(command.Name) == "" || seenCommands[command.Name] {
			return fmt.Errorf("%w: invalid or duplicate command %q", errInvalidRegistration, command.Name)
		}
		seenCommands[command.Name] = true
	}
	totalPromptBytes := 0
	seenFragments := map[string]bool{}
	for _, fragment := range registration.PromptFragments {
		totalPromptBytes += len(fragment.Text)
		if fragment.ID == "" || fragment.Text == "" || seenFragments[fragment.ID] {
			return fmt.Errorf("%w: invalid or duplicate prompt fragment %q", errInvalidRegistration, fragment.ID)
		}
		seenFragments[fragment.ID] = true
	}
	if totalPromptBytes > limits.MaxPromptBytes {
		return fmt.Errorf("%w: prompt fragments exceed %d bytes", errInvalidRegistration, limits.MaxPromptBytes)
	}
	seenHooks := map[string]bool{}
	for _, hook := range registration.Hooks {
		if hook.ID == "" || !validHookEvent(hook.Event) || seenHooks[hook.ID] {
			return fmt.Errorf("%w: invalid or duplicate hook %q", errInvalidRegistration, hook.ID)
		}
		seenHooks[hook.ID] = true
	}
	if len(registration.SettingsSchema) > 0 && !jsonObject(registration.SettingsSchema) {
		return fmt.Errorf("%w: settings_schema must be a JSON object", errInvalidRegistration)
	}
	seenUI := map[string]bool{}
	for _, item := range registration.UI {
		if item.ID == "" || item.Kind == "" || seenUI[item.ID] || len(item.Schema) > 0 && !jsonObject(item.Schema) {
			return fmt.Errorf("%w: invalid UI contribution %q", errInvalidRegistration, item.ID)
		}
		seenUI[item.ID] = true
	}
	return nil
}

func jsonObject(raw json.RawMessage) bool {
	var value map[string]any
	return json.Unmarshal(raw, &value) == nil && value != nil
}

func cloneRegistration(input Registration) Registration {
	result := input
	result.Commands = append([]CommandSpec(nil), input.Commands...)
	result.PromptFragments = append([]PromptFragment(nil), input.PromptFragments...)
	result.Hooks = append([]HookSpec(nil), input.Hooks...)
	result.SettingsSchema = append(json.RawMessage(nil), input.SettingsSchema...)
	result.UI = append([]UIContribution(nil), input.UI...)
	for index := range result.UI {
		result.UI[index].Schema = append(json.RawMessage(nil), result.UI[index].Schema...)
	}
	result.ToolRenderers = append([]ToolRenderer(nil), input.ToolRenderers...)
	sort.SliceStable(result.Commands, func(i, j int) bool { return result.Commands[i].Name < result.Commands[j].Name })
	sort.SliceStable(result.PromptFragments, func(i, j int) bool { return result.PromptFragments[i].Priority < result.PromptFragments[j].Priority })
	return result
}
