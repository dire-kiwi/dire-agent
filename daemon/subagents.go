package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dire-kiwi/dire-agent/agentteam"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/threadstore"
	"github.com/dire-kiwi/dire-agent/tools"
)

var validAgentName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// SpawnAgent creates a persistent child conversation and immediately starts
// its assigned task. Every child gets its own SQLite file and provider session.
func (m *Manager) SpawnAgent(ctx context.Context, request agentteam.SpawnRequest) (agentteam.Agent, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.Mode = strings.TrimSpace(request.Mode)
	request.Task = strings.TrimSpace(request.Task)
	request.Profile = strings.TrimSpace(request.Profile)
	request.Role = strings.TrimSpace(request.Role)
	request.Model = strings.TrimSpace(request.Model)
	request.Thinking = strings.TrimSpace(request.Thinking)
	if request.ParentID == "" {
		return agentteam.Agent{}, errors.New("daemon: parent agent id is required")
	}
	if !validAgentName.MatchString(request.Name) {
		return agentteam.Agent{}, errors.New("daemon: agent name must be 1-64 letters, digits, dots, dashes, or underscores")
	}
	if request.Task == "" || len(request.Task) > 100_000 {
		return agentteam.Agent{}, errors.New("daemon: agent task must be between 1 and 100000 bytes")
	}
	if len(request.Role) > 256 {
		return agentteam.Agent{}, errors.New("daemon: agent role must not exceed 256 bytes")
	}
	if request.Thinking != "" && !validThinkingLevel(request.Thinking) {
		return agentteam.Agent{}, errors.New("daemon: subagent thinking level is invalid")
	}
	if request.Mode == "" {
		request.Mode = agentteam.SpawnModeDirect
	}
	if request.Mode != agentteam.SpawnModeDirect && request.Mode != agentteam.SpawnModeModelRouter {
		return agentteam.Agent{}, fmt.Errorf("daemon: invalid subagent spawn mode %q", request.Mode)
	}

	parentRuntime, err := m.getRuntime(ctx, request.ParentID)
	if err != nil {
		return agentteam.Agent{}, err
	}
	if err := m.refreshCapabilities(ctx, parentRuntime); err != nil {
		return agentteam.Agent{}, err
	}
	parent := parentRuntime.snapshotThread()
	rootID := teamRootID(parent)
	settings, err := m.runtimeSettings(ctx, rootID)
	if err != nil {
		return agentteam.Agent{}, err
	}
	if err := validateSpawnPolicy(parent, settings); err != nil {
		return agentteam.Agent{}, err
	}
	var routedPolicy *agentteam.SpawnRequest
	if parent.AgentProfile == configuration.ModelRouterControllerProfile {
		if request.Mode != agentteam.SpawnModeDirect {
			return agentteam.Agent{}, errors.New("daemon: a model-router controller can only spawn a direct worker")
		}
		if err := validateModelRouterChoice(request.Model, settings.Subagents.ModelRouting.AllowedModels); err != nil {
			return agentteam.Agent{}, err
		}
		if request.Thinking == "" {
			return agentteam.Agent{}, errors.New("daemon: model-router controller must choose a worker thinking level")
		}
		policy := modelRouterSpawnPolicy(parent.ModelRouterPolicy)
		if policy == nil {
			return agentteam.Agent{}, errors.New("daemon: model-router controller has no persisted worker policy")
		}
		request.Mode, request.Profile, request.Role = agentteam.SpawnModeDirect, policy.Profile, policy.Role
		request.Tools = cloneOptionalStrings(policy.Tools)
	} else if request.Mode == agentteam.SpawnModeModelRouter {
		if request.Model != "" {
			return agentteam.Agent{}, errors.New("daemon: model-router mode selects the worker model; request model must be empty")
		}
		if err := validateModelRouterRequest(parent, settings.Subagents); err != nil {
			return agentteam.Agent{}, err
		}
	}
	if request.Profile == "" {
		request.Profile = "general"
	}
	profile, ok := settings.Subagents.Profiles[request.Profile]
	if !ok {
		return agentteam.Agent{}, fmt.Errorf("daemon: unknown subagent profile %q", request.Profile)
	}
	var routedProfile *configuration.AgentProfile
	if request.Mode == agentteam.SpawnModeModelRouter {
		original := request
		routedPolicy = &original
		workerProfile := profile
		routedProfile = &workerProfile
		request = modelRouterRequest(parent, original, settings.Subagents.ModelRouting)
		profile = modelRouterProfile(settings.Subagents.ModelRouting)
	}

	parentTools := make(map[string]bool)
	if parent.AgentProfile == configuration.ModelRouterControllerProfile {
		granted, err := m.effectiveModelRouterGrant(ctx, parent)
		if err != nil {
			return agentteam.Agent{}, err
		}
		for _, name := range granted {
			parentTools[name] = true
		}
	} else {
		parentRuntime.mu.Lock()
		for name := range parentRuntime.tools {
			if !isTeamTool(name) {
				parentTools[name] = true
			}
		}
		parentRuntime.mu.Unlock()
	}
	grantProfile, grantTools := profile, request.Tools
	if routedPolicy != nil && routedProfile != nil {
		grantProfile, grantTools = *routedProfile, routedPolicy.Tools
	}
	allowedTools, err := narrowAgentTools(parent, parentTools, grantProfile, grantTools)
	if err != nil {
		return agentteam.Agent{}, err
	}

	m.teamMu.Lock()
	defer m.teamMu.Unlock()
	team, err := m.listTeamThreads(ctx, rootID)
	if err != nil {
		return agentteam.Agent{}, err
	}
	if err := enforceTeamLimits(team, parent.ID, settings.Subagents); err != nil {
		return agentteam.Agent{}, err
	}
	id, err := newAgentID()
	if err != nil {
		return agentteam.Agent{}, err
	}
	child := m.newChildThread(id, parent, request, profile, allowedTools)
	if routedPolicy != nil {
		child.ModelRouterPolicy = routedAgentPolicyFromRequest(*routedPolicy)
	}
	created, err := m.createResource(ctx, child, "agent_created")
	if err != nil {
		return agentteam.Agent{}, err
	}
	m.emit(ctx, parentRuntime, "agent_spawned", agentFromThread(created))
	if err := m.Prompt(ctx, created.ID, request.Task, ""); err != nil {
		return agentteam.Agent{}, fmt.Errorf("daemon: start child agent: %w", err)
	}
	m.notifyTeamLocked(rootID)
	running, err := m.Thread(ctx, created.ID)
	if err != nil {
		return agentteam.Agent{}, err
	}
	return agentFromThread(running), nil
}

