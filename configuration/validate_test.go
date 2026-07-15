package configuration

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateMCPTransportsAndSecretMetadata(t *testing.T) {
	tests := []struct {
		name   string
		server MCPServer
		want   string
	}{
		{name: "stdio command", server: MCPServer{Transport: MCPStdio, Approval: ApprovalNever}, want: "command is required"},
		{name: "stdio URL", server: MCPServer{Transport: MCPStdio, Command: "server", URL: "https://example.test", Approval: ApprovalNever}, want: "cannot set URL"},
		{name: "HTTP URL", server: MCPServer{Transport: MCPStreamableHTTP, URL: "file:///tmp/mcp", Approval: ApprovalNever}, want: "HTTP(S) URL"},
		{name: "HTTP command", server: MCPServer{Transport: MCPStreamableHTTP, URL: "https://example.test", Command: "server", Approval: ApprovalNever}, want: "cannot set command"},
		{name: "secret metadata", server: MCPServer{Transport: MCPStdio, Command: "server", Env: map[string]string{}, SecretEnv: []string{"TOKEN"}, Approval: ApprovalNever}, want: "not configured"},
		{name: "approval", server: MCPServer{Transport: MCPStdio, Command: "server", Approval: "sometimes"}, want: "invalid approval"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := DefaultConfig(t.TempDir())
			config.Global.MCP.Servers["test"] = test.server
			err := Validate(config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateProjectUsesEffectiveSettings(t *testing.T) {
	config := DefaultConfig(t.TempDir())
	invalid := SandboxMode("escape")
	config.Projects["project"] = ProjectOverride{
		Folder:   filepath.Join(t.TempDir(), "project"),
		Settings: SettingsPatch{Tools: &ToolPatch{Sandbox: &invalid}},
	}
	if err := Validate(config); err == nil || !strings.Contains(err.Error(), "project project") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateSubagentModelRouting(t *testing.T) {
	missing := DefaultConfig(t.TempDir())
	missing.Global.Subagents.ModelRouting = SubagentModelRoutingSettings{}
	if err := Validate(missing); err == nil || !strings.Contains(err.Error(), "controller model is required") {
		t.Fatalf("missing v2 model routing error = %v", err)
	}

	tests := []struct {
		name    string
		routing SubagentModelRoutingSettings
		want    string
	}{
		{
			name: "controller thinking only",
			routing: SubagentModelRoutingSettings{
				ControllerThinking: ThinkingXHigh,
			},
			want: "controller model is required",
		},
		{
			name:    "controller only",
			routing: SubagentModelRoutingSettings{ControllerModel: "router"},
			want:    "prompt is required",
		},
		{
			name:    "prompt only",
			routing: SubagentModelRoutingSettings{Prompt: "route the task"},
			want:    "controller model is required",
		},
		{
			name:    "allowed models only",
			routing: SubagentModelRoutingSettings{AllowedModels: []string{"worker"}},
			want:    "controller model is required",
		},
		{
			name: "blank controller",
			routing: SubagentModelRoutingSettings{
				ControllerModel: "  ", Prompt: "route the task", AllowedModels: []string{"worker"},
			},
			want: "controller model is required",
		},
		{
			name: "blank prompt",
			routing: SubagentModelRoutingSettings{
				ControllerModel: "router", Prompt: "\t", AllowedModels: []string{"worker"},
			},
			want: "prompt is required",
		},
		{
			name: "invalid controller thinking",
			routing: SubagentModelRoutingSettings{
				ControllerModel: "router", ControllerThinking: "extreme",
				Prompt: "route the task", AllowedModels: []string{"worker"},
			},
			want: "controller thinking is invalid",
		},
		{
			name: "missing allowed models",
			routing: SubagentModelRoutingSettings{
				ControllerModel: "router", Prompt: "route the task",
			},
			want: "at least one allowed model",
		},
		{
			name: "explicit empty allowed models",
			routing: SubagentModelRoutingSettings{
				ControllerModel: "router", Prompt: "route the task", AllowedModels: []string{},
			},
			want: "at least one allowed model",
		},
		{
			name: "blank allowed model",
			routing: SubagentModelRoutingSettings{
				ControllerModel: "router", Prompt: "route the task", AllowedModels: []string{"worker", " "},
			},
			want: "cannot contain an empty value",
		},
		{
			name: "duplicate allowed model",
			routing: SubagentModelRoutingSettings{
				ControllerModel: "router", Prompt: "route the task", AllowedModels: []string{"worker", "worker"},
			},
			want: "contains duplicate",
		},
		{
			name: "duplicate normalized allowed model",
			routing: SubagentModelRoutingSettings{
				ControllerModel: "router", Prompt: "route the task", AllowedModels: []string{"worker", " worker "},
			},
			want: "contains duplicate",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := DefaultConfig(t.TempDir())
			config.Global.Subagents.ModelRouting = test.routing
			err := Validate(config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateAcceptsXHighThinking(t *testing.T) {
	config := DefaultConfig(t.TempDir())
	config.Global.Thinking.Level = ThinkingXHigh
	config.Global.StandaloneChat.Thinking = ThinkingXHigh
	profile := config.Global.Subagents.Profiles["general"]
	profile.Thinking = ThinkingXHigh
	config.Global.Subagents.Profiles["general"] = profile
	config.Global.Subagents.ModelRouting.ControllerThinking = ThinkingXHigh
	if err := Validate(config); err != nil {
		t.Fatalf("xhigh thinking should be valid: %v", err)
	}
}

func TestValidateRejectsReservedModelRouterControllerProfile(t *testing.T) {
	config := DefaultConfig(t.TempDir())
	config.Global.Subagents.Profiles = map[string]AgentProfile{
		ModelRouterControllerProfile: {
			Description: "Must remain reserved for the runtime controller.",
		},
	}
	err := Validate(config)
	if err == nil || !strings.Contains(err.Error(), `profile "`+ModelRouterControllerProfile+`" is reserved`) {
		t.Fatalf("error = %v", err)
	}
}

func TestMCPAndExtensionRemoval(t *testing.T) {
	config := DefaultConfig(t.TempDir())
	config.Global.MCP.Servers["remove"] = validStdioServer()
	config.Global.Extensions.Sources["remove"] = ExtensionSource{
		Kind: ExtensionRegistry, Location: "example/remove", Trust: TrustPrompt,
	}
	config.Projects["project"] = ProjectOverride{
		Folder: filepath.Join(t.TempDir(), "project"),
		Settings: SettingsPatch{
			MCP:        &MCPPatch{RemoveServers: []string{"remove"}},
			Extensions: &ExtensionPatch{RemoveSources: []string{"remove"}},
		},
	}
	effective, _ := config.Effective("project")
	if _, ok := effective.MCP.Servers["remove"]; ok {
		t.Fatal("MCP server was not removed")
	}
	if _, ok := effective.Extensions.Sources["remove"]; ok {
		t.Fatal("extension source was not removed")
	}
}

func TestValidateRejectsPatchAndRemovalOfSameEntry(t *testing.T) {
	config := DefaultConfig(t.TempDir())
	config.Projects["project"] = ProjectOverride{
		Folder: filepath.Join(t.TempDir(), "project"),
		Settings: SettingsPatch{MCP: &MCPPatch{
			Servers:       map[string]MCPServerPatch{"same": {}},
			RemoveServers: []string{"same"},
		}},
	}
	if err := Validate(config); err == nil || !strings.Contains(err.Error(), "both patches and removes") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateProjectLaunchers(t *testing.T) {
	tests := []struct {
		name      string
		launchers []ProjectLauncher
		want      string
	}{
		{
			name: "duplicate id",
			launchers: []ProjectLauncher{
				{ID: "same", Label: "One", Kind: LauncherTerminal},
				{ID: "same", Label: "Two", Kind: LauncherTerminal},
			},
			want: "duplicate launcher id",
		},
		{name: "invalid id", launchers: []ProjectLauncher{{ID: "bad id", Label: "Bad", Kind: LauncherTerminal}}, want: "invalid launcher id"},
		{name: "invalid kind", launchers: []ProjectLauncher{{ID: "bad", Label: "Bad", Kind: "browser"}}, want: "invalid kind"},
		{name: "desktop command", launchers: []ProjectLauncher{{ID: "code", Label: "Code", Kind: LauncherDesktop}}, want: "command is required"},
		{name: "shell arguments", launchers: []ProjectLauncher{{ID: "shell", Label: "Shell", Kind: LauncherTerminal, Args: []string{"-l"}}}, want: "cannot set arguments"},
		{
			name: "duplicate shortcut",
			launchers: []ProjectLauncher{
				{ID: "one", Label: "One", Kind: LauncherTerminal, Shortcut: "mod+shift+x"},
				{ID: "two", Label: "Two", Kind: LauncherTerminal, Shortcut: " shift + MOD + X "},
			},
			want: "same shortcut",
		},
		{name: "shortcut without modifier", launchers: []ProjectLauncher{{ID: "one", Label: "One", Kind: LauncherTerminal, Shortcut: "x"}}, want: "invalid shortcut"},
		{name: "shortcut without key", launchers: []ProjectLauncher{{ID: "one", Label: "One", Kind: LauncherTerminal, Shortcut: "mod+shift"}}, want: "invalid shortcut"},
		{name: "shortcut with two keys", launchers: []ProjectLauncher{{ID: "one", Label: "One", Kind: LauncherTerminal, Shortcut: "mod+x+y"}}, want: "invalid shortcut"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := DefaultConfig(t.TempDir())
			config.Global.Launchers = test.launchers
			err := Validate(config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}

	config := DefaultConfig(t.TempDir())
	config.Global.Launchers = []ProjectLauncher{}
	if err := Validate(config); err != nil {
		t.Fatalf("explicitly disabling every launcher: %v", err)
	}
}
