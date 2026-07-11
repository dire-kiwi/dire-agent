package configuration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtensionProcessSettingsPatchAndCopy(t *testing.T) {
	home := t.TempDir()
	config := DefaultConfig(home)
	config.Global.Extensions.Sources["adapter"] = ExtensionSource{
		Kind: ExtensionLocal, Location: filepath.Join(home, "plugin"), Trust: TrustTrusted,
		Enabled: true, Command: "node", Args: []string{"adapter.js"},
		Env: map[string]string{"TOKEN": "global"}, SecretEnv: []string{"TOKEN"},
	}
	command := "bun"
	args := []string{"run", "adapter.ts"}
	env := map[string]string{"TOKEN": "project"}
	secrets := []string{"TOKEN"}
	inherit := true
	config.Projects["project"] = ProjectOverride{
		Folder: filepath.Join(home, "project"),
		Settings: SettingsPatch{Extensions: &ExtensionPatch{Sources: map[string]ExtensionSourcePatch{
			"adapter": {Command: &command, Args: &args, Env: &env, SecretEnv: &secrets, InheritEnv: &inherit},
		}}},
	}
	if err := Validate(config); err != nil {
		t.Fatal(err)
	}
	effective, found := config.Effective("project")
	if !found {
		t.Fatal("project not found")
	}
	source := effective.Extensions.Sources["adapter"]
	if source.Command != "bun" || !source.InheritEnv || source.Env["TOKEN"] != "project" {
		t.Fatalf("effective source = %+v", source)
	}
	source.Args[0] = "changed"
	source.Env["TOKEN"] = "changed"
	if config.Projects["project"].Settings.Extensions.Sources["adapter"].Args == nil ||
		(*config.Projects["project"].Settings.Extensions.Sources["adapter"].Args)[0] == "changed" {
		t.Fatal("effective extension aliases patch")
	}
}

func TestStoreRedactsExtensionEnvironmentAndRestoresPlaceholder(t *testing.T) {
	home := t.TempDir()
	defaults := DefaultConfig(home)
	defaults.Global.Extensions.Sources["adapter"] = ExtensionSource{
		Kind: ExtensionLocal, Location: filepath.Join(home, "plugin"), Trust: TrustTrusted,
		Enabled: true, Command: "adapter",
		Env:       map[string]string{"VISIBLE": "yes", "OPAQUE": "extension-secret", "API_TOKEN": "automatic-secret"},
		SecretEnv: []string{"OPAQUE"},
	}
	path := filepath.Join(t.TempDir(), "config.json")
	store, err := NewStore(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	public, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	source := public.Global.Extensions.Sources["adapter"]
	if source.Env["OPAQUE"] != RedactedValue || source.Env["API_TOKEN"] != RedactedValue || source.Env["VISIBLE"] != "yes" {
		t.Fatalf("public environment = %v", source.Env)
	}
	public.Global.Thinking.Level = ThinkingHigh
	if _, err := store.Update(context.Background(), public.Revision, public); err != nil {
		t.Fatal(err)
	}
	runtime, _, err := store.RuntimeSettings(context.Background(), "")
	if err != nil || runtime.Extensions.Sources["adapter"].Env["OPAQUE"] != "extension-secret" {
		t.Fatalf("runtime source = %+v, %v", runtime.Extensions.Sources["adapter"], err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "extension-secret") || !strings.Contains(string(contents), "automatic-secret") {
		t.Fatal("redacted placeholders replaced stored secrets")
	}
}

func TestExtensionValidationRejectsUnsafeAndRemoteProcesses(t *testing.T) {
	tests := []ExtensionSource{
		{Kind: ExtensionLocal, Location: "/tmp/plugin", Trust: TrustTrusted, Enabled: true, Command: "bad\x00command"},
		{Kind: ExtensionLocal, Location: "/tmp/plugin", Trust: TrustTrusted, Enabled: true, Command: "adapter", Env: map[string]string{"BAD=KEY": "value"}},
		{Kind: ExtensionLocal, Location: "/tmp/plugin", Trust: TrustTrusted, Enabled: true, Command: "adapter", SecretEnv: []string{"MISSING"}},
		{Kind: ExtensionGit, Location: "https://example.test/plugin.git", Trust: TrustTrusted, Enabled: true, Command: "adapter"},
	}
	for index, source := range tests {
		config := DefaultConfig(t.TempDir())
		config.Global.Extensions.Sources["bad"] = source
		if err := Validate(config); err == nil {
			t.Fatalf("case %d accepted", index)
		}
	}
}

func TestMCPInheritEnvironmentPatchesAndHTTPRejectsIt(t *testing.T) {
	config := DefaultConfig(t.TempDir())
	server := validStdioServer()
	config.Global.MCP.Servers["local"] = server
	inherit := true
	config.Projects["project"] = ProjectOverride{
		Folder: filepath.Join(t.TempDir(), "project"),
		Settings: SettingsPatch{MCP: &MCPPatch{Servers: map[string]MCPServerPatch{
			"local": {InheritEnv: &inherit},
		}}},
	}
	if err := Validate(config); err != nil {
		t.Fatal(err)
	}
	effective, _ := config.Effective("project")
	if !effective.MCP.Servers["local"].InheritEnv {
		t.Fatal("inherit_env patch was ignored")
	}
	http := validHTTPServer()
	http.InheritEnv = true
	config.Global.MCP.Servers["http"] = http
	if err := Validate(config); err == nil {
		t.Fatal("HTTP MCP accepted process environment inheritance")
	}
}
