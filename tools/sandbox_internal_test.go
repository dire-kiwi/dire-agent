package tools

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDefaultSandboxProfileHasWorkspaceAndNetworkBoundaries(t *testing.T) {
	root := filepath.Join(t.TempDir(), `project "quoted"`)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := defaultSandboxProfile(root)
	if err != nil {
		t.Fatal(err)
	}

	for _, required := range []string{
		"(deny default)",
		"(import \"system.sb\")",
		"(deny network*)",
		"(deny file-write*)",
		"(allow process*)",
		"(allow file-read* file-test-existence",
		"(allow file-write*",
		"(subpath " + strconv.Quote(canonical) + ")",
	} {
		if !strings.Contains(profile, required) {
			t.Fatalf("profile does not contain %q:\n%s", required, profile)
		}
	}
	if strings.Contains(profile, "(allow network") {
		t.Fatalf("profile directly allows network access:\n%s", profile)
	}
	if strings.Contains(profile, "(allow file-read*)") || strings.Contains(profile, "(allow file-write*)") {
		t.Fatalf("profile contains an unfiltered file grant:\n%s", profile)
	}
}

func TestSandboxProfileAllowsIncludedFolderWritesWithoutChangingWorkspace(t *testing.T) {
	main := t.TempDir()
	extra := t.TempDir()
	profile, err := sandboxProfileWithWritePaths(main, nil, []string{extra}, false)
	if err != nil {
		t.Fatal(err)
	}
	canonicalMain, _ := filepath.EvalSymlinks(main)
	canonicalExtra, _ := filepath.EvalSymlinks(extra)
	for _, folder := range []string{canonicalMain, canonicalExtra} {
		if !strings.Contains(profile, "(subpath "+strconv.Quote(folder)+")") {
			t.Fatalf("profile does not include writable folder %q:\n%s", folder, profile)
		}
	}
}
