package extensions

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestDiscoverCodexAndPiPlugins(t *testing.T) {
	root := t.TempDir()
	codex := filepath.Join(root, "codex")
	makeDir(t, filepath.Join(codex, ".codex-plugin"))
	makeDir(t, filepath.Join(codex, "skills", "review"))
	writeFile(t, filepath.Join(codex, ".codex-plugin", "plugin.json"), `{
  "name":"Acme Plugin","version":"1.2.3","description":"Codex fixture",
  "skills":"./skills","apps":"./.app.json"
}`)
	writeFile(t, filepath.Join(codex, "skills", "review", "SKILL.md"), "# Review")
	writeFile(t, filepath.Join(codex, ".mcp.json"), `{}`)

	pi := filepath.Join(root, "pi")
	makeDir(t, filepath.Join(pi, "skills"))
	makeDir(t, filepath.Join(pi, "prompts"))
	makeDir(t, filepath.Join(pi, "themes"))
	writeFile(t, filepath.Join(pi, "extension.ts"), "export default {}")
	writeFile(t, filepath.Join(pi, "package.json"), `{
  "name":"@scope/pi-demo","version":"2.0.0","pi":{
    "extensions":["extension.ts"],"skills":"skills",
    "prompts":["prompts"],"themes":["themes"]
  }
}`)
	other := filepath.Join(root, "other")
	makeDir(t, other)
	writeFile(t, filepath.Join(other, "package.json"), `{"name":"ordinary-package"}`)

	catalog, err := Discover(context.Background(), DiscoverOptions{
		Sources: []Source{{
			Location: codex, Enabled: true, Trust: TrustTrusted,
			Command: "adapter", Args: []string{"--literal", "$(not-a-shell)"},
		}},
		PluginRoots: []string{root},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Extensions) != 2 {
		t.Fatalf("extensions = %#v", catalog.Extensions)
	}
	codexEntry, ok := catalog.Find("acme_plugin")
	if !ok || codexEntry.Format != FormatCodex || codexEntry.State != StateRunnable {
		t.Fatalf("codex entry = %#v, found %v", codexEntry, ok)
	}
	if !codexEntry.HasApp || !codexEntry.HasMCP || len(codexEntry.SkillRoots) != 1 {
		t.Fatalf("Codex resources = %#v", codexEntry)
	}
	if !slices.Equal(codexEntry.Process.Args, []string{"--literal", "$(not-a-shell)"}) {
		t.Fatalf("args changed: %#v", codexEntry.Process.Args)
	}
	piEntry, ok := catalog.Find("scope_pi-demo")
	if !ok || piEntry.Format != FormatPi || piEntry.State != StateNeedsTrust {
		t.Fatalf("Pi entry = %#v, found %v", piEntry, ok)
	}
	if len(piEntry.Entrypoints) != 1 || len(piEntry.SkillRoots) != 1 ||
		len(piEntry.PromptRoots) != 1 || len(piEntry.ThemeRoots) != 1 {
		t.Fatalf("Pi resources = %#v", piEntry)
	}
}

func TestDiscoverRejectsEscapesAndDuplicateIDs(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	makeDir(t, outside)
	plugin := filepath.Join(root, "plugin")
	makeDir(t, plugin)
	writeFile(t, filepath.Join(plugin, "package.json"), `{
  "name":"unsafe","pi":{"skills":["../outside"]}
}`)
	first := filepath.Join(root, "one.js")
	second := filepath.Join(root, "two.js")
	writeFile(t, first, "one")
	writeFile(t, second, "two")
	catalog, err := Discover(context.Background(), DiscoverOptions{Sources: []Source{
		{Location: plugin, Enabled: true, Trust: TrustPrompt},
		{ID: "same", Location: first, Enabled: true, Trust: TrustTrusted, Command: "adapter"},
		{ID: "same", Location: second, Enabled: true, Trust: TrustTrusted, Command: "adapter"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	unsafe, _ := catalog.Find("unsafe")
	if len(unsafe.SkillRoots) != 0 || !hasDiagnostic(unsafe.Diagnostics, "skill-outside-root") {
		t.Fatalf("escape was accepted: %#v", unsafe)
	}
	duplicates := 0
	for _, entry := range catalog.Extensions {
		if entry.ID == "same" && entry.State == StateInvalid && hasDiagnostic(entry.Diagnostics, "duplicate-id") {
			duplicates++
		}
	}
	if duplicates != 2 {
		t.Fatalf("duplicate entries = %d, catalog %#v", duplicates, catalog)
	}
}

func TestDiscoverStateIsStable(t *testing.T) {
	root := t.TempDir()
	states := []struct {
		name    string
		enabled bool
		trust   Trust
		command string
		want    State
	}{
		{"disabled", false, TrustTrusted, "adapter", StateDisabled},
		{"denied", true, TrustDenied, "adapter", StateDenied},
		{"prompt", true, TrustPrompt, "adapter", StateNeedsTrust},
		{"catalog", true, TrustTrusted, "", StateCatalogued},
		{"ready", true, TrustTrusted, "adapter", StateRunnable},
	}
	for _, test := range states {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(root, test.name)
			makeDir(t, path)
			catalog, err := Discover(context.Background(), DiscoverOptions{Sources: []Source{{
				ID: test.name, Location: path, Enabled: test.enabled, Trust: test.trust, Command: test.command,
			}}})
			if err != nil || len(catalog.Extensions) != 1 {
				t.Fatalf("discover: %v, %#v", err, catalog)
			}
			if got := catalog.Extensions[0].State; got != test.want {
				t.Fatalf("state = %q, want %q", got, test.want)
			}
		})
	}
}

func TestExplicitNodeAdapterDoesNotRequirePiMetadata(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"plain-node-adapter"}`)
	catalog, err := Discover(context.Background(), DiscoverOptions{Sources: []Source{{
		ID: "adapter", Location: root, Enabled: true, Trust: TrustTrusted, Command: "node",
	}}})
	if err != nil || len(catalog.Extensions) != 1 {
		t.Fatalf("discover = %#v, %v", catalog, err)
	}
	entry := catalog.Extensions[0]
	if entry.Format != FormatLocal || entry.State != StateRunnable {
		t.Fatalf("entry = %#v", entry)
	}
}

func hasDiagnostic(values []Diagnostic, code string) bool {
	return slices.ContainsFunc(values, func(value Diagnostic) bool { return value.Code == code })
}

func makeDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
