package configuration

func cloneSettings(in Settings) Settings {
	out := in
	out.Tools.Enabled = cloneStrings(in.Tools.Enabled)
	out.Skills.Roots = cloneStrings(in.Skills.Roots)
	out.Skills.Disabled = cloneStrings(in.Skills.Disabled)
	out.StandaloneChat.Tools = cloneStrings(in.StandaloneChat.Tools)
	out.Launchers = cloneProjectLaunchers(in.Launchers)
	out.MCP.Servers = make(map[string]MCPServer, len(in.MCP.Servers))
	for name, server := range in.MCP.Servers {
		out.MCP.Servers[name] = cloneMCPServer(server)
	}
	out.Extensions.Sources = make(map[string]ExtensionSource, len(in.Extensions.Sources))
	for name, source := range in.Extensions.Sources {
		out.Extensions.Sources[name] = cloneExtensionSource(source)
	}
	out.Subagents.ModelRouting.AllowedModels = cloneStrings(in.Subagents.ModelRouting.AllowedModels)
	out.Subagents.Profiles = cloneAgentProfiles(in.Subagents.Profiles)
	return out
}

func cloneProjectLaunchers(input []ProjectLauncher) []ProjectLauncher {
	if input == nil {
		return nil
	}
	result := make([]ProjectLauncher, len(input))
	copy(result, input)
	for index := range result {
		if input[index].Args != nil {
			result[index].Args = append([]string{}, input[index].Args...)
		}
	}
	return result
}

func cloneAgentProfiles(input map[string]AgentProfile) map[string]AgentProfile {
	if input == nil {
		return nil
	}
	result := make(map[string]AgentProfile, len(input))
	for name, profile := range input {
		if profile.Tools != nil {
			profile.Tools = append([]string{}, profile.Tools...)
		}
		result[name] = profile
	}
	return result
}

func cloneExtensionSource(in ExtensionSource) ExtensionSource {
	out := in
	out.Args = cloneStrings(in.Args)
	out.Env = cloneStringMap(in.Env)
	out.SecretEnv = cloneStrings(in.SecretEnv)
	return out
}

func cloneMCPServer(in MCPServer) MCPServer {
	out := in
	out.Args = cloneStrings(in.Args)
	out.SecretEnv = cloneStrings(in.SecretEnv)
	out.SecretHeaders = cloneStrings(in.SecretHeaders)
	out.EnabledTools = cloneStrings(in.EnabledTools)
	out.Env = cloneStringMap(in.Env)
	out.Headers = cloneStringMap(in.Headers)
	out.ToolApprovals = make(map[string]ApprovalMode, len(in.ToolApprovals))
	for name, approval := range in.ToolApprovals {
		out.ToolApprovals[name] = approval
	}
	return out
}

func cloneStrings(in []string) []string { return append([]string(nil), in...) }

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
