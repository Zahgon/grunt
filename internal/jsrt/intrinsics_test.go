package jsrt

import (
	"testing"

	"github.com/gruntjs/grunt-go/js"
)

// TestInheritedBuiltins checks that what values inherit outside a template are the
// runtime's working builtins.
func TestInheritedBuiltins(t *testing.T) {
	call := func(v any, key string, args ...any) any {
		t.Helper()
		f, ok := GetProperty(v, key).(*js.Function)
		if !ok {
			t.Fatalf("%v[%q] is not a function", v, key)
		}
		out, err := f.Call(v, args)
		if err != nil {
			t.Fatalf("%v[%q](): %v", v, key, err)
		}
		return out
	}
	obj := js.ObjectOf("a", 1.0)
	if got := call(obj, "toString"); got != "[object Object]" {
		t.Errorf("({a: 1}).toString() = %v", got)
	}
	if got := call(obj, "hasOwnProperty", "a"); got != true {
		t.Errorf("hasOwnProperty('a') = %v", got)
	}
	if got := call("abc", "toUpperCase"); got != "ABC" {
		t.Errorf("'abc'.toUpperCase() = %v", got)
	}
	if got := call(js.NewArray("x", "y"), "join", "-"); got != "x-y" {
		t.Errorf("['x', 'y'].join('-') = %v", got)
	}
	ctor, ok := GetProperty(js.NewArray(), "constructor").(*js.Function)
	if !ok || ctor.Name != "Array" {
		t.Fatalf("[].constructor = %v", GetProperty(js.NewArray(), "constructor"))
	}
	if made, err := ctor.Call(js.Undefined, []any{"a", "b"}); err != nil || js.ToString(made) != "a,b" {
		t.Errorf("Array('a', 'b') = %v, %v", made, err)
	}
	if GetProperty(obj, "toString") != GetProperty(js.NewObject(), "toString") {
		t.Error("two objects inherit different toString functions")
	}
	if p, ok := GetProperty(js.NewArray(), "__proto__").(*js.Array); !ok || len(p.Items) != 0 {
		t.Errorf("[].__proto__ = %v", p)
	}
	for _, key := range []string{"constructor", "toString", "__proto__", "length"} {
		if !HasProperty("abc", key) {
			t.Errorf("%q in Object('abc') = false", key)
		}
	}
	if HasProperty(obj, "missing") || HasProperty(nil, "toString") || GetProperty(nil, "toString") != js.Undefined {
		t.Error("missing keys or nullish values report properties")
	}
}
