package configuration

// ProjectOverride scopes a settings patch to one canonical project folder.
type ProjectOverride struct {
	Folder   string        `json:"folder"`
	Settings SettingsPatch `json:"settings"`
}

// SettingsPatch uses pointers so false, zero, and empty-list overrides remain
// distinct from inherited values.
type SettingsPatch struct {
	Model          *ModelPatch          `json:"model,omitempty"`
	Thinking       *ThinkingPatch       `json:"thinking,omitempty"`
	Tools          *ToolPatch           `json:"tools,omitempty"`
	Queues         *QueuePatch          `json:"queues,omitempty"`
	Skills         *SkillPatch          `json:"skills,omitempty"`
	MCP            *MCPPatch            `json:"mcp,omitempty"`
	Extensions     *ExtensionPatch      `json:"extensions,omitempty"`
	Subagents      *SubagentPatch       `json:"subagents,omitempty"`
	Launchers      *[]ProjectLauncher   `json:"launchers,omitempty"`
	Desktop        *DesktopPatch        `json:"desktop,omitempty"`
	StandaloneChat *StandaloneChatPatch `json:"standalone_chat,omitempty"`
}

type ModelPatch struct {
	Provider      *string `json:"provider,omitempty"`
	ID            *string `json:"id,omitempty"`
	ContextWindow *int64  `json:"context_window,omitempty"`
}

type ThinkingPatch struct {
	Level *ThinkingLevel `json:"level,omitempty"`
}

type ToolPatch struct {
	Enabled  *[]string     `json:"enabled,omitempty"`
	Sandbox  *SandboxMode  `json:"sandbox,omitempty"`
	Approval *ApprovalMode `json:"approval,omitempty"`
}

type QueuePatch struct {
	SteeringMode *QueueMode `json:"steering_mode,omitempty"`
	FollowUpMode *QueueMode `json:"follow_up_mode,omitempty"`
	MaxPending   *int       `json:"max_pending,omitempty"`
}

type SkillPatch struct {
	Roots    *[]string  `json:"roots,omitempty"`
	Disabled *[]string  `json:"disabled,omitempty"`
	Trust    *TrustMode `json:"trust,omitempty"`
}

// Server entries deeply patch servers with the same name and otherwise add a
// new definition. RemoveServers explicitly removes inherited entries.
type MCPPatch struct {
	Servers       map[string]MCPServerPatch `json:"servers,omitempty"`
	RemoveServers []string                  `json:"remove_servers,omitempty"`
}

type MCPServerPatch struct {
	Transport     *MCPTransport            `json:"transport,omitempty"`
	Command       *string                  `json:"command,omitempty"`
	Args          *[]string                `json:"args,omitempty"`
	InheritEnv    *bool                    `json:"inherit_env,omitempty"`
	URL           *string                  `json:"url,omitempty"`
	Env           *map[string]string       `json:"env,omitempty"`
	SecretEnv     *[]string                `json:"secret_env,omitempty"`
	Headers       *map[string]string       `json:"headers,omitempty"`
	SecretHeaders *[]string                `json:"secret_headers,omitempty"`
	EnabledTools  *[]string                `json:"enabled_tools,omitempty"`
	Approval      *ApprovalMode            `json:"approval,omitempty"`
	ToolApprovals *map[string]ApprovalMode `json:"tool_approvals,omitempty"`
	Enabled       *bool                    `json:"enabled,omitempty"`
}

// Source entries deeply patch sources with the same name and otherwise add a
// new definition. RemoveSources explicitly removes inherited entries.
type ExtensionPatch struct {
	Sources       map[string]ExtensionSourcePatch `json:"sources,omitempty"`
	RemoveSources []string                        `json:"remove_sources,omitempty"`
	AllowUnsigned *bool                           `json:"allow_unsigned,omitempty"`
}

type ExtensionSourcePatch struct {
	Kind       *ExtensionKind     `json:"kind,omitempty"`
	Location   *string            `json:"location,omitempty"`
	Ref        *string            `json:"ref,omitempty"`
	Trust      *TrustMode         `json:"trust,omitempty"`
	Enabled    *bool              `json:"enabled,omitempty"`
	Command    *string            `json:"command,omitempty"`
	Args       *[]string          `json:"args,omitempty"`
	Env        *map[string]string `json:"env,omitempty"`
	SecretEnv  *[]string          `json:"secret_env,omitempty"`
	InheritEnv *bool              `json:"inherit_env,omitempty"`
}

type PluginPatch = ExtensionPatch
type PluginSourcePatch = ExtensionSourcePatch

type SubagentPatch struct {
	Enabled              *bool                    `json:"enabled,omitempty"`
	MaxDepth             *int                     `json:"max_depth,omitempty"`
	MaxChildren          *int                     `json:"max_children,omitempty"`
	MaxConcurrent        *int                     `json:"max_concurrent,omitempty"`
	AllowSiblingMessages *bool                    `json:"allow_sibling_messages,omitempty"`
	AutoReport           *bool                    `json:"auto_report,omitempty"`
	Profiles             *map[string]AgentProfile `json:"profiles,omitempty"`
}

type DesktopPatch struct {
	CodexHome       *string   `json:"codex_home,omitempty"`
	DesktopConfig   *string   `json:"desktop_config,omitempty"`
	SyncMode        *SyncMode `json:"sync_mode,omitempty"`
	SyncSkills      *bool     `json:"sync_skills,omitempty"`
	SyncMCP         *bool     `json:"sync_mcp,omitempty"`
	SyncExtensions  *bool     `json:"sync_extensions,omitempty"`
	WatchForChanges *bool     `json:"watch_for_changes,omitempty"`
}

type StandaloneChatPatch struct {
	Model          *string        `json:"model,omitempty"`
	Thinking       *ThinkingLevel `json:"thinking,omitempty"`
	Tools          *[]string      `json:"tools,omitempty"`
	Instructions   *string        `json:"instructions,omitempty"`
	PersistHistory *bool          `json:"persist_history,omitempty"`
}