func (m *Manager) Agent(ctx context.Context, id string) (agentteam.Agent, error) {
	thread, err := m.Thread(ctx, id)
	if err != nil {
		return agentteam.Agent{}, err
	}
	if !thread.IsSubagent() {
		return agentteam.Agent{}, errors.New("daemon: conversation is not a subagent")
	}
	return agentFromThread(thread), nil
}

func (m *Manager) DeleteAgent(ctx context.Context, callerID, targetID string) error {
	_, target, _, err := m.authorizeTeamRoute(ctx, callerID, targetID)
	if err != nil {
		return err
	}
	if !target.IsSubagent() {
		return errors.New("daemon: the team root cannot be deleted as an agent")
	}
	return m.DeleteThread(ctx, target.ID)
}

func validateSpawnPolicy(parent threadstore.Thread, settings configuration.Settings) error {
	policy := settings.Subagents
	if !policy.Enabled {
		return errors.New("daemon: subagents are disabled")
	}
	if parent.Depth >= policy.MaxDepth {
		return fmt.Errorf("daemon: subagent depth limit %d reached", policy.MaxDepth)
	}
	if parent.IsSubagent() {
		if parent.AgentProfile == configuration.ModelRouterControllerProfile {
			return nil
		}
		profile, ok := policy.Profiles[parent.AgentProfile]
		if !ok || !profile.CanSpawn {
			return errors.New("daemon: this subagent profile cannot spawn children")
		}
	}
	return nil
}

func validateModelRouterRequest(parent threadstore.Thread, settings configuration.SubagentSettings) error {
	routing := settings.ModelRouting
	if strings.TrimSpace(routing.ControllerModel) == "" || strings.TrimSpace(routing.Prompt) == "" || len(routing.AllowedModels) == 0 {
		return errors.New("daemon: model-router mode is not configured")
	}
	if parent.Depth+2 > settings.MaxDepth {
		return fmt.Errorf("daemon: model-router mode needs two levels below the parent; subagent depth limit is %d", settings.MaxDepth)
	}
	if settings.MaxConcurrent < 2 {
		return errors.New("daemon: model-router mode requires at least two concurrent subagents")
	}
	return nil
}

