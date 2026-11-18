package nodefs

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMkdirAll(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MkdirAll(filepath.Join(dir, "a", "b", "c"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatalf("MkdirAll of an existing directory: %v", err)
	}
	// Node: an existing file is EEXIST, a file among the parents ENOTDIR.
	for path, code := range map[string]string{file: "EEXIST", filepath.Join(file, "x", "y"): "ENOTDIR"} {
		var e *Error
		err := MkdirAll(path, 0o755)
		if !errors.As(err, &e) || e.Code != code || e.Syscall != "mkdir" {
			t.Errorf("MkdirAll(%s) = %v, want %s", path, err, code)
		}
	}
}

func TestReaddirAndErrors(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b", "C", "a", "é", "_"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	names, err := Readdir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"C", "_", "a", "b", "é"}; !reflect.DeepEqual(names, want) {
		t.Errorf("Readdir = %q, want %q", names, want)
	}
	_, err = Stat(filepath.Join(dir, "missing"))
	want := "ENOENT: no such file or directory, stat '" + filepath.Join(dir, "missing") + "'"
	if err == nil || err.Error() != want {
		t.Errorf("Stat error = %v, want %s", err, want)
	}
	e := NewError(os.ErrExist, "symlink", "target")
	e.Dest = "path"
	if got := e.Error(); got != "UNKNOWN: unknown error, symlink 'target' -> 'path'" {
		t.Errorf("two-path message = %q", got)
	}
}

func TestRemoveAll(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a", "b", "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/nowhere", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveAll(filepath.Join(dir, "missing")); err != nil {
		t.Errorf("RemoveAll of a missing path: %v", err)
	}
	// Like Node's rmSync, removing "." deletes the contents, then fails on rmdir.
	t.Chdir(dir)
	err := RemoveAll(".")
	if want := ", Unknown error: Invalid argument '.'"; err == nil || err.Error() != want {
		t.Errorf("RemoveAll(\".\") = %v, want %s", err, want)
	}
	if names, _ := Readdir("."); len(names) != 0 {
		t.Errorf("left behind: %q", names)
	}
}
