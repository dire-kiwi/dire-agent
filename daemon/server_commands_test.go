package daemon

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCompleteFolderPath(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"Alpha", "alpine", "beta", ".hidden"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "also-a-file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := completeFolderPath(filepath.Join(root, "al"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "Alpha"), filepath.Join(root, "alpine")}
	if got := result["folders"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("folders = %#v, want %#v", got, want)
	}
}

func TestCompleteFolderPathRejectsRelativePaths(t *testing.T) {
	result, err := completeFolderPath("relative/path")
	if err != nil {
		t.Fatal(err)
	}
	if got := result["folders"]; !reflect.DeepEqual(got, []string{}) {
		t.Fatalf("folders = %#v, want empty", got)
	}
}

func TestCompleteFolderPathPreservesTildeDirectorySeparator(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := filepath.Join(home, "work")
	if err := os.MkdirAll(filepath.Join(work, "project-a"), 0o700); err != nil {
		t.Fatal(err)
	}
	tilde, err := completeFolderPath("~/")
	if err != nil {
		t.Fatal(err)
	}
	absolute, err := completeFolderPath(home + string(filepath.Separator))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tilde["folders"], absolute["folders"]) {
		t.Fatalf("~/ folders = %#v, want %#v", tilde["folders"], absolute["folders"])
	}
	nested, err := completeFolderPath("~/work/")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := nested["folders"], []string{filepath.Join(work, "project-a")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("~/work/ folders = %#v, want %#v", got, want)
	}
}