func validateModelRouterChoice(model string, allowed []string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return errors.New("daemon: model-router controller must choose a worker model")
	}
	for _, candidate := range allowed {
		if model == strings.TrimSpace(candidate) {
			return nil
		}
	}
	return fmt.Errorf("daemon: model-router worker model %q is not allowed", model)
}

func enforceTeamLimits(team []threadstore.Thread, parentID string, policy configuration.SubagentSettings) error {
	children, running := 0, 0
	for _, member := range team {
		if member.ParentID == parentID {
			children++
		}
		if member.IsSubagent() && member.Status == "running" {
			running++
		}
	}
	if children >= policy.MaxChildren {
		return fmt.Errorf("daemon: parent already has the maximum %d children", policy.MaxChildren)
	}
	if running >= policy.MaxConcurrent {
		return fmt.Errorf("daemon: team already has the maximum %d concurrent subagents", policy.MaxConcurrent)
	}
	return nil
}

func narrowAgentTools(parent threadstore.Thread, parentTools map[string]bool, profile configuration.AgentProfile, requested []string) ([]string, error) {
	if parent.ResourceKind() == threadstore.KindChat {
		if len(requested) != 0 {
			return nil, errors.New("daemon: standalone chat agents cannot use project tools")
		}
		return []string{}, nil
	}
	base := make([]string, 0, len(parentTools))
	if profile.Tools == nil {
		for name := range parentTools {
			base = append(base, name)
		}
		sort.Strings(base)
	} else {
		for _, name := range profile.Tools {
			if parentTools[name] {
				base = append(base, name)
			}
		}
	}
	if requested == nil {
		return base, nil
	}
	granted := make(map[string]bool, len(base))
	for _, name := range base {
		granted[name] = true
	}
	seen := make(map[string]bool, len(requested))
	result := make([]string, 0, len(requested))
	for _, name := range requested {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return nil, fmt.Errorf("daemon: invalid or duplicate requested tool %q", name)
		}
		if !granted[name] {
			return nil, fmt.Errorf("daemon: requested tool %q is not granted by the parent and profile", name)
		}
		seen[name] = true
		result = append(result, name)
	}
	return result, nil
}

func (m *Manager) newChildThread(id string, parent threadstore.Thread, request agentteam.SpawnRequest, profile configuration.AgentProfile, allowed []string) threadstore.Thread {
	builtin := make(map[string]bool)
	for _, name := range tools.Names() {
		builtin[name] = true
	}
	localTools := make([]string, 0, len(allowed))
	if request.Profile != configuration.ModelRouterControllerProfile {
		for _, name := range allowed {
			if builtin[name] {
				localTools = append(localTools, name)
			}
		}
	}
	model := firstNonEmpty(request.Model, profile.Model, parent.Model)
	thinking := firstNonEmpty(request.Thinking, string(profile.Thinking), parent.ThinkingLevel)
	child := threadstore.Thread{
		ID: id, Kind: parent.ResourceKind(), SettingsID: parent.SettingsID,
		ParentID: parent.ID, RootID: teamRootID(parent),
		AgentName: request.Name, AgentRole: request.Role, AgentProfile: request.Profile,
		AgentTools: append([]string(nil), allowed...), Depth: parent.Depth + 1,
		Name: request.Name, Model: model, CWD: parent.CWD,
		AdditionalFolders: append([]string(nil), parent.AdditionalFolders...),
		Instructions:      combineInstructions(profile.Instructions, childAgentInstructions(parent, request)),
		ThinkingLevel:     thinking, SteeringMode: parent.SteeringMode, FollowUpMode: parent.FollowUpMode,
		Tools: localTools, Status: "idle",
	}
	if modelInfo, ok := m.modelInfo(model); ok {
		child.Usage.ContextWindow = modelInfo.ContextWindow
	}
	return child
}

