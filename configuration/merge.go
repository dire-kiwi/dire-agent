package configuration

// Effective returns a deep copy of global settings with a project's overrides
// applied. The boolean reports whether the project exists.
func (c Config) Effective(projectID string) (Settings, bool) {
	settings := cloneSettings(c.Global)
	project, ok := c.Projects[projectID]
	if !ok {
		return settings, false
	}
	applyPatch(&settings, project.Settings)
	return settings, true
}

func applyPatch(dst *Settings, patch SettingsPatch) {
	if p := patch.Model; p != nil {
		if p.Provider != nil {
			dst.Model.Provider = *p.Provider
		}
		if p.ID != nil {
			dst.Model.ID = *p.ID
		}
		if p.ContextWindow != nil {
			dst.Model.ContextWindow = *p.ContextWindow
		}
	}
	if p := patch.Thinking; p != nil && p.Level != nil {
		dst.Thinking.Level = *p.Level
	}
	if p := patch.Tools; p != nil {
		if p.Enabled != nil {
			dst.Tools.Enabled = cloneStrings(*p.Enabled)
		}
		if p.Sandbox != nil {
			dst.Tools.Sandbox = *p.Sandbox
		}
		if p.Approval != nil {
			dst.Tools.Approval = *p.Approval
		}
	}
	if p := patch.Queues; p != nil {
		if p.SteeringMode != nil {
			dst.Queues.SteeringMode = *p.SteeringMode
		}
		if p.FollowUpMode != nil {
			dst.Queues.FollowUpMode = *p.FollowUpMode
		}
		if p.MaxPending != nil {
			dst.Queues.MaxPending = *p.MaxPending
		}
	}
	if p := patch.Skills; p != nil {
		if p.Roots != nil {
			dst.Skills.Roots = cloneStrings(*p.Roots)
		}
		if p.Disabled != nil {
			dst.Skills.Disabled = cloneStrings(*p.Disabled)
		}
		if p.Trust != nil {
			dst.Skills.Trust = *p.Trust
		}
	}
	if patch.Launchers != nil {
		dst.Launchers = cloneProjectLaunchers(*patch.Launchers)
	}
	applyMCPPatch(&dst.MCP, patch.MCP)
	applyExtensionPatch(&dst.Extensions, patch.Extensions)
	applySubagentPatch(&dst.Subagents, patch.Subagents)
	applyDesktopPatch(&dst.Desktop, patch.Desktop)
	applyStandalonePatch(&dst.StandaloneChat, patch.StandaloneChat)
}

func applySubagentPatch(dst *SubagentSettings, patch *SubagentPatch) {
	if patch == nil {
		return
	}
	if patch.Enabled != nil {
		dst.Enabled = *patch.Enabled
	}
	if patch.MaxDepth != nil {
		dst.MaxDepth = *patch.MaxDepth
	}
	if patch.MaxChildren != nil {
		dst.MaxChildren = *patch.MaxChildren
	}
	if patch.MaxConcurrent != nil {
		dst.MaxConcurrent = *patch.MaxConcurrent
	}
	if patch.AllowSiblingMessages != nil {
		dst.AllowSiblingMessages = *patch.AllowSiblingMessages
	}
	if patch.AutoReport != nil {
		dst.AutoReport = *patch.AutoReport
	}
	if routing := patch.ModelRouting; routing != nil {
		if routing.ControllerModel != nil {
			dst.ModelRouting.ControllerModel = *routing.ControllerModel
		}
		if routing.ControllerThinking != nil {
			dst.ModelRouting.ControllerThinking = *routing.ControllerThinking
		}
		if routing.Prompt != nil {
			dst.ModelRouting.Prompt = *routing.Prompt
		}
		if routing.AllowedModels != nil {
			dst.ModelRouting.AllowedModels = cloneStrings(*routing.AllowedModels)
		}
	}
	if patch.Profiles != nil {
		dst.Profiles = cloneAgentProfiles(*patch.Profiles)
	}
}

func applyMCPPatch(dst *MCPSettings, patch *MCPPatch) {
	if patch == nil {
		return
	}
	if dst.Servers == nil {
		dst.Servers = make(map[string]MCPServer)
	}
	for name, serverPatch := range patch.Servers {
		server := dst.Servers[name]
		applyMCPServerPatch(&server, serverPatch)
		dst.Servers[name] = server
	}
	for _, name := range patch.RemoveServers {
		delete(dst.Servers, name)
	}
}

