// Package daemon manages persistent agent conversations and their asynchronous runs.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/imeredith/dire-agent/agent"
	"github.com/imeredith/dire-agent/agentloop"
	"github.com/imeredith/dire-agent/agentteam"
	"github.com/imeredith/dire-agent/capability"
	"github.com/imeredith/dire-agent/configuration"
	"github.com/imeredith/dire-agent/skills"
	"github.com/imeredith/dire-agent/threadstore"
)

type ManagerConfig struct {
	Store           *threadstore.Store
	Provider        agent.StatefulProvider
	DefaultProvider string
	DefaultModel    string
	DefaultCWD      string
	DefaultTools    []string
	DefaultThinking string
	MaxAgentSteps   int
	AvailableModels []ModelInfo
	// OverrideModelDefaults makes the explicitly supplied DefaultProvider and
	// DefaultModel take precedence over model defaults in Settings. It is used
	// by daemon command-line overrides; ordinary configuration remains the
	// source of truth when false.
	OverrideModelDefaults bool
	// SupportedProviders optionally constrains provider names accepted through
	// the daemon configuration API. An empty list preserves the manager's
	// provider-neutral embedding behavior.
	SupportedProviders []string
	Settings           *configuration.Store
	Capabilities       capability.Resolver
}

type Manager struct {
	config   ManagerConfig
	mu       sync.Mutex
	runtimes map[string]*threadRuntime

	subMu       sync.Mutex
	subscribers map[string]map[uint64]chan Event
	nextSubID   atomic.Uint64

	teamMu        sync.Mutex
	teamSignals   map[string]chan struct{}
	teamMailboxes map[string][]agentteam.Message
}

type threadRuntime struct {
	manager                *Manager
	db                     *threadstore.ThreadDB
	session                agent.StepSession
	stateful               agent.StatefulSession
	tools                  map[string]agentloop.Tool
	capabilityInstructions string
	capabilities           []capability.Descriptor
	skills                 []skills.Skill
	skillDiagnostics       []skills.Diagnostic
	preparePrompt          func(context.Context, string) (string, error)
	hooks                  agentloop.Hooks
	commands               map[string]capability.Command

	mu        sync.Mutex
	thread    threadstore.Thread
	running   bool
	finishing bool
	steering  []string
	followUps []string
	cancel    context.CancelFunc
	runWG     sync.WaitGroup
}

func NewManager(config ManagerConfig) (*Manager, error) {
	if config.Store == nil {
		return nil, errors.New("daemon: project store is required")
	}
	if config.Provider == nil {
		return nil, errors.New("daemon: stateful provider is required")
	}
	if config.DefaultModel == "" {
		config.DefaultModel = "gpt-5.6"
	}
	if config.DefaultProvider == "" {
		config.DefaultProvider = "codex"
	}
	if config.DefaultThinking == "" {
		config.DefaultThinking = "medium"
	}
	if config.DefaultCWD == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		config.DefaultCWD = cwd
	}
	if len(config.DefaultTools) == 0 {
		config.DefaultTools = []string{"read", "grep", "find", "ls"}
	}
	if config.MaxAgentSteps <= 0 {
		config.MaxAgentSteps = 32
	}
	if len(config.AvailableModels) == 0 {
		if config.DefaultProvider == "codex" {
			config.AvailableModels = defaultModels()
		} else {
			config.AvailableModels = []ModelInfo{{Provider: config.DefaultProvider, ID: config.DefaultModel}}
		}
	}
	config.AvailableModels = normalizeModels(config.AvailableModels, config.DefaultProvider, config.DefaultModel)
	return &Manager{
		config:        config,
		runtimes:      make(map[string]*threadRuntime),
		subscribers:   make(map[string]map[uint64]chan Event),
		teamSignals:   make(map[string]chan struct{}),
		teamMailboxes: make(map[string][]agentteam.Message),
	}, nil
}

func (m *Manager) requireActiveProvider(configured string) error {
	configured = strings.TrimSpace(configured)
	active := strings.TrimSpace(m.config.DefaultProvider)
	if configured == "" || strings.EqualFold(configured, active) {
		return nil
	}
	return fmt.Errorf("daemon: configured model provider %q is not active (currently %q); restart the daemon to apply the provider change", configured, active)
}

func (m *Manager) validateActiveModel(model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return errors.New("daemon: model must not be empty")
	}
	if strings.EqualFold(strings.TrimSpace(m.config.DefaultProvider), "openrouter") && !qualifiedProviderModel(model) {
		return fmt.Errorf("daemon: OpenRouter model %q must be an organization-qualified slug such as openrouter/auto", model)
	}
	return nil
}

func qualifiedProviderModel(model string) bool {
	parts := strings.Split(strings.TrimSpace(model), "/")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return false
		}
	}
	return true
}

func (m *Manager) validateConfigProviders(config configuration.Config) error {
	if len(m.config.SupportedProviders) == 0 {
		return nil
	}
	allowed := make(map[string]bool, len(m.config.SupportedProviders))
	for _, provider := range m.config.SupportedProviders {
		if provider = strings.ToLower(strings.TrimSpace(provider)); provider != "" {
			allowed[provider] = true
		}
	}
	validate := func(settings configuration.Settings, scope string) error {
		provider := strings.ToLower(strings.TrimSpace(settings.Model.Provider))
		if !allowed[provider] {
			return fmt.Errorf("daemon: %s model provider %q is unsupported (supported: %s)", scope, settings.Model.Provider, strings.Join(m.config.SupportedProviders, ", "))
		}
		if provider == "openrouter" && !qualifiedProviderModel(settings.Model.ID) {
			return fmt.Errorf("daemon: %s OpenRouter model %q must be an organization-qualified slug such as openrouter/auto", scope, settings.Model.ID)
		}
		return nil
	}
	if err := validate(config.Global, "global"); err != nil {
		return err
	}
	for id := range config.Projects {
		settings, _ := config.Effective(id)
		if err := validate(settings, "project "+id); err != nil {
			return err
		}
	}
	return nil
}
