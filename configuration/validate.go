package configuration

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

var configName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// Validate checks both global settings and every effective project view.
func Validate(config Config) error {
	if config.Version != CurrentVersion {
		return fmt.Errorf("configuration: unsupported version %d", config.Version)
	}
	if config.Revision == 0 {
		return errors.New("configuration: revision must be positive")
	}
	if err := validateSettings(config.Global, "global"); err != nil {
		return err
	}
	for id, project := range config.Projects {
		if !configName.MatchString(id) {
			return fmt.Errorf("configuration: invalid project id %q", id)
		}
		if strings.TrimSpace(project.Folder) == "" || !filepath.IsAbs(project.Folder) {
			return fmt.Errorf("configuration: project %q folder must be absolute", id)
		}
		if err := validatePatch(project.Settings, id); err != nil {
			return err
		}
		effective, _ := config.Effective(id)
		if err := validateSettings(effective, "project "+id); err != nil {
			return err
		}
	}
	return nil
}

func validatePatch(patch SettingsPatch, projectID string) error {
	if patch.MCP != nil {
		removed := stringSet(patch.MCP.RemoveServers)
		if err := validateNames(patch.MCP.RemoveServers, "removed MCP server"); err != nil {
			return err
		}
		for name := range patch.MCP.Servers {
			if _, ok := removed[name]; ok {
				return fmt.Errorf("configuration: project %q both patches and removes MCP server %q", projectID, name)
			}
		}
	}
	if patch.Extensions != nil {
		removed := stringSet(patch.Extensions.RemoveSources)
		if err := validateNames(patch.Extensions.RemoveSources, "removed extension source"); err != nil {
			return err
		}
		for name := range patch.Extensions.Sources {
			if _, ok := removed[name]; ok {
				return fmt.Errorf("configuration: project %q both patches and removes extension %q", projectID, name)
			}
		}
	}
	return nil
}

func validateNames(values []string, label string) error {
	if err := uniqueNonEmpty(values, label+" names"); err != nil {
		return err
	}
	for _, value := range values {
		if !configName.MatchString(value) {
			return fmt.Errorf("configuration: invalid %s name %q", label, value)
		}
	}
	return nil
}

func validateSettings(settings Settings, scope string) error {
	if strings.TrimSpace(settings.Model.Provider) == "" || strings.TrimSpace(settings.Model.ID) == "" {
		return fmt.Errorf("configuration: %s model provider and id are required", scope)
	}
	if settings.Model.ContextWindow < 0 {
		return fmt.Errorf("configuration: %s context window cannot be negative", scope)
	}
	if !validThinking(settings.Thinking.Level) {
		return fmt.Errorf("configuration: %s invalid thinking level %q", scope, settings.Thinking.Level)
	}
	if err := validateTools(settings.Tools, scope); err != nil {
		return err
	}
	if !validQueue(settings.Queues.SteeringMode) || !validQueue(settings.Queues.FollowUpMode) {
		return fmt.Errorf("configuration: %s invalid queue mode", scope)
	}
	if settings.Queues.MaxPending <= 0 {
		return fmt.Errorf("configuration: %s max pending must be positive", scope)
	}
	if err := validateSkills(settings.Skills, scope); err != nil {
		return err
	}
	if err := validateMCP(settings.MCP, scope); err != nil {
		return err
	}
	if err := validateExtensions(settings.Extensions, scope); err != nil {
		return err
	}
	if err := validateSubagents(settings.Subagents, scope); err != nil {
		return err
	}
	if err := validateLaunchers(settings.Launchers, scope); err != nil {
		return err
	}
	if err := validateDesktop(settings.Desktop, scope); err != nil {
		return err
	}
	if strings.TrimSpace(settings.StandaloneChat.Model) == "" {
		return fmt.Errorf("configuration: %s standalone chat model is required", scope)
	}
	if !validThinking(settings.StandaloneChat.Thinking) {
		return fmt.Errorf("configuration: %s invalid standalone chat thinking level", scope)
	}
	return uniqueNonEmpty(settings.StandaloneChat.Tools, scope+" standalone chat tools")
}