func applyMCPServerPatch(dst *MCPServer, patch MCPServerPatch) {
	if patch.Transport != nil {
		dst.Transport = *patch.Transport
	}
	if patch.Command != nil {
		dst.Command = *patch.Command
	}
	if patch.Args != nil {
		dst.Args = cloneStrings(*patch.Args)
	}
	if patch.InheritEnv != nil {
		dst.InheritEnv = *patch.InheritEnv
	}
	if patch.URL != nil {
		dst.URL = *patch.URL
	}
	if patch.Env != nil {
		dst.Env = cloneStringMap(*patch.Env)
	}
	if patch.SecretEnv != nil {
		dst.SecretEnv = cloneStrings(*patch.SecretEnv)
	}
	if patch.Headers != nil {
		dst.Headers = cloneStringMap(*patch.Headers)
	}
	if patch.SecretHeaders != nil {
		dst.SecretHeaders = cloneStrings(*patch.SecretHeaders)
	}
	if patch.EnabledTools != nil {
		dst.EnabledTools = cloneStrings(*patch.EnabledTools)
	}
	if patch.Approval != nil {
		dst.Approval = *patch.Approval
	}
	if patch.ToolApprovals != nil {
		dst.ToolApprovals = make(map[string]ApprovalMode, len(*patch.ToolApprovals))
		for name, approval := range *patch.ToolApprovals {
			dst.ToolApprovals[name] = approval
		}
	}
	if patch.Enabled != nil {
		dst.Enabled = *patch.Enabled
	}
}

func applyExtensionPatch(dst *ExtensionSettings, patch *ExtensionPatch) {
	if patch == nil {
		return
	}
	if dst.Sources == nil {
		dst.Sources = make(map[string]ExtensionSource)
	}
	for name, sourcePatch := range patch.Sources {
		source := dst.Sources[name]
		if sourcePatch.Kind != nil {
			source.Kind = *sourcePatch.Kind
		}
		if sourcePatch.Location != nil {
			source.Location = *sourcePatch.Location
		}
		if sourcePatch.Ref != nil {
			source.Ref = *sourcePatch.Ref
		}
		if sourcePatch.Trust != nil {
			source.Trust = *sourcePatch.Trust
		}
		if sourcePatch.Enabled != nil {
			source.Enabled = *sourcePatch.Enabled
		}
		if sourcePatch.Command != nil {
			source.Command = *sourcePatch.Command
		}
		if sourcePatch.Args != nil {
			source.Args = cloneStrings(*sourcePatch.Args)
		}
		if sourcePatch.Env != nil {
			source.Env = cloneStringMap(*sourcePatch.Env)
		}
		if sourcePatch.SecretEnv != nil {
			source.SecretEnv = cloneStrings(*sourcePatch.SecretEnv)
		}
		if sourcePatch.InheritEnv != nil {
			source.InheritEnv = *sourcePatch.InheritEnv
		}
		dst.Sources[name] = source
	}
	for _, name := range patch.RemoveSources {
		delete(dst.Sources, name)
	}
	if patch.AllowUnsigned != nil {
		dst.AllowUnsigned = *patch.AllowUnsigned
	}
}

func applyDesktopPatch(dst *DesktopSettings, patch *DesktopPatch) {
	if patch == nil {
		return
	}
	if patch.CodexHome != nil {
		dst.CodexHome = *patch.CodexHome
	}
	if patch.DesktopConfig != nil {
		dst.DesktopConfig = *patch.DesktopConfig
	}
	if patch.SyncMode != nil {
		dst.SyncMode = *patch.SyncMode
	}
	if patch.SyncSkills != nil {
		dst.SyncSkills = *patch.SyncSkills
	}
	if patch.SyncMCP != nil {
		dst.SyncMCP = *patch.SyncMCP
	}
	if patch.SyncExtensions != nil {
		dst.SyncExtensions = *patch.SyncExtensions
	}
	if patch.WatchForChanges != nil {
		dst.WatchForChanges = *patch.WatchForChanges
	}
}

func applyStandalonePatch(dst *StandaloneChatSettings, patch *StandaloneChatPatch) {
	if patch == nil {
		return
	}
	if patch.Model != nil {
		dst.Model = *patch.Model
	}
	if patch.Thinking != nil {
		dst.Thinking = *patch.Thinking
	}
	if patch.Tools != nil {
		dst.Tools = cloneStrings(*patch.Tools)
	}
	if patch.Instructions != nil {
		dst.Instructions = *patch.Instructions
	}
	if patch.PersistHistory != nil {
		dst.PersistHistory = *patch.PersistHistory
	}
}
