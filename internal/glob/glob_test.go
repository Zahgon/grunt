//go:build unix

package glob

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// The fixtures under testdata were recorded from glob 7.1.7 on Node 24 by
// testdata/gen/glob.js.

func loadFixture(t *testing.T, name string, v any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}

func TestCollationFixtures(t *testing.T) {
	var fixture struct {
		Pairs [][3]json.RawMessage `json:"pairs"`
		Lists [][]string           `json:"lists"`
	}
	loadFixture(t, "collation.json", &fixture)
	table := loadCollation()
	for _, p := range fixture.Pairs {
		var a, b string
		var want int
		if json.Unmarshal(p[0], &a) != nil || json.Unmarshal(p[1], &b) != nil || json.Unmarshal(p[2], &want) != nil {
			t.Fatalf("bad pair %s", p)
		}
		if got := compareKeys(table.collationKey(a), table.collationKey(b)); got != want {
			t.Errorf("%q.localeCompare(%q) = %d, want %d", a, b, got, want)
		}
	}
	for _, list := range fixture.Lists {
		got := slices.Clone(list)
		slices.Reverse(got)
		sortLocale(got)
		for i := 1; i < len(got); i++ {
			if compareKeys(table.collationKey(got[i-1]), table.collationKey(got[i])) > 0 {
				t.Errorf("sorted out of order: %q before %q", got[i-1], got[i])
			}
		}
		if !slices.EqualFunc(got, list, func(a, b string) bool {
			return compareKeys(table.collationKey(a), table.collationKey(b)) == 0
		}) {
			t.Errorf("sortLocale = %q, want %q", got, list)
		}
	}
}

func TestKeySetOrder(t *testing.T) {
	s := newKeySet()
	for _, k := range []string{"b", "10", "a", "2", "01", "4294967295", "4294967294", "0", "b"} {
		s.add(k)
	}
	want := []string{"0", "2", "10", "4294967294", "b", "a", "01", "4294967295"}
	if got := s.keys(); !slices.Equal(got, want) {
		t.Errorf("keys() = %q, want %q", got, want)
	}
}

type treeEntry struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Target string `json:"target"`
	Mode   string `json:"mode"`
}

type globCase struct {
	Pattern       string         `json:"pattern"`
	Options       map[string]any `json:"options"`
	Result        []string       `json:"result"`
	Error         string         `json:"error"`
	Warnings      []string       `json:"warnings"`
	CaseSensitive bool           `json:"caseSensitive"`
	NonRoot       bool           `json:"nonRoot"`
}