func validateTools(settings ToolSettings, scope string) error {
	if err := uniqueNonEmpty(settings.Enabled, scope+" tools"); err != nil {
		return err
	}
	if settings.Sandbox != SandboxStrict && settings.Sandbox != SandboxWorkspace && settings.Sandbox != SandboxOff {
		return fmt.Errorf("configuration: %s invalid sandbox mode %q", scope, settings.Sandbox)
	}
	if !validApproval(settings.Approval) {
		return fmt.Errorf("configuration: %s invalid tool approval %q", scope, settings.Approval)
	}
	return nil
}

func validateSkills(settings SkillSettings, scope string) error {
	if len(settings.Roots) == 0 {
		return fmt.Errorf("configuration: %s requires a skill root", scope)
	}
	if err := uniqueNonEmpty(settings.Roots, scope+" skill roots"); err != nil {
		return err
	}
	for _, root := range settings.Roots {
		if !filepath.IsAbs(root) {
			return fmt.Errorf("configuration: %s skill roots must be absolute", scope)
		}
	}
	if err := uniqueNonEmpty(settings.Disabled, scope+" disabled skills"); err != nil {
		return err
	}
	if !validTrust(settings.Trust) {
		return fmt.Errorf("configuration: %s invalid skill trust %q", scope, settings.Trust)
	}
	return nil
}

func validateMCP(settings MCPSettings, scope string) error {
	for name, server := range settings.Servers {
		if !configName.MatchString(name) {
			return fmt.Errorf("configuration: %s invalid MCP server name %q", scope, name)
		}
		if !validApproval(server.Approval) {
			return fmt.Errorf("configuration: %s MCP server %q has invalid approval", scope, name)
		}
		if err := uniqueNonEmpty(server.EnabledTools, scope+" MCP enabled tools"); err != nil {
			return err
		}
		for _, approval := range server.ToolApprovals {
			if !validApproval(approval) {
				return fmt.Errorf("configuration: %s MCP server %q has invalid tool approval", scope, name)
			}
		}
		if err := validateSecretKeys(server.Env, server.SecretEnv, "environment", name); err != nil {
			return err
		}
		if err := validateProcessValues(server.Command, server.Args, server.Env, "MCP server "+name); err != nil {
			return err
		}
		if err := validateSecretKeys(server.Headers, server.SecretHeaders, "header", name); err != nil {
			return err
		}
		switch server.Transport {
		case MCPStdio:
			if strings.TrimSpace(server.Command) == "" {
				return fmt.Errorf("configuration: MCP server %q command is required", name)
			}
			if server.URL != "" {
				return fmt.Errorf("configuration: MCP server %q stdio transport cannot set URL", name)
			}
		case MCPStreamableHTTP:
			parsed, err := url.Parse(server.URL)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return fmt.Errorf("configuration: MCP server %q requires an HTTP(S) URL", name)
			}
			if server.Command != "" {
				return fmt.Errorf("configuration: MCP server %q HTTP transport cannot set command", name)
			}
			if len(server.Args) > 0 || server.InheritEnv {
				return fmt.Errorf("configuration: MCP server %q HTTP transport cannot set process options", name)
			}
		default:
			return fmt.Errorf("configuration: MCP server %q has invalid transport %q", name, server.Transport)
		}
	}
	return nil
}

func validateSecretKeys(values map[string]string, keys []string, kind, server string) error {
	if err := uniqueNonEmpty(keys, "MCP secret "+kind+" keys"); err != nil {
		return err
	}
	for _, key := range keys {
		if _, ok := values[key]; !ok {
			return fmt.Errorf("configuration: MCP server %q secret %s %q is not configured", server, kind, key)
		}
	}
	return nil
}
