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

// Discover catalogs configured sources and compatible plugin manifests. Bad
// entries become diagnostics so one broken plugin cannot hide the others.
func Discover(ctx context.Context, options DiscoverOptions) (Catalog, error) {
	if options.MaxDepth <= 0 {
		options.MaxDepth = 8
	}
	if options.MaxEntries <= 0 {
		options.MaxEntries = 512
	}
	var catalog Catalog
	seen := map[string]struct{}{}
	for _, source := range options.Sources {
		if err := ctx.Err(); err != nil {
			return Catalog{}, err
		}
		entry, diagnostics := discoverSource(source)
		catalog.Diagnostics = append(catalog.Diagnostics, diagnostics...)
		if entry != nil {
			addEntry(&catalog, *entry, seen)
		}
	}
	for _, root := range options.PluginRoots {
		entries, diagnostics, err := scanRoot(ctx, root, options.MaxDepth, options.MaxEntries)
		if err != nil {
			return Catalog{}, err
		}
		catalog.Diagnostics = append(catalog.Diagnostics, diagnostics...)
		for _, entry := range entries {
			addEntry(&catalog, entry, seen)
		}
	}
	markDuplicateIDs(&catalog)
	sort.Slice(catalog.Extensions, func(i, j int) bool {
		if catalog.Extensions[i].ID == catalog.Extensions[j].ID {
			return catalog.Extensions[i].Root < catalog.Extensions[j].Root
		}
		return catalog.Extensions[i].ID < catalog.Extensions[j].ID
	})
	sortDiagnostics(catalog.Diagnostics)
	return catalog, nil
}

func discoverSource(source Source) (*Extension, []Diagnostic) {
	location, err := filepath.Abs(strings.TrimSpace(source.Location))
	if err != nil || strings.TrimSpace(source.Location) == "" {
		return nil, []Diagnostic{{Severity: SeverityError, Code: "source-path-invalid", Message: "source location is required", Path: source.Location}}
	}
	info, err := os.Stat(location)
	if err != nil {
		return nil, []Diagnostic{{Severity: SeverityError, Code: "source-not-found", Message: err.Error(), Path: location}}
	}
	if evaluated, evalErr := filepath.EvalSymlinks(location); evalErr == nil {
		location = evaluated
	}
	manifest, format, root := manifestAt(location, info)
	if manifest == "" {
		entry := localEntry(source, location, info)
		return entry, entry.Diagnostics
	}
	data, err := parseManifest(manifest, format)
	if err != nil {
		if format == FormatPi && errors.Is(err, errNotPiPackage) {
			entry := localEntry(source, location, info)
			return entry, entry.Diagnostics
		}
		id := normalizeID(firstNonEmpty(source.ID, filepath.Base(root)))
		diagnostic := Diagnostic{Severity: SeverityError, Code: "manifest-invalid", Message: err.Error(), Path: manifest, ExtensionID: id}
		entry := baseEntry(source, id, filepath.Base(root), root)
		entry.Manifest, entry.Format, entry.State = manifest, format, StateInvalid
		entry.Diagnostics = []Diagnostic{diagnostic}
		return &entry, []Diagnostic{diagnostic}
	}
	entry := entryFromManifest(source, root, manifest, data)
	return &entry, entry.Diagnostics
}

func manifestAt(location string, info fs.FileInfo) (string, Format, string) {
	if !info.IsDir() {
		switch {
		case filepath.Base(location) == "plugin.json" && filepath.Base(filepath.Dir(location)) == ".codex-plugin":
			return location, FormatCodex, filepath.Dir(filepath.Dir(location))
		case filepath.Base(location) == "package.json":
			return location, FormatPi, filepath.Dir(location)
		default:
			return "", "", filepath.Dir(location)
		}
	}
	codex := filepath.Join(location, ".codex-plugin", "plugin.json")
	if fileExists(codex) {
		return codex, FormatCodex, location
	}
	pi := filepath.Join(location, "package.json")
	if fileExists(pi) {
		return pi, FormatPi, location
	}
	return "", "", location
}

func localEntry(source Source, location string, info fs.FileInfo) *Extension {
	root, name := location, filepath.Base(location)
	var entrypoints []string
	if !info.IsDir() {
		root, name, entrypoints = filepath.Dir(location), strings.TrimSuffix(name, filepath.Ext(name)), []string{location}
	}
	id := normalizeID(firstNonEmpty(source.ID, name))
	entry := baseEntry(source, id, name, root)
	entry.Format, entry.Entrypoints = FormatLocal, entrypoints
	if skillPath := filepath.Join(root, "skills"); isDirectory(skillPath) {
		entry.SkillRoots = []string{skillPath}
	}
	finishState(&entry)
	return &entry
}

func entryFromManifest(source Source, root, manifest string, data manifestData) Extension {
	id := normalizeID(firstNonEmpty(source.ID, data.name))
	entry := baseEntry(source, id, data.name, root)
	entry.Version, entry.Description, entry.Format, entry.Manifest = data.version, data.description, data.format, manifest
	entry.HasMCP, entry.HasApp = data.hasMCP, data.hasApp
	if data.format == FormatCodex && len(data.skills) == 0 && isDirectory(filepath.Join(root, "skills")) {
		data.skills = []string{"skills"}
	}
	entry.Entrypoints, entry.Diagnostics = resolvePaths(root, data.entrypoints, "extension", id)
	entry.SkillRoots, entry.Diagnostics = resolveAndAppend(root, data.skills, "skill", id, entry.Diagnostics)
	entry.PromptRoots, entry.Diagnostics = resolveAndAppend(root, data.prompts, "prompt", id, entry.Diagnostics)
	entry.ThemeRoots, entry.Diagnostics = resolveAndAppend(root, data.themes, "theme", id, entry.Diagnostics)
	if source.ID == "" && id != data.name {
		entry.Diagnostics = append(entry.Diagnostics, Diagnostic{Severity: SeverityInfo, Code: "id-normalized", Message: fmt.Sprintf("normalized %q to %q", data.name, id), Path: manifest, ExtensionID: id})
	}
	finishState(&entry)
	return entry
}

func resolveAndAppend(root string, values []string, kind, id string, diagnostics []Diagnostic) ([]string, []Diagnostic) {
	paths, more := resolvePaths(root, values, kind, id)
	return paths, append(diagnostics, more...)
}
