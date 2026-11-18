package lodash

import (
	"testing"

	"github.com/gruntjs/grunt-go/js"
)

// TestGoAPI checks the exported functions grunt's own code calls; their `_`
// counterparts are covered by the recorded fixtures.
func TestGoAPI(t *testing.T) {
	dst := js.ObjectOf("a", js.ObjectOf("x", 1), "list", []any{1, 2, 3})
	Merge(dst, js.ObjectOf("a", js.ObjectOf("y", 2), "list", []any{9}, "u", js.Undefined), js.ObjectOf("__proto__", js.ObjectOf("polluted", true)))
	if got, want := canonical(dst), `{"a":{"x":1,"y":2},"list":[9,2,3],"u":undefined}`; got != want {
		t.Errorf("Merge = %s, want %s", got, want)
	}
	ext := Extend(js.ObjectOf("a", 1), nil, js.ObjectOf("b", 2), "xy")
	if got, want := canonical(ext), `{"0":"x","1":"y","a":1,"b":2}`; got != want {
		t.Errorf("Extend = %s, want %s", got, want)
	}
	def := Defaults(js.ObjectOf("a", js.Undefined, "b", nil), js.ObjectOf("a", 1, "b", 2, "c", 3))
	if got, want := canonical(def), `{"a":1,"b":null,"c":3}`; got != want {
		t.Errorf("Defaults = %s, want %s", got, want)
	}
	arrays := []struct {
		name string
		got  *js.Array
		want string
	}{
		{"Union", Union(js.NewArray(2.0, 1.0), js.NewArray(2.0, 3.0), "skipped"), "[2,1,3]"},
		{"Difference", Difference(js.NewArray(1.0, 2.0, 3.0), js.NewArray(2.0), js.NewArray(3.0)), "[1]"},
		{"Uniq", Uniq(js.NewArray(1.0, "1", 1.0)), `[1,"1"]`},
		{"Flatten", Flatten(js.NewArray(1.0, js.NewArray(2.0, js.NewArray(3.0)))), "[1,2,[3]]"},
		{"FlattenDeep", FlattenDeep(js.NewArray(1.0, js.NewArray(2.0, js.NewArray(3.0)))), "[1,2,3]"},
		{"ToArray", ToArray("a😀"), `["a","😀"]`},
	}
	for _, c := range arrays {
		if got := canonical(c.got); got != c.want {
			t.Errorf("%s = %s, want %s", c.name, got, c.want)
		}
	}
	calls := 0.0
	once := Once(&js.Function{Call: func(_ any, args []any) (any, error) { calls++; return calls, nil }})
	for range 3 {
		if v, err := once.Call(js.Undefined, nil); err != nil || v != 1.0 {
			t.Fatalf("Once call = %v, %v", v, err)
		}
	}
	orig := js.ObjectOf("k", []any{1})
	clone := Clone(orig).(*js.Object)
	if clone == orig || clone.Get("k") != orig.Get("k") {
		t.Error("Clone must copy the object but share its values")
	}
	if deep := CloneDeep(orig).(*js.Object); deep.Get("k") == orig.Get("k") || !IsEqual(deep, orig) {
		t.Error("CloneDeep must copy nested values")
	}
	if got, err := Pad("ab", 5.0, js.Undefined, js.Undefined); err != nil || got != "   ab" {
		t.Errorf("Pad = %q, %v", got, err)
	}
}
