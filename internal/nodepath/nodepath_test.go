package nodepath

import "testing"

// Expected values printed by Node 24's path.posix, with /tmp as the working directory.

func TestRelative(t *testing.T) {
	for _, c := range []struct{ from, to, want string }{
		{"/a/b/c", "/a/b", ".."},
		{"/a/b", "/a/b/c/d", "c/d"},
		{"/a/b/c", "/x/y", "../../../x/y"},
		{"/", "/a", "a"},
		{"/a", "/", ".."},
		{"a/b", "a", ".."},
		{"/a/bc", "/a/b", "../b"},
		{"/a/b", "/a/bc", "../bc"},
		{"", "", ""},
		{"x", "../y", "../../y"},
	} {
		if got := Relative("/tmp", c.from, c.to); got != c.want {
			t.Errorf("Relative(%q, %q) = %q, want %q", c.from, c.to, got, c.want)
		}
	}
}

func TestDirnameBasename(t *testing.T) {
	for _, c := range []struct{ path, dir, base string }{
		{"", ".", ""}, {"/", "/", ""}, {"//", "/", ""}, {"a", ".", "a"}, {"a/", ".", "a"}, {"/a", "/", "a"},
		{"/a/b/", "/a", "b"}, {"a//b", "a/", "b"}, {"//a", "//", "a"}, {"../x/..", "../x", ".."},
	} {
		if got := Dirname(c.path); got != c.dir {
			t.Errorf("Dirname(%q) = %q, want %q", c.path, got, c.dir)
		}
		if got := Basename(c.path); got != c.base {
			t.Errorf("Basename(%q) = %q, want %q", c.path, got, c.base)
		}
	}
}

func TestJoinResolve(t *testing.T) {
	for _, c := range []struct {
		parts []string
		want  string
	}{{nil, "."}, {[]string{"a", "", "b/"}, "a/b/"}, {[]string{"a/..", ".."}, ".."}, {[]string{"/a", "../.."}, "/"}} {
		if got := Join(c.parts...); got != c.want {
			t.Errorf("Join(%q) = %q, want %q", c.parts, got, c.want)
		}
	}
	if got := Resolve("/tmp", "a/", "../b"); got != "/tmp/b" {
		t.Errorf("Resolve = %q", got)
	}
	if !IsAbsolute("/x") || IsAbsolute("x") {
		t.Error("IsAbsolute")
	}
}
