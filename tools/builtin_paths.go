package tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type pathSandbox struct {
	main       string
	additional []string
	roots      []string
}

func decode(raw json.RawMessage, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	return nil
}

func newPathSandbox(main string, additional []string) (pathSandbox, error) {
	mainInput, err := filepath.Abs(strings.TrimSpace(main))
	if err != nil {
		return pathSandbox{}, fmt.Errorf("tools: resolve main project folder: %w", err)
	}
	main, err = canonicalDirectory(main)
	if err != nil {
		return pathSandbox{}, fmt.Errorf("tools: resolve main project folder: %w", err)
	}
	seen := map[string]bool{main: true}
	normalized := make([]string, 0, len(additional))
	rootCandidates := []string{mainInput, main}
	for _, folder := range additional {
		folder = strings.TrimSpace(folder)
		if folder == "" {
			continue
		}
		resolved, err := canonicalDirectory(folder)
		if err != nil {
			return pathSandbox{}, fmt.Errorf("tools: resolve additional sandbox folder %q: %w", folder, err)
		}
		if filepath.Dir(resolved) == resolved {
			return pathSandbox{}, errors.New("tools: filesystem root cannot be an additional sandbox folder")
		}
		if seen[resolved] || within(main, resolved) {
			continue
		}
		absolute, err := filepath.Abs(folder)
		if err != nil {
			return pathSandbox{}, fmt.Errorf("tools: resolve additional sandbox folder %q: %w", folder, err)
		}
		seen[resolved] = true
		normalized = append(normalized, resolved)
		rootCandidates = append(rootCandidates, absolute, resolved)
	}
	sort.Strings(normalized)
	return pathSandbox{
		main: main, additional: normalized,
		roots: normalizedPaths(rootCandidates),
	}, nil
}

func (s pathSandbox) secureExistingPath(requested string) (string, error) {
	path, err := s.secureLexicalPath(requested)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if !s.contains(resolved) {
		return "", errors.New("path escapes the project sandbox")
	}
	return resolved, nil
}

func (s pathSandbox) secureNewPath(requested string) (string, error) {
	path, err := s.secureLexicalPath(requested)
	if err != nil {
		return "", err
	}
	parent := filepath.Dir(path)
	for {
		resolved, resolveErr := filepath.EvalSymlinks(parent)
		if resolveErr == nil {
			if !s.contains(resolved) {
				return "", errors.New("path escapes the project sandbox")
			}
			return path, nil
		}
		next := filepath.Dir(parent)
		if next == parent {
			return "", resolveErr
		}
		parent = next
	}
}

func (s pathSandbox) secureLexicalPath(requested string) (string, error) {
	if strings.TrimSpace(requested) == "" {
		return "", errors.New("path is required")
	}
	path := requested
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.main, path)
	}
	path = filepath.Clean(path)
	if !s.contains(path) {
		return "", errors.New("path escapes the project sandbox")
	}
	return path, nil
}

func (s pathSandbox) contains(path string) bool {
	for _, root := range s.roots {
		if within(root, path) {
			return true
		}
	}
	return false
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (s pathSandbox) displayPath(path string) string {
	if !within(s.main, path) {
		return path
	}
	rel, err := filepath.Rel(s.main, path)
	if err != nil {
		return path
	}
	return rel
}
