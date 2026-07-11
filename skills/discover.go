package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type rootSpec struct {
	path          string
	logical       string
	boundary      string
	scope         Scope
	plugin        string
	allowExternal bool
}

type candidate struct {
	path      string
	logical   string
	root      rootSpec
	metadata  Metadata
	directory string
}

// Discover scans configured and project-ancestor roots. Filesystem and
// metadata problems are reported in Catalog.Diagnostics without preventing
// other valid skills from loading.
func Discover(config Config) (*Catalog, error) {
	if config.MaxSkillBytes <= 0 {
		config.MaxSkillBytes = DefaultMaxSkillBytes
	}
	roots, err := discoveryRoots(config)
	if err != nil {
		return nil, err
	}
	rules := compileRules(config)
	catalog := &Catalog{maxBytes: config.MaxSkillBytes}
	var candidates []candidate

	for _, root := range roots {
		files, issues := walkSkillFiles(root)
		catalog.Diagnostics = append(catalog.Diagnostics, issues...)
		for _, file := range files {
			contents, issue := readBounded(file.path, config.MaxSkillBytes)
			if issue != nil {
				catalog.Diagnostics = append(catalog.Diagnostics, *issue)
				continue
			}
			metadata, issues := ParseFrontmatter(contents, file.logical)
			catalog.Diagnostics = append(catalog.Diagnostics, issues...)
			if hasErrors(issues) {
				continue
			}
			directory := filepath.Dir(file.path)
			logicalName := filepath.Base(filepath.Dir(file.logical))
			if !strings.EqualFold(metadata.Name, logicalName) {
				catalog.Diagnostics = append(catalog.Diagnostics, Diagnostic{
					Severity: SeverityWarning, Code: "directory-name-mismatch", Path: file.logical,
					Message: fmt.Sprintf("skill name %q does not match directory %q", metadata.Name, logicalName),
				})
			}
			candidates = append(candidates, candidate{
				path: file.path, logical: file.logical, root: root,
				metadata: metadata, directory: directory,
			})
		}
	}

	seenNames := make(map[string]Skill)
	for _, item := range candidates {
		key := strings.ToLower(item.metadata.Name)
		if winner, exists := seenNames[key]; exists {
			catalog.Diagnostics = append(catalog.Diagnostics, Diagnostic{
				Severity: SeverityWarning, Code: "duplicate-name", Path: item.logical,
				Message: fmt.Sprintf("duplicate skill %q ignored; %s takes precedence", item.metadata.Name, winner.Path),
			})
			continue
		}
		enabled, reason := rules.enabled(item.path, item.directory)
		skill := Skill{
			Name: item.metadata.Name, Description: item.metadata.Description,
			Path: item.path, Directory: item.directory, Root: item.root.path,
			Scope: item.root.scope, Plugin: item.root.plugin, Enabled: enabled,
			DisabledReason: reason,
		}
		seenNames[key] = skill
		catalog.Skills = append(catalog.Skills, skill)
	}

	sort.Slice(catalog.Skills, func(i, j int) bool {
		return strings.ToLower(catalog.Skills[i].Name) < strings.ToLower(catalog.Skills[j].Name)
	})
	sort.SliceStable(catalog.Diagnostics, func(i, j int) bool {
		a, b := catalog.Diagnostics[i], catalog.Diagnostics[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Code < b.Code
	})
	return catalog, nil
}

func discoveryRoots(config Config) ([]rootSpec, error) {
	var roots []rootSpec
	if config.ProjectDir != "" {
		project, err := filepath.Abs(config.ProjectDir)
		if err != nil {
			return nil, fmt.Errorf("skills: resolve project folder: %w", err)
		}
		if info, err := os.Stat(project); err == nil && !info.IsDir() {
			project = filepath.Dir(project)
		}
		if resolved, err := filepath.EvalSymlinks(project); err == nil {
			project = resolved
		}
		for current := project; ; current = filepath.Dir(current) {
			for _, relative := range []string{".agents/skills", ".codex/skills", ".pi/skills"} {
				path := filepath.Join(current, filepath.FromSlash(relative))
				if info, err := os.Stat(path); err == nil && info.IsDir() {
					roots = append(roots, rootSpec{
						path: canonical(path), logical: path, boundary: canonical(current),
						scope: ScopeProject, allowExternal: config.AllowExternalSymlinks,
					})
				}
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
		}
	}
	for _, path := range config.GlobalRoots {
		resolved := canonical(path)
		roots = append(roots, rootSpec{path: resolved, logical: path, boundary: resolved, scope: ScopeGlobal, allowExternal: config.AllowExternalSymlinks})
	}
	for _, plugin := range config.PluginRoots {
		resolved := canonical(plugin.Path)
		roots = append(roots, rootSpec{
			path: resolved, logical: plugin.Path, boundary: resolved, scope: ScopePlugin, plugin: plugin.Name,
			allowExternal: config.AllowExternalSymlinks,
		})
	}
	return dedupeRoots(roots), nil
}

func dedupeRoots(roots []rootSpec) []rootSpec {
	seen := make(map[string]bool)
	result := make([]rootSpec, 0, len(roots))
	for _, root := range roots {
		key := string(root.scope) + "\x00" + root.plugin + "\x00" + root.path
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, root)
	}
	return result
}

func canonical(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Clean(path)
}

func hasErrors(issues []Diagnostic) bool {
	for _, issue := range issues {
		if issue.Severity == SeverityError {
			return true
		}
	}
	return false
}
