package yaml

import (
	"errors"
	"math"
	"testing"

	"github.com/gruntjs/grunt-go/js"
)

func TestSafeLoadValues(t *testing.T) {
	v, err := SafeLoad("name: grunt\nversion: 1.6\nlist: [a, 0x10, ~, true]\n2: two\nwhen: 2001-12-14\n")
	if err != nil {
		t.Fatal(err)
	}
	obj := v.(*js.Object)
	if got, want := obj.Keys(), []string{"2", "name", "version", "list", "when"}; !equalStrings(got, want) {
		t.Errorf("keys %q, want %q", got, want)
	}
	list := obj.Get("list").(*js.Array).Items
	if list[0] != "a" || list[1] != 16.0 || list[2] != nil || list[3] != true {
		t.Errorf("list %#v", list)
	}
	if got := obj.Get("when"); got != (js.Date{Ms: 1008288000000}) {
		t.Errorf("when %#v", got)
	}
}

func TestDateKeyIsStringified(t *testing.T) {
	v, err := SafeLoad("2001-01-01: x")
	if err != nil {
		t.Fatal(err)
	}
	if keys := v.(*js.Object).Keys(); keys[0] != "Mon Jan 01 2001 00:00:00 GMT+0000 (Coordinated Universal Time)" {
		t.Errorf("key %q", keys[0])
	}
}

func TestErrorMessage(t *testing.T) {
	_, err := SafeLoad("a: 1\na: 2\n")
	var e *js.Error
	if !errors.As(err, &e) {
		t.Fatalf("error %v", err)
	}
	const want = "duplicated mapping key at line 2, column 1:\n    a: 2\n    ^"
	if e.Name != "YAMLException" || e.Message != want {
		t.Errorf("got %s %q, want %q", e.Name, e.Message, want)
	}
	if e.Props.Get("reason") != "duplicated mapping key" {
		t.Errorf("reason %v", e.Props.Get("reason"))
	}
	if line := e.Props.Get("mark").(*js.Object).Get("line"); line != 1.0 {
		t.Errorf("mark line %v", line)
	}
}

func TestSchemas(t *testing.T) {
	// The mark falls after the line break js-yaml appends to the input.
	if _, err := SafeLoad("!!js/undefined"); err == nil || err.Error() != "unknown tag !<tag:yaml.org,2002:js/undefined> at line 2, column 1:\n    \n    ^" {
		t.Errorf("SafeLoad: %v", err)
	}
	v, err := Load("- !!js/undefined\n- !!js/regexp /a\\/b/mg\n- .nan")
	if err != nil {
		t.Fatal(err)
	}
	items := v.(*js.Array).Items
	if items[0] != js.Undefined {
		t.Errorf("undefined: %#v", items[0])
	}
	if re := items[1].(*js.RegExp); re.Source != `a\/b` || re.Flags != "gm" {
		t.Errorf("regexp: %#v", re)
	}
	if f := items[2].(float64); !math.IsNaN(f) {
		t.Errorf("nan: %v", f)
	}
}

func TestEmptyAndMultipleDocuments(t *testing.T) {
	if v, err := SafeLoad(""); err != nil || v != js.Undefined {
		t.Errorf("empty: %v %v", v, err)
	}
	if v, err := SafeLoad("# comment"); err != nil || v != nil {
		t.Errorf("comment only: %v %v", v, err)
	}
	if _, err := SafeLoad("a\n---\nb"); err == nil || err.Error() != "expected a single document in the stream, but found more" {
		t.Errorf("two documents: %v", err)
	}
}

func TestInvalidUTF8(t *testing.T) {
	// Node decodes the truncated sequence E2 82 as a single U+FFFD.
	v, err := SafeLoad("a\xe2\x82A")
	if err != nil || v != "a�A" {
		t.Errorf("got %q %v", v, err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
