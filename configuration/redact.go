package configuration

import "strings"

func publicConfig(raw Config) Config {
	out, err := cloneConfig(raw)
	if err != nil {
		return Config{}
	}
	redactSettings(&out.Global)
	for id, project := range out.Projects {
		if project.Settings.MCP != nil {
			for name, patch := range project.Settings.MCP.Servers {
				redactServerPatch(&patch)
				project.Settings.MCP.Servers[name] = patch
			}
		}
		if project.Settings.Extensions != nil {
			for name, patch := range project.Settings.Extensions.Sources {
				redactExtensionPatch(&patch)
				project.Settings.Extensions.Sources[name] = patch
			}
		}
		out.Projects[id] = project
	}
	return out
}

func redactServerPatch(patch *MCPServerPatch) {
	if patch.Env != nil {
		secret := []string(nil)
		if patch.SecretEnv != nil {
			secret = *patch.SecretEnv
		}
		redactValues(*patch.Env, secret)
	}
	if patch.Headers != nil {
		secret := []string(nil)
		if patch.SecretHeaders != nil {
			secret = *patch.SecretHeaders
		}
		redactValues(*patch.Headers, secret)
	}
}

func redactSettings(settings *Settings) {
	for name, server := range settings.MCP.Servers {
		redactServer(&server)
		settings.MCP.Servers[name] = server
	}
	for name, source := range settings.Extensions.Sources {
		redactExtensionSource(&source)
		settings.Extensions.Sources[name] = source
	}
}

func redactExtensionSource(source *ExtensionSource) {
	redactValues(source.Env, source.SecretEnv)
}

func redactExtensionPatch(patch *ExtensionSourcePatch) {
	if patch.Env == nil {
		return
	}
	var secret []string
	if patch.SecretEnv != nil {
		secret = *patch.SecretEnv
	}
	redactValues(*patch.Env, secret)
}

func redactServer(server *MCPServer) {
	redactValues(server.Env, server.SecretEnv)
	redactValues(server.Headers, server.SecretHeaders)
}

func redactValues(values map[string]string, explicit []string) {
	secrets := stringSet(explicit)
	for key := range values {
		if _, marked := secrets[key]; marked || looksSecret(key) {
			values[key] = RedactedValue
		}
	}
}

func restoreRedacted(candidate *Config, old Config) {
	restoreSettings(&candidate.Global, old.Global)
	for id, project := range candidate.Projects {
		previous, ok := old.Projects[id]
		if !ok {
			continue
		}
		if project.Settings.MCP != nil && previous.Settings.MCP != nil {
			for name, serverPatch := range project.Settings.MCP.Servers {
				oldPatch, ok := previous.Settings.MCP.Servers[name]
				if !ok {
					continue
				}
				restoreServerPatch(&serverPatch, oldPatch)
				project.Settings.MCP.Servers[name] = serverPatch
			}
		}
		if project.Settings.Extensions != nil && previous.Settings.Extensions != nil {
			for name, patch := range project.Settings.Extensions.Sources {
				oldPatch, ok := previous.Settings.Extensions.Sources[name]
				if !ok {
					continue
				}
				restoreExtensionPatch(&patch, oldPatch)
				project.Settings.Extensions.Sources[name] = patch
			}
		}
		candidate.Projects[id] = project
	}
}

func restoreExtensionPatch(candidate *ExtensionSourcePatch, old ExtensionSourcePatch) {
	if candidate.Env == nil || old.Env == nil {
		return
	}
	restoreValues(*candidate.Env, *old.Env)
	if old.SecretEnv == nil {
		return
	}
	for key, value := range *candidate.Env {
		if value != (*old.Env)[key] || !contains(*old.SecretEnv, key) {
			continue
		}
		if candidate.SecretEnv == nil {
			empty := []string{}
			candidate.SecretEnv = &empty
		}
		*candidate.SecretEnv = addUnique(*candidate.SecretEnv, key)
	}
}

func restoreServerPatch(candidate *MCPServerPatch, old MCPServerPatch) {
	if candidate.Env != nil && old.Env != nil {
		restoreValues(*candidate.Env, *old.Env)
		if old.SecretEnv != nil {
			for key, value := range *candidate.Env {
				if value == (*old.Env)[key] && contains(*old.SecretEnv, key) {
					if candidate.SecretEnv == nil {
						empty := []string{}
						candidate.SecretEnv = &empty
					}
					*candidate.SecretEnv = addUnique(*candidate.SecretEnv, key)
				}
			}
		}
	}
	if candidate.Headers != nil && old.Headers != nil {
		restoreValues(*candidate.Headers, *old.Headers)
		if old.SecretHeaders != nil {
			for key, value := range *candidate.Headers {
				if value == (*old.Headers)[key] && contains(*old.SecretHeaders, key) {
					if candidate.SecretHeaders == nil {
						empty := []string{}
						candidate.SecretHeaders = &empty
					}
					*candidate.SecretHeaders = addUnique(*candidate.SecretHeaders, key)
				}
			}
		}
	}
}

func restoreValues(candidate, old map[string]string) {
	for key, value := range candidate {
		if value != RedactedValue {
			continue
		}
		if oldValue, ok := old[key]; ok {
			candidate[key] = oldValue
		}
	}
}

func restoreSettings(candidate *Settings, old Settings) {
	for name, server := range candidate.MCP.Servers {
		oldServer, ok := old.MCP.Servers[name]
		if !ok {
			continue
		}
		restoreServer(&server, oldServer)
		candidate.MCP.Servers[name] = server
	}
	for name, source := range candidate.Extensions.Sources {
		oldSource, ok := old.Extensions.Sources[name]
		if !ok {
			continue
		}
		restoreExtensionSource(&source, oldSource)
		candidate.Extensions.Sources[name] = source
	}
}

func restoreExtensionSource(candidate *ExtensionSource, old ExtensionSource) {
	for key, value := range candidate.Env {
		if value != RedactedValue {
			continue
		}
		if oldValue, ok := old.Env[key]; ok {
			candidate.Env[key] = oldValue
			if contains(old.SecretEnv, key) {
				candidate.SecretEnv = addUnique(candidate.SecretEnv, key)
			}
		}
	}
}

func restoreServer(candidate *MCPServer, old MCPServer) {
	for key, value := range candidate.Env {
		if value != RedactedValue {
			continue
		}
		if oldValue, ok := old.Env[key]; ok {
			candidate.Env[key] = oldValue
			if contains(old.SecretEnv, key) {
				candidate.SecretEnv = addUnique(candidate.SecretEnv, key)
			}
		}
	}
	for key, value := range candidate.Headers {
		if value != RedactedValue {
			continue
		}
		if oldValue, ok := old.Headers[key]; ok {
			candidate.Headers[key] = oldValue
			if contains(old.SecretHeaders, key) {
				candidate.SecretHeaders = addUnique(candidate.SecretHeaders, key)
			}
		}
	}
}

func looksSecret(key string) bool {
	normalized := strings.NewReplacer("-", "_", " ", "_").Replace(strings.ToLower(key))
	for _, marker := range []string{"authorization", "credential", "password", "passwd", "secret", "token", "api_key", "cookie"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func addUnique(values []string, value string) []string {
	if contains(values, value) {
		return values
	}
	return append(values, value)
}
