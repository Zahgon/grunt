package jsrt

import (
	"errors"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntjs/grunt-go/internal/fixture"
	"github.com/gruntjs/grunt-go/js"
)

// The fixtures under testdata/ (gzip-compressed JSON) were recorded with Node 24 and lodash 4.18.1 using
// grunt's delimiter settings. A file is {"d": defaultData, "cases": [...]}; each
// case is a template (t), optional data (d, JSON text with tags for values JSON
// cannot hold, defaulting to the file's), optional delimiters, and the rendered
// output (o) or the error as "Name: message" (e). Thrown non-Error values are
// recorded as "Thrown: String(value)". The template sees Object.create(data).

// lodashToString is lodash's baseToString, which _.escape applies first.
func lodashToString(r *Realm, v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case *js.Array:
		parts := make([]string, len(x.Items))
		for i, it := range x.Items {
			s, err := lodashToString(r, it)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return strings.Join(parts, ","), nil
	case float64:
		if x == 0 && math.Signbit(x) {
			return "-0", nil
		}
	}
	s, err := r.add(v, "")
	if err != nil {
		return "", err
	}
	return s.(string), nil
}

// testImports is the _ the fixtures were recorded with: {escape: _.escape}.
func testImports(r *Realm) *js.Object {
	escape := newNative("escape", 1, func(_ any, args []any) (any, error) {
		v := js.Arg(args, 0)
		if js.IsNullish(v) {
			return "", nil
		}
		s, err := lodashToString(r, v)
		if err != nil {
			return nil, err
		}
		return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;").Replace(s), nil
	})
	return js.ObjectOf("_", js.ObjectOf("escape", escape))
}

// revive turns the fixture data tags into values.
func revive(t *testing.T, r *Realm, v any) any {
	switch x := v.(type) {
	case *js.Array:
		for i, it := range x.Items {
			x.Items[i] = revive(t, r, it)
		}
	case *js.Object:
		switch {
		case x.Has("$undef"):
			return js.Undefined
		case x.Has("$date"):
			return js.Date{Ms: js.ToNumber(x.Get("$date"))}
		case x.Has("$nan"):
			return math.NaN()
		case x.Has("$inf"):
			return math.Inf(int(js.ToNumber(x.Get("$inf"))))
		case x.Has("$negzero"):
			return math.Copysign(0, -1)
		case x.Has("$re"):
			flags, _ := x.Get("flags").(string)
			re, err := r.newRegExp(x.Get("$re").(string), flags)
			if err != nil {
				t.Fatalf("fixture regexp: %v", err)
			}
			return re
		case x.Has("$fn"):
			var args []any
			if p, ok := x.Get("params").(*js.Array); ok {
				args = append(args, p.Items...)
			}
			f, err := r.functionConstructor(append(args, x.Get("$fn")))
			if err != nil {
				t.Fatalf("fixture function: %v", err)
			}
			return f
		}
		for _, k := range x.Keys() {
			x.Set(k, revive(t, r, x.Get(k)))
		}
	}
	return v
}

// errorText renders an error as the fixtures record it.
func errorText(r *Realm, err error) string {
	var th *Thrown
	if errors.As(err, &th) {
		s, serr := r.toString(th.Value)
		if serr != nil {
			s = "?"
		}
		return "Thrown: " + s
	}
	var e *js.Error
	if errors.As(err, &e) {
		return e.Name + ": " + e.Message
	}
	return "Go error: " + err.Error()
}

func str(o *js.Object, k string) (string, bool) {
	s, ok := o.Get(k).(string)
	return s, ok
}

// runCase renders one fixture case and returns the output or error text.
func runCase(t *testing.T, c *js.Object, defData string) (got string, isErr bool) {
	// The clock is fixed; the fixtures print nothing that depends on it.
	r := NewRealm(Options{Now: func() float64 { return 1700000000000 }})
	s := Delimiters("<%", "%>")
	if d, ok := c.Get("delims").(*js.Array); ok {
		s = Delimiters(d.Items[0].(string), d.Items[1].(string))
	}
	s.Realm = r
	s.Imports = testImports(r)
	proto := any(js.NewObject())
	d, ok := str(c, "d")
	if !ok {
		d, ok = defData, defData != ""
	}
	if ok {
		v, err := JSONParse(d)
		if err != nil {
			t.Fatalf("fixture data %q: %v", d, err)
		}
		proto = revive(t, r, v)
	}
	text, _ := str(c, "t")
	tmpl, err := CompileTemplate(text, s)
	if err != nil {
		return errorText(r, err), true
	}
	v, err := tmpl.Call(WithProto(nil, proto))
	if err != nil {
		return errorText(r, err), true
	}
	out, err := r.toString(v)
	if err != nil {
		return errorText(r, err), true
	}
	return out, false
}

func TestTemplateFixtures(t *testing.T) {
	files, err := filepath.Glob("testdata/*.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no fixtures")
	}
	for _, path := range files {
		t.Run(strings.TrimSuffix(filepath.Base(path), ".json.gz"), func(t *testing.T) {
			runFixtureFile(t, path)
		})
	}
}

// runFixtureFile replays one fixture file.
func runFixtureFile(t *testing.T, path string) {
	raw, err := fixture.ReadGzip(path)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := JSONParse(string(raw))
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	file := parsed.(*js.Object)
	defData, _ := str(file, "d")
	cases := file.Get("cases").(*js.Array).Items
	failed := 0
	for i, item := range cases {
		c := item.(*js.Object)
		want, wantErr := str(c, "e")
		if !wantErr {
			want, _ = str(c, "o")
		}
		got, gotErr := runCase(t, c, defData)
		if got != want || gotErr != wantErr {
			failed++
			if failed <= 40 {
				text, _ := str(c, "t")
				t.Errorf("%s #%d %q\n got: %q\nwant: %q", filepath.Base(path), i, text, got, want)
			}
		}
	}
	t.Logf("%s: %d/%d cases match", filepath.Base(path), len(cases)-failed, len(cases))
}