func createTree(t *testing.T, root string, entries []treeEntry) {
	t.Helper()
	for _, e := range entries {
		p := filepath.Join(root, e.Path)
		var err error
		switch e.Type {
		case "dir":
			err = os.Mkdir(p, 0o755)
		case "file":
			err = os.WriteFile(p, nil, 0o644)
		case "fifo":
			err = syscall.Mkfifo(p, 0o644)
		case "symlink":
			err = os.Symlink(e.Target, p)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range entries {
		if e.Mode == "" {
			continue
		}
		mode, err := strconv.ParseUint(e.Mode, 8, 32)
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(root, e.Path)
		if err := os.Chmod(p, os.FileMode(mode)); err != nil {
			t.Fatal(err)
		}
		// Let t.TempDir's cleanup remove it.
		t.Cleanup(func() {
			if err := os.Chmod(p, 0o755); err != nil {
				t.Error(err)
			}
		})
	}
}

// caseInsensitiveFS reports whether dir's file system folds case.
func caseInsensitiveFS(t *testing.T, dir string) bool {
	p := filepath.Join(dir, "CaseProbe")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := os.Stat(filepath.Join(dir, "caseprobe"))
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	return err == nil
}

// fixtureOptions maps the JavaScript options of a fixture, with @ROOT@ in
// strings replaced by root.
func fixtureOptions(t *testing.T, o map[string]any, root string) Options {
	t.Helper()
	var opts Options
	flags := map[string]*bool{
		"dot": &opts.Dot, "nomount": &opts.NoMount, "mark": &opts.Mark, "nosort": &opts.NoSort,
		"stat": &opts.Stat, "silent": &opts.Silent, "nounique": &opts.NoUnique, "nonull": &opts.NoNull,
		"nodir": &opts.NoDir, "follow": &opts.Follow, "realpath": &opts.Realpath,
		"absolute": &opts.Absolute, "matchBase": &opts.MatchBase, "nocase": &opts.NoCase,
		"noext": &opts.NoExt, "nobrace": &opts.NoBrace, "noglobstar": &opts.NoGlobstar,
	}
	place := func(s string) string { return strings.ReplaceAll(s, "@ROOT@", root) }
	for k, v := range o {
		switch k {
		case "cwd":
			opts.Cwd = place(v.(string))
		case "root":
			opts.Root = place(v.(string))
		case "strict":
			opts.NoStrict = v == false
		case "maxDepth":
			// Not a glob 7 option.
		case "maxLength":
			opts.MaxLength = int(v.(float64))
		case "ignore":
			switch v := v.(type) {
			case string:
				opts.Ignore = []string{place(v)}
			case []any:
				for _, p := range v {
					opts.Ignore = append(opts.Ignore, place(p.(string)))
				}
			}
		default:
			f, ok := flags[k]
			if !ok {
				t.Fatalf("unknown option %q", k)
			}
			*f = v == true
		}
	}
	return opts
}

func TestGlobFixtures(t *testing.T) {
	var trees []struct {
		Tree  []treeEntry `json:"tree"`
		Cases []globCase  `json:"cases"`
	}
	loadFixture(t, "glob.json", &trees)
	isRoot := os.Geteuid() == 0
	for i, tree := range trees {
		t.Run(fmt.Sprint("tree", i), func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			foldsCase := caseInsensitiveFS(t, root)
			createTree(t, root, tree.Tree)
			t.Chdir(root)
			for _, c := range tree.Cases {
				if c.CaseSensitive && foldsCase || c.NonRoot && isRoot {
					continue
				}
				runGlobCase(t, root, c)
			}
		})
	}
}

// unplace writes root as the generator does (testdata/gen/glob.js).
func unplace(s, root string) string {
	top := root + "/" + strings.Split(root, "/")[1] + "'"
	s = strings.ReplaceAll(s, root+root, "@ROOT@@ROOT@")
	s = strings.ReplaceAll(s, top, "@ROOT@/@TOP@'")
	return strings.ReplaceAll(s, root, "@ROOT@")
}

const eaccesScandir = "EACCES: permission denied, scandir '"

// sameError compares error messages. In a tree with an unreadable directory,
// which path reaches it first depends on the directory order, so any two
// EACCES failures of readdir are taken as the same.
func sameError(got, want string, nonRoot bool) bool {
	if nonRoot && strings.HasPrefix(got, eaccesScandir) && strings.HasPrefix(want, eaccesScandir) {
		return true
	}
	return got == want
}

func runGlobCase(t *testing.T, root string, c globCase) {
	t.Helper()
	opts := fixtureOptions(t, c.Options, root)
	var warnings []string
	opts.OnError = func(err error) {
		warnings = append(warnings, unplace(err.Error(), root))
	}
	got, err := Sync(strings.ReplaceAll(c.Pattern, "@ROOT@", root), opts)
	desc := fmt.Sprintf("Sync(%q, %v)", c.Pattern, c.Options)
	if c.Error != "" {
		if err == nil {
			t.Errorf("%s = %q, want error %q", desc, got, c.Error)
		} else if msg := unplace(err.Error(), root); !sameError(msg, c.Error, c.NonRoot) {
			t.Errorf("%s: error %q, want %q", desc, msg, c.Error)
		}
		return
	}
	if err != nil {
		t.Errorf("%s: %v", desc, err)
		return
	}
	for i := range got {
		got[i] = unplace(got[i], root)
	}
	want := slices.Clone(c.Result)
	if c.Options["nosort"] == true {
		// Unsorted results follow the directory order, which differs between
		// file systems.
		slices.Sort(got)
		slices.Sort(want)
	}
	if !slices.Equal(got, want) {
		t.Errorf("%s = %q, want %q", desc, got, want)
	}
	if c.NonRoot {
		// Reported in directory order.
		slices.Sort(warnings)
		slices.Sort(c.Warnings)
	}
	if !slices.Equal(warnings, c.Warnings) {
		t.Errorf("%s: warnings %q, want %q", desc, warnings, c.Warnings)
	}
}
