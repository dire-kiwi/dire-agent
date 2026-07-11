package tools

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

// ProcessSandbox describes an argv-safe macOS sandbox-exec wrapper. Workspace,
// additional write paths, and temporary paths are writable; ExtraReadPaths
// remain read-only.
type ProcessSandbox struct {
	Workspace            string
	Command              string
	Args                 []string
	ExtraReadPaths       []string
	AdditionalWritePaths []string
	AllowNetwork         bool
}

// WrapSandboxedProcess returns a sandbox-exec command and argv without invoking
// a shell. It fails closed when sandbox-exec or the workspace is unavailable.
func WrapSandboxedProcess(options ProcessSandbox) (string, []string, error) {
	sandbox, err := validateExecutable(defaultSandboxExecutable)
	if err != nil {
		return "", nil, fmt.Errorf("tools: process sandbox unavailable: %w", err)
	}
	command := options.Command
	if !filepath.IsAbs(command) {
		command, err = exec.LookPath(command)
		if err != nil {
			return "", nil, fmt.Errorf("tools: resolve process command: %w", err)
		}
	}
	command, err = filepath.Abs(command)
	if err != nil {
		return "", nil, fmt.Errorf("tools: resolve process command: %w", err)
	}
	reads := append([]string(nil), options.ExtraReadPaths...)
	reads = append(reads, command)
	profile, err := sandboxProfileWithWritePaths(options.Workspace, reads, options.AdditionalWritePaths, options.AllowNetwork)
	if err != nil {
		return "", nil, fmt.Errorf("tools: build process sandbox profile: %w", err)
	}
	args := []string{"-p", profile, command}
	args = append(args, options.Args...)
	return sandbox, args, nil
}
