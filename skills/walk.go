package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type skillFile struct {
	path    string
	logical string
}

func walkSkillFiles(root rootSpec) ([]skillFile, []Diagnostic) {
	if root.path == "" {
		return nil, []Diagnostic{{
			Severity: SeverityError, Code: "empty-root", Message: "skill root path is empty",
		}}
	}
	if !root.allowExternal && root.boundary != "" && !containsPath(root.boundary, root.path) {
		return nil, []Diagnostic{{
			Severity: SeverityWarning, Code: "symlink-root-escape", Path: root.logical,
			Message: "ignoring project skill root symlinked outside the project ancestor",
		}}
	}
	info, err := os.Stat(root.path)
	if err != nil {
		severity := SeverityWarning
		if !os.IsNotExist(err) {
			severity = SeverityError
		}
		return nil, []Diagnostic{{
			Severity: severity, Code: "root-unavailable", Path: root.path,
			Message: fmt.Sprintf("cannot scan skill root: %v", err),
		}}
	}
	if !info.IsDir() {
		if filepath.Base(root.path) != "SKILL.md" || !info.Mode().IsRegular() {
			return nil, []Diagnostic{{
				Severity: SeverityError, Code: "invalid-root", Path: root.path,
				Message: "skill root must be a directory or SKILL.md file",
			}}
		}
		return []skillFile{{path: canonical(root.path), logical: root.path}}, nil
	}

	var files []skillFile
	var issues []Diagnostic
	visited := make(map[string]bool)
	var walk func(string)
	walk = func(path string) {
		resolved := canonical(path)
		if !root.allowExternal && !containsPath(root.path, resolved) {
			issues = append(issues, Diagnostic{
				Severity: SeverityWarning, Code: "symlink-escape", Path: path,
				Message: "ignoring symlinked directory outside the skill root",
			})
			return
		}
		if visited[resolved] {
			return
		}
		visited[resolved] = true
		entries, err := os.ReadDir(path)
		if err != nil {
			issues = append(issues, Diagnostic{
				Severity: SeverityError, Code: "directory-unreadable", Path: path,
				Message: fmt.Sprintf("cannot read skill directory: %v", err),
			})
			return
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			child := filepath.Join(path, entry.Name())
			info, err := os.Stat(child) // Stat intentionally follows symlinks.
			if err != nil {
				issues = append(issues, Diagnostic{
					Severity: SeverityWarning, Code: "entry-unavailable", Path: child,
					Message: fmt.Sprintf("cannot inspect skill entry: %v", err),
				})
				continue
			}
			if info.IsDir() {
				walk(child)
				continue
			}
			if entry.Name() == "SKILL.md" && info.Mode().IsRegular() {
				files = append(files, skillFile{path: canonical(child), logical: child})
			}
		}
	}
	walk(root.path)
	sort.Slice(files, func(i, j int) bool { return files[i].logical < files[j].logical })
	return files, issues
}
