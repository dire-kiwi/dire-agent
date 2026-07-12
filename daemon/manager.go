// Package daemon manages persistent agent conversations and their asynchronous runs.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/agentloop"
	"github.com/dire-kiwi/dire-agent/capability"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/skills"
	"github.com/dire-kiwi/dire-agent/threadstore"
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
	Settings        *configuration.Store
	Capabilities    capability.Resolver
}

type Manager struct {
	config   ManagerConfig
	mu       sync.Mutex
	runtimes map[string]*threadRuntime

	subMu       sync.Mutex
	subscribers map[string]map[uint64]chan Event
	nextSubID   atomic.Uint64

	teamMu      sync.Mutex
	teamSignals map[string]chan struct{}
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
	if err := recoverPersistedRuns(context.Background(), config.Store); err != nil {
		return nil, err
	}
	return &Manager{
		config:      config,
		runtimes:    make(map[string]*threadRuntime),
		subscribers: make(map[string]map[uint64]chan Event),
		teamSignals: make(map[string]chan struct{}),
	}, nil
}

// recoverPersistedRuns runs before any runtime or provider session is opened.
// A newly constructed manager has no live goroutines, so every stored running
// status belongs to a process that is no longer executing it.
func recoverPersistedRuns(ctx context.Context, store *threadstore.Store) error {
	threads, err := store.List(ctx)
	if err != nil {
		return fmt.Errorf("daemon: list conversations for run recovery: %w", err)
	}
	for _, thread := range threads {
		if thread.Status != "running" {
			continue
		}
		db, err := store.Open(ctx, thread.ID)
		if err != nil {
			return fmt.Errorf("daemon: open %s for run recovery: %w", thread.ID, err)
		}
		_, updateErr := db.UpdateThread(ctx, func(stored *threadstore.Thread) error {
			stored.Status = recoveredRunStatus(*stored)
			return nil
		})
		closeErr := db.Close()
		if updateErr != nil {
			return fmt.Errorf("daemon: recover interrupted conversation %s: %w", thread.ID, updateErr)
		}
		if closeErr != nil {
			return fmt.Errorf("daemon: close recovered conversation %s: %w", thread.ID, closeErr)
		}
	}
	return nil
}

func recoveredRunStatus(thread threadstore.Thread) string {
	if thread.IsSubagent() {
		return "interrupted"
	}
	return "idle"
}
