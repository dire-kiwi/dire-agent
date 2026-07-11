package extensions

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func resolvePaths(root string, values []string, kind, id string) ([]string, []Diagnostic) {
	var resolved []string
	var diagnostics []Diagnostic
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			diagnostics = append(diagnostics, pathDiagnostic(id, kind+"-path-empty", "empty "+kind+" path", root))
			continue
		}
		pattern := value
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(root, pattern)
		}
		matches := []string{pattern}
		if hasGlob(value) {
			var err error
			matches, err = filepath.Glob(pattern)
			if err != nil {
				diagnostics = append(diagnostics, pathDiagnostic(id, kind+"-glob-invalid", err.Error(), pattern))
				continue
			}
			if len(matches) == 0 {
				diagnostics = append(diagnostics, pathDiagnostic(id, kind+"-not-found", "no paths matched", pattern))
			}
		}
		for _, match := range matches {
			path, err := containedPath(root, match)
			if err != nil {
				diagnostics = append(diagnostics, pathDiagnostic(id, kind+"-outside-root", err.Error(), match))
				continue
			}
			if _, err := os.Stat(path); err != nil {
				diagnostics = append(diagnostics, pathDiagnostic(id, kind+"-not-found", err.Error(), path))
				continue
			}
			if _, ok := seen[path]; !ok {
				seen[path] = struct{}{}
				resolved = append(resolved, path)
			}
		}
	}
	sort.Strings(resolved)
	return resolved, diagnostics
}

func containedPath(root, candidate string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if evaluated, evalErr := filepath.EvalSymlinks(root); evalErr == nil {
		root = evaluated
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	if evaluated, evalErr := filepath.EvalSymlinks(candidate); evalErr == nil {
		candidate = evaluated
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes extension root")
	}
	return filepath.Clean(candidate), nil
}

func hasGlob(path string) bool { return strings.ContainsAny(path, "*?[") }

func pathDiagnostic(id, code, message, path string) Diagnostic {
	return Diagnostic{Severity: SeverityWarning, Code: code, Message: message, Path: path, ExtensionID: id}
}
