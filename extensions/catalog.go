package extensions

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func scanRoot(ctx context.Context, root string, maxDepth, maxEntries int) ([]Extension, []Diagnostic, error) {
	rawRoot := strings.TrimSpace(root)
	root, err := filepath.Abs(rawRoot)
	if err != nil || rawRoot == "" {
		return nil, []Diagnostic{{Severity: SeverityError, Code: "plugin-root-invalid", Message: "plugin root is required", Path: root}}, nil
	}
	if evaluated, evalErr := filepath.EvalSymlinks(root); evalErr == nil {
		root = evaluated
	}
	if !isDirectory(root) {
		return nil, []Diagnostic{{Severity: SeverityError, Code: "plugin-root-not-found", Message: "plugin root is not a directory", Path: root}}, nil
	}
	var entries []Extension
	var diagnostics []Diagnostic
	count := 0
	err = filepath.WalkDir(root, func(path string, item fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			diagnostics = append(diagnostics, Diagnostic{Severity: SeverityWarning, Code: "plugin-scan-error", Message: walkErr.Error(), Path: path})
			return nil
		}
		relative, _ := filepath.Rel(root, path)
		depth := 0
		if relative != "." {
			depth = strings.Count(relative, string(filepath.Separator)) + 1
		}
		if item.IsDir() {
			if path != root && (depth > maxDepth || ignoredDirectory(item.Name())) {
				return filepath.SkipDir
			}
			return nil
		}
		format := Format("")
		if item.Name() == "plugin.json" && filepath.Base(filepath.Dir(path)) == ".codex-plugin" {
			format = FormatCodex
		} else if item.Name() == "package.json" {
			format = FormatPi
		}
		if format == "" {
			return nil
		}
		data, parseErr := parseManifest(path, format)
		if errors.Is(parseErr, errNotPiPackage) {
			return nil
		}
		if count >= maxEntries {
			diagnostics = append(diagnostics, Diagnostic{Severity: SeverityWarning, Code: "plugin-scan-limit", Message: fmt.Sprintf("stopped after %d manifests", maxEntries), Path: root})
			return filepath.SkipAll
		}
		count++
		pluginRoot := filepath.Dir(path)
		if format == FormatCodex {
			pluginRoot = filepath.Dir(pluginRoot)
		}
		if parseErr != nil {
			diagnostics = append(diagnostics, Diagnostic{Severity: SeverityWarning, Code: "manifest-invalid", Message: parseErr.Error(), Path: path})
			return nil
		}
		source := Source{Location: pluginRoot, Enabled: true, Trust: TrustPrompt}
		entry := entryFromManifest(source, pluginRoot, path, data)
		entries = append(entries, entry)
		diagnostics = append(diagnostics, entry.Diagnostics...)
		if format == FormatCodex {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, diagnostics, err
	}
	return entries, diagnostics, nil
}

func ignoredDirectory(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".cache", "dist", "build":
		return true
	default:
		return false
	}
}

func addEntry(catalog *Catalog, entry Extension, seen map[string]struct{}) {
	key := entry.Root
	if entry.Manifest == "" {
		key = entry.Root + "\x00" + strings.Join(entry.Entrypoints, "\x00")
	}
	if _, exists := seen[key]; exists {
		return
	}
	seen[key] = struct{}{}
	catalog.Extensions = append(catalog.Extensions, entry)
}

func markDuplicateIDs(catalog *Catalog) {
	byID := map[string][]int{}
	for index := range catalog.Extensions {
		byID[catalog.Extensions[index].ID] = append(byID[catalog.Extensions[index].ID], index)
	}
	for id, indexes := range byID {
		if len(indexes) < 2 {
			continue
		}
		for _, index := range indexes {
			diagnostic := Diagnostic{Severity: SeverityError, Code: "duplicate-id", Message: "multiple extensions use id " + id, Path: catalog.Extensions[index].Root, ExtensionID: id}
			catalog.Extensions[index].State = StateInvalid
			catalog.Extensions[index].Diagnostics = append(catalog.Extensions[index].Diagnostics, diagnostic)
			catalog.Diagnostics = append(catalog.Diagnostics, diagnostic)
		}
	}
}

func sortDiagnostics(diagnostics []Diagnostic) {
	sort.Slice(diagnostics, func(i, j int) bool {
		left, right := diagnostics[i], diagnostics[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		return left.ExtensionID < right.ExtensionID
	})
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (c Catalog) Find(id string) (Extension, bool) {
	for _, extension := range c.Extensions {
		if extension.ID == id {
			return extension, true
		}
	}
	return Extension{}, false
}
