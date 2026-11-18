package grunt

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gruntjs/grunt-go/js"
)

// Expected values in this file were printed by Node 24 with grunt 1.6.3's
// dependencies (grunt-legacy-util 2.0.2, getobject 1.0.2, lib/util/task.js).

func testGrunt() *Grunt {
	return newGrunt(&process{
		stdout: newSyncWriter(&strings.Builder{}),
		stderr: &strings.Builder{},
		exit:   func(code int) { panic(exitSignal{code}) },
		getenv: func(string) (string, bool) { return "", false },
		now:    func() float64 { return 0 },
	})
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	s, ok := JSON(v).(string)
	if !ok {
		t.Fatalf("JSON(%v) is undefined", v)
	}
	return s
}

func TestSplitArgs(t *testing.T) {
	g := testGrunt()
	for in, want := range map[string][]string{
		"a:b":       {"a", "b"},
		`a\:b:c`:    {"a:b", "c"},
		`x\\:y`:     {`x\`, "y"},
		"::":        {"", "", ""},
		"p:":        {"p", ""},
		"":          {},
		`a\\\:b`:    {`a\:b`},
		"no-colons": {"no-colons"},
	} {
		if got := g.Task.SplitArgs(in); !reflect.DeepEqual(got, want) {
			t.Errorf("SplitArgs(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTaskPlusArgs(t *testing.T) {
	g := testGrunt()
	noop := TaskFunc(func(*Task) any { return nil })
	g.RegisterTask("foo", noop)
	g.RegisterTask("foo:bar", noop)
	for _, c := range []struct {
		name, task string
		args       []string
	}{
		{"foo", "foo", []string{}},
		{"foo:bar", "foo:bar", []string{}},
		{"foo:bar:baz", "foo:bar", []string{"baz"}},
		{"foo:baz:bar", "foo", []string{"baz", "bar"}},
		{"nope:x", "", []string{"nope", "x"}},
	} {
		it := g.Task.taskPlusArgs(c.name)
		name := ""
		if it.task != nil {
			name = it.task.name
		}
		if name != c.task || !reflect.DeepEqual(it.args, c.args) {
			t.Errorf("taskPlusArgs(%q) = %q %q, want %q %q", c.name, name, it.args, c.task, c.args)
		}
	}
}

func TestRegisterInfo(t *testing.T) {
	g := testGrunt()
	noop := TaskFunc(func(*Task) any { return nil })
	g.RegisterTask("plain", noop)
	g.RegisterTask("described", "Does things.", noop)
	g.RegisterTask("one", "plain")
	g.RegisterTask("many", []string{"plain", "one"})
	g.RegisterMultiTask("multi", noop)
	g.RegisterMultiTask("multi-empty", "", noop)
	g.RegisterInitTask("init", noop)
	for name, want := range map[string]string{
		"plain":       "Custom task.",
		"described":   "Does things.",
		"one":         `Alias for "plain" task.`,
		"many":        `Alias for "plain", "one" tasks.`,
		"multi":       "Custom multi task.",
		"multi-empty": "Custom task.",
		"init":        "Custom task.",
	} {
		if got := g.Task.lookup(name).info; got != want {
			t.Errorf("info of %s = %q, want %q", name, got, want)
		}
	}
	if !g.Task.lookup("init").init || !g.Task.lookup("multi").multi {
		t.Error("init or multi flag not set")
	}
	g.RenameTask("plain", "renamed")
	if g.Task.Exists("plain") || !g.Task.Exists("renamed") || g.Task.lookup("renamed").name != "renamed" {
		t.Error("RenameTask did not move the task")
	}
	if keys := g.Task.tasks.Keys(); keys[len(keys)-1] != "renamed" {
		t.Errorf("renamed task is not last: %q", keys)
	}
}

func TestPluralizeRepeatKindOf(t *testing.T) {
	g := testGrunt()
	for _, c := range []struct {
		n    any
		s    string
		sep  []string
		want string
	}{
		{0, "y/ies", nil, "ies"}, {1, "y/ies", nil, "y"}, {2, "one|many", []string{"|"}, "many"},
		{"1", "a/b", nil, "b"}, {1, "x", nil, "x"}, {2, "x", nil, ""}, {1, "/", []string{""}, ""}, {3, "a/b/c", nil, "b"},
	} {
		if got := g.Util.Pluralize(c.n, c.s, c.sep...); got != c.want {
			t.Errorf("Pluralize(%v, %q) = %q, want %q", c.n, c.s, got, c.want)
		}
	}
	for _, c := range []struct {
		n    int
		s    []string
		want string
	}{{3, []string{"ab"}, "ababab"}, {2, []string{""}, "  "}, {0, []string{"x"}, ""}, {2, nil, "  "}} {
		if got := g.Util.Repeat(c.n, c.s...); got != c.want {
			t.Errorf("Repeat(%d, %q) = %q, want %q", c.n, c.s, got, c.want)
		}
	}
	values := []any{nil, js.Undefined, 1, "x", true, A(), O(), js.Date{}, &js.RegExp{Source: "x"}, js.NewError("e"),
		Fn(func(...any) any { return nil }), []byte("a")}
	want := []string{"null", "undefined", "number", "string", "boolean", "array", "object", "date", "regexp", "error",
		"function", "object"}
	for i, v := range values {
		if got := g.Util.KindOf(v); got != want[i] {
			t.Errorf("KindOf(%#v) = %q, want %q", v, got, want[i])
		}
	}
	if got := g.Util.NormalizeLF("a\r\nb\nc\r"); got != "a\nb\nc\r" {
		t.Errorf("NormalizeLF = %q", got)
	}
}

func TestNamespace(t *testing.T) {
	g := testGrunt()
	obj := O("a", O("b", O("c", 1)), "x.y", 2, "arr", A(1, O("z", 3)))
	gets := map[string]string{"a.b.c": "1", "a.b": `{"c":1}`, `x\.y`: "2", "arr.1.z": "3", "arr.length": "2"}
	for path, want := range gets {
		if got := jsonOf(t, g.Util.NamespaceGet(obj, path)); got != want {
			t.Errorf("NamespaceGet(%q) = %s, want %s", path, got, want)
		}
	}
	if got := g.Util.NamespaceGet(obj, "nope.x"); got != js.Undefined {
		t.Errorf("NamespaceGet(nope.x) = %v", got)
	}
	for path, want := range map[string]bool{"a.b": true, "a.q": false, `x\.y`: true, "arr.0": true, "arr.9": false} {
		if got := g.Util.NamespaceExists(obj, path); got != want {
			t.Errorf("NamespaceExists(%q) = %v, want %v", path, got, want)
		}
	}
	for _, c := range []struct {
		path  string
		value any
		set   bool
	}{{"a.n", 5, true}, {"new.deep.key", "v", true}, {"__proto__.p", 1, false}, {"arr.4", "x", true}, {"a.b.c.d", 1, false}} {
		got := g.Util.NamespaceSet(obj, c.path, c.value)
		if (got != js.Undefined) != c.set {
			t.Errorf("NamespaceSet(%q) = %v, want set=%v", c.path, got, c.set)
		}
	}
	want := `{"a":{"b":{"c":1},"n":5},"x.y":2,"arr":[1,{"z":3},null,null,"x"],"new":{"deep":{"key":"v"}}}`
	if got := jsonOf(t, obj); got != want {
		t.Errorf("object after NamespaceSet = %s, want %s", got, want)
	}
}

func TestRecurse(t *testing.T) {
	g := testGrunt()
	upper := func(v any) any {
		if s, ok := v.(string); ok {
			return strings.ToUpper(s)
		}
		return v
	}
	got, err := g.Util.Recurse(O("a", A(1, "x", O("b c", nil)), "d", "y"), upper)
	if err != nil {
		t.Fatal(err)
	}
	if s := jsonOf(t, got); s != `{"a":[1,"X",{"b c":null}],"d":"Y"}` {
		t.Errorf("Recurse = %s", s)
	}
	inner := O("b", A(1))
	cyclic := O("a", inner)
	inner.Get("b").(*js.Array).Items = append(inner.Get("b").(*js.Array).Items, inner)
	if _, err := g.Util.Recurse(cyclic, upper); err == nil || err.Error() != "Circular reference detected (.a.b[1])" {
		t.Errorf("Recurse of a cycle: %v", err)
	}
}

func TestOptionFlags(t *testing.T) {
	g := testGrunt()
	g.OptionInit(O("verbose", true, "write", false, "tasks", A(), "npm", A("a", "b"), "debug", 2, "base", "/x"))
	want := []string{"--verbose", "--no-write", "--npm=a,b", "--debug=2", "--base=/x"}
	if got := g.OptionFlags(); !reflect.DeepEqual(got, want) {
		t.Errorf("OptionFlags = %q, want %q", got, want)
	}
	if g.Option("no-write") != true || g.Option("no-verbose") != false || g.Option("no-missing") != false {
		t.Error("no- options")
	}
	if g.Option("x", 3) != 3.0 || g.Option("x") != 3.0 {
		t.Error("Option set/get")
	}
}

func TestReplaceExt(t *testing.T) {
	for _, c := range []struct{ path, extDot, ext, want string }{
		{"a/b.c.js", "first", ".min.js", "a/b.min.js"},
		{"a/b.c.js", "last", ".x", "a/b.c.x"},
		{"a.d/b", "first", ".x", "a.d/b.x"},
		{"a/.hidden", "first", "", "a/"},
		{"a/b.js", "first", "$&-$$", "a/b.js-$"},
		{"undefined/b.js", "other", "X", "X/b.js"},
		{"x/undefined.js", "middle", "[$1|$&|$`]", "x/[$1|undefined|x/].js"},
		{"a/b.min.js", "first", "[$1]", "a/b[.min.js]"},
		{"a/b", "last", "[$1|$']", "a/b[|]"},
		{"é/b.ü.js", "last", "[$&]", "é/b.ü[.js]"},
	} {
		if got := replaceExt(c.path, c.extDot, c.ext); got != c.want {
			t.Errorf("replaceExt(%q, %q, %q) = %q, want %q", c.path, c.extDot, c.ext, got, c.want)
		}
	}
}
