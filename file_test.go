package grunt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntjs/grunt-go/js"
)

// TestExpandGlobErrors checks how an unreadable directory is reported: glob prints
// it with console.error when strict is off (Node 24 prints the error's stack where
// this prints [Error: message]), stays quiet with silent, and throws by default.
func TestExpandGlobErrors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply to root")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "a", "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	t.Chdir(dir)

	g := testGrunt()
	stderr := g.proc.stderr.(*strings.Builder)
	if got := g.File.Expand(js.ObjectOf("strict", false), "a/locked/*"); len(got) != 0 {
		t.Errorf("Expand = %q, want no matches", got)
	}
	out := stderr.String()
	for _, want := range []string{"glob error [Error: EACCES: permission denied, scandir '", "/a/locked'] {\n", "  errno: -13,\n  code: 'EACCES',\n  syscall: 'scandir',\n  path: '"} {
		if !strings.Contains(out, want) {
			t.Errorf("stderr = %q, want it to contain %q", out, want)
		}
	}

	stderr.Reset()
	g.File.Expand(js.ObjectOf("strict", false, "silent", true), "a/locked/*")
	if stderr.Len() != 0 {
		t.Errorf("silent: stderr = %q", stderr.String())
	}

	defer func() {
		if err, ok := recover().(error); !ok || !strings.Contains(err.Error(), "EACCES: permission denied, scandir") {
			t.Errorf("strict: recovered %v, want the EACCES error", err)
		}
	}()
	g.File.Expand("a/locked/*")
}