func modelRouterRequest(parent threadstore.Thread, original agentteam.SpawnRequest, routing configuration.SubagentModelRoutingSettings) agentteam.SpawnRequest {
	return agentteam.SpawnRequest{
		ParentID: parent.ID,
		Name:     modelRouterName(original.Name),
		Mode:     agentteam.SpawnModeDirect,
		Profile:  configuration.ModelRouterControllerProfile,
		Role:     "model routing controller",
		Task:     modelRouterTask(original),
		Model:    strings.TrimSpace(routing.ControllerModel),
		Thinking: string(routing.ControllerThinking),
		Tools:    nil,
	}
}

func modelRouterName(workerName string) string {
	const suffix = "-router"
	if len(workerName)+len(suffix) <= 64 {
		return workerName + suffix
	}
	return workerName[:64-len(suffix)] + suffix
}

func modelRouterProfile(routing configuration.SubagentModelRoutingSettings) configuration.AgentProfile {
	return configuration.AgentProfile{
		Description:  "Decomposes work and delegates bounded subtasks to allowed worker models.",
		Instructions: modelRouterInstructions(routing),
		Tools:        nil,
		CanSpawn:     true,
	}
}

func routedAgentPolicyFromRequest(request agentteam.SpawnRequest) *threadstore.RoutedAgentPolicy {
	return &threadstore.RoutedAgentPolicy{
		Profile: request.Profile, Role: request.Role, Tools: cloneOptionalStrings(request.Tools),
	}
}

func cloneOptionalStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string{}, values...)
}

func modelRouterInstructions(routing configuration.SubagentModelRoutingSettings) string {
	return fmt.Sprintf("You are a model-routing controller. Do not perform the requested work yourself. Decompose the request into useful independent subtasks and call spawn_agent once for each, choosing a distinct name, focused task, allowed model, and appropriate thinking level. The daemon fixes the worker profile, role, and tool restrictions from the parent request. Stay within child and concurrency limits, and summarize worker results to your parent. Allowed worker models: %s.\n\nConfigured routing policy:\n%s",
		strings.Join(routing.AllowedModels, ", "), routing.Prompt)
}

func modelRouterTask(request agentteam.SpawnRequest) string {
	var details strings.Builder
	details.WriteString("Plan and spawn one or more bounded workers for this request.\n")
	fmt.Fprintf(&details, "Controller request name: %s\nWorker profile: %s\n", request.Name, request.Profile)
	if request.Role != "" {
		fmt.Fprintf(&details, "Worker role: %s\n", request.Role)
	}
	if request.Tools != nil {
		fmt.Fprintf(&details, "Worker tools: %s\n", strings.Join(request.Tools, ", "))
	}
	details.WriteString("Overall requested work follows between the delimiters.\n<requested-work>\n")
	details.WriteString(request.Task)
	details.WriteString("\n</requested-work>")
	return details.String()
}

func childAgentInstructions(parent threadstore.Thread, request agentteam.SpawnRequest) string {
	role := request.Role
	if role == "" {
		role = "independent collaborator"
	}
	return fmt.Sprintf("You are subagent %q (%s), a %s in conversation team %q. Work on the assigned task, stay within inherited permissions, and communicate useful findings or blockers to your parent with send_agent_message. Do not claim work you have not verified.", request.Name, request.Profile, role, teamRootID(parent))
}

func newTeamID(prefix string) (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(value[:]), nil
}

func newAgentID() (string, error) { return newTeamID("agent_") }

func agentFromThread(thread threadstore.Thread) agentteam.Agent {
	return agentteam.Agent{
		ID: thread.ID, ParentID: thread.ParentID, RootID: teamRootID(thread),
		Name: firstNonEmpty(thread.AgentName, thread.Name, thread.ID), Role: thread.AgentRole,
		Profile: thread.AgentProfile, Depth: thread.Depth, Status: thread.Status,
		Model: thread.Model, CreatedAt: thread.CreatedAt, UpdatedAt: thread.UpdatedAt,
	}
}

func teamRootID(thread threadstore.Thread) string {
	if thread.RootID != "" {
		return thread.RootID
	}
	return thread.ID
}

func isTeamTool(name string) bool {
	switch name {
	case "spawn_agent", "list_agents", "send_agent_message", "wait_agents", "interrupt_agent":
		return true
	default:
		return false
	}
}

// Compile-time assertion keeps the orchestration tools and manager in sync.
var _ agentteam.Backend = (*Manager)(nil)
