// Package skills discovers and loads Agent Skills-compatible SKILL.md files.
package skills

import (
	"errors"
	"fmt"
	"strings"
)

const (
	// MaxCatalogChars is the maximum progressive-disclosure catalog size.
	MaxCatalogChars = 8000
	// DefaultMaxSkillBytes bounds a skill loaded into the model context.
	DefaultMaxSkillBytes int64 = 1 << 20
)

// Scope identifies where a skill was discovered.
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
	ScopePlugin  Scope = "plugin"
)

// Severity describes a discovery or metadata diagnostic.
type Severity string

const (
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Diagnostic explains why a source was skipped or accepted with caveats.
type Diagnostic struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Path     string   `json:"path,omitempty"`
	Line     int      `json:"line,omitempty"`
	Message  string   `json:"message"`
}

func (d Diagnostic) Error() string {
	location := d.Path
	if d.Line > 0 {
		location = fmt.Sprintf("%s:%d", location, d.Line)
	}
	if location == "" {
		return d.Message
	}
	return location + ": " + d.Message
}

// Metadata is the progressively disclosed portion of a SKILL.md file.
type Metadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Skill is a discovered skill. Path is the canonical SKILL.md path.
type Skill struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Path           string `json:"path"`
	Directory      string `json:"directory"`
	Root           string `json:"root"`
	Scope          Scope  `json:"scope"`
	Plugin         string `json:"plugin,omitempty"`
	Enabled        bool   `json:"enabled"`
	DisabledReason string `json:"disabled_reason,omitempty"`
}

// PluginRoot registers a directory of skills supplied by an extension/plugin.
type PluginRoot struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// PathRule enables or disables a skill file, directory, or directory subtree.
// The most specific matching rule wins; equal-specificity rules use the last
// configured value.
type PathRule struct {
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
}

// Config controls skill discovery.
type Config struct {
	GlobalRoots           []string     `json:"global_roots,omitempty"`
	PluginRoots           []PluginRoot `json:"plugin_roots,omitempty"`
	ProjectDir            string       `json:"project_dir,omitempty"`
	PathRules             []PathRule   `json:"path_rules,omitempty"`
	EnabledPaths          []string     `json:"enabled_paths,omitempty"`
	DisabledPaths         []string     `json:"disabled_paths,omitempty"`
	MaxSkillBytes         int64        `json:"max_skill_bytes,omitempty"`
	AllowExternalSymlinks bool         `json:"allow_external_symlinks,omitempty"`
}

// Catalog is a stable, name-deduplicated snapshot. Skills contains enabled and
// disabled entries so a configuration UI can display and toggle both.
type Catalog struct {
	Skills      []Skill      `json:"skills"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	maxBytes    int64
}

// EnabledSkills returns a copy of the skills available to the model.
func (c *Catalog) EnabledSkills() []Skill {
	if c == nil {
		return nil
	}
	result := make([]Skill, 0, len(c.Skills))
	for _, skill := range c.Skills {
		if skill.Enabled {
			result = append(result, skill)
		}
	}
	return result
}

// Find returns an enabled skill using a case-insensitive name match.
func (c *Catalog) Find(name string) (Skill, bool) {
	skill, ok := c.FindAny(name)
	return skill, ok && skill.Enabled
}

// FindAny returns a skill regardless of its enabled state.
func (c *Catalog) FindAny(name string) (Skill, bool) {
	if c == nil {
		return Skill{}, false
	}
	name = strings.TrimSpace(name)
	for _, skill := range c.Skills {
		if strings.EqualFold(skill.Name, name) {
			return skill, true
		}
	}
	return Skill{}, false
}

var (
	ErrSkillNotFound = errors.New("skill not found")
	ErrSkillDisabled = errors.New("skill is disabled")
)
