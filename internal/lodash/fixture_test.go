package lodash

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntjs/grunt-go/internal/fixture"
	"github.com/gruntjs/grunt-go/internal/jsregexp"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// tagged is a JavaScript value as the fixture recorder (fixlib.js) encodes it:
// u undefined, n null, b boolean, s string, d number, f named callback, a array,
// D date, r regexp, e error and o object (ordered key/value pairs).
type tagged struct {
	T string          `json:"t"`
	V json.RawMessage `json:"v"`
}

// fixtureCase is one recorded call _[Fn](...Args): its result or thrown error and
// the arguments after the call.
type fixtureCase struct {
	Fn    string   `json:"fn"`
	Args  []tagged `json:"args"`
	Out   *tagged  `json:"out"`
	Err   *string  `json:"err"`
	After []tagged `json:"after"`
}

// callbacks are the Go twins of fixlib.js's named callbacks.
var callbacks = map[string]*js.Function{
	"double": jsFn(func(a []any) (any, error) { return js.ToNumber(js.Arg(a, 0)) * 2, nil }),
	"isEven": jsFn(func(a []any) (any, error) { return math.Mod(js.ToNumber(js.Arg(a, 0)), 2) == 0, nil }),
	"gt2":    jsFn(func(a []any) (any, error) { return js.ToNumber(toPrimitiveNumber(js.Arg(a, 0))) > 2, nil }),
	"negate": jsFn(func(a []any) (any, error) { return -js.ToNumber(js.Arg(a, 0)), nil }),
	"floor":  jsFn(func(a []any) (any, error) { return math.Floor(js.ToNumber(js.Arg(a, 0))), nil }),
	"upper":  jsFn(func(a []any) (any, error) { return strings.ToUpper(js.ToString(js.Arg(a, 0))), nil }),
	"len":    jsFn(func(a []any) (any, error) { return float64(jsstr.Length(js.ToString(js.Arg(a, 0)))), nil }),
	"add":    jsFn(func(a []any) (any, error) { return js.Add(js.Arg(a, 0), js.Arg(a, 1), toPrimitive) }),
	"argstr": jsFn(func(a []any) (any, error) {
		parts := make([]string, len(a))
		for i, v := range a {
			parts[i] = js.ToString(v)
		}
		return strings.Join(parts, "|"), nil
	}),
	"keyOf":    jsFn(func(a []any) (any, error) { return js.Arg(a, 1), nil }),
	"isString": jsFn(func(a []any) (any, error) { _, ok := js.Arg(a, 0).(string); return ok, nil }),
	"truthy":   jsFn(func(a []any) (any, error) { return js.Arg(a, 0), nil }),
	"throws":   jsFn(func([]any) (any, error) { return nil, js.NewError("boom") }),
	"wrapUpper": jsFn(func(a []any) (any, error) {
		return "<" + strings.ToUpper(js.ToString(js.Arg(a, 0))) + ">", nil
	}),
}

func jsFn(call func([]any) (any, error)) *js.Function {
	return &js.Function{Call: func(_ any, args []any) (any, error) { return call(args) }}
}

func decodeTagged(t *testing.T, e tagged) any {
	t.Helper()
	switch e.T {
	case "u":
		return js.Undefined
	case "n":
		return nil
	case "b":
		var b bool
		mustDecode(t, e.V, &b)
		return b
	case "s":
		var s string
		mustDecode(t, e.V, &s)
		return s
	case "d", "D":
		var raw any
		mustDecode(t, e.V, &raw)
		f, ok := raw.(float64)
		if !ok {
			s := raw.(string)
			if s == "-0" {
				f = math.Copysign(0, -1)
			} else {
				var err error
				if f, err = strconv.ParseFloat(s, 64); err != nil {
					t.Fatal(err)
				}
			}
		}
		if e.T == "D" {
			return js.Date{Ms: f}
		}
		return f
	case "f":
		var name string
		mustDecode(t, e.V, &name)
		if f, ok := callbacks[name]; ok {
			return f
		}
		// A function the recorder could not name (a builtin, for instance) only
		// takes part in comparisons; a case that calls one has no Go twin to run.
		return &js.Function{Name: name, Call: func(any, []any) (any, error) {
			return nil, errors.New("fixture: the recorded function " + strconv.Quote(name) + " has no Go twin to call")
		}}
	case "a":
		var items []tagged
		mustDecode(t, e.V, &items)
		arr := &js.Array{Items: make([]any, len(items))}
		for i, it := range items {
			arr.Items[i] = decodeTagged(t, it)
		}
		return arr
	case "r":
		var parts [2]string
		mustDecode(t, e.V, &parts)
		return &js.RegExp{Source: parts[0], Flags: parts[1]}
	case "e":
		var parts [2]string
		mustDecode(t, e.V, &parts)
		return &js.Error{Name: parts[0], Message: parts[1]}
	case "o":
		var pairs [][2]json.RawMessage
		mustDecode(t, e.V, &pairs)
		o := js.NewObject()
		for _, p := range pairs {
			var k string
			var v tagged
			mustDecode(t, p[0], &k)
			mustDecode(t, p[1], &v)
			o.Set(k, decodeTagged(t, v))
		}
		return o
	}
	t.Fatalf("unknown tag %q", e.T)
	return nil
}

func mustDecode(t *testing.T, data []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(data, dst); err != nil {
		t.Fatal(err)
	}
}

// canonical renders a value for comparison: the same text for values the recorder
// would encode identically. Functions compare by callback name only.
func canonical(v any) string {
	switch x := v.(type) {
	case js.UndefinedType:
		return "undefined"
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(x)
	case string:
		// The fixtures' JSON decoder reads lone surrogates as U+FFFD; results keep
		// them in WTF-8, so compare the well-formed forms.
		return strconv.Quote(jsstr.ToWellFormed(x))
	case float64:
		if x == 0 && math.Signbit(x) {
			return "-0"
		}
		return js.NumberToString(x)
	case *js.Function:
		for name, f := range callbacks {
			if f == x {
				return "fn:" + name
			}
		}
		return "fn"
	case *js.Array:
		parts := make([]string, len(x.Items))
		for i, it := range x.Items {
			parts[i] = canonical(it)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case js.Date:
		return "Date(" + js.NumberToString(x.Ms) + ")"
	case *js.RegExp:
		return "/" + jsregexp.EscapeSource(x.Source) + "/" + x.Flags
	case *js.Error:
		return "Error(" + x.Name + ":" + x.Message + ")"
	case *js.Object:
		var parts []string
		for _, k := range x.Keys() {
			parts = append(parts, strconv.Quote(k)+":"+canonical(x.Get(k)))
		}
		return "{" + strings.Join(parts, ",") + "}"
	}
	return "?"
}

// runFixtures replays a recorded fixture file against Underscore().
func runFixtures(t *testing.T, path string) {
	t.Helper()
	var cases []fixtureCase
	if err := fixture.DecodeGzipJSON(path, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("empty fixture set")
	}
	u := Underscore()
	for i, c := range cases {
		f, ok := u.Get(c.Fn).(*js.Function)
		if !ok {
			t.Errorf("case %d: _.%s is missing", i, c.Fn)
			continue
		}
		args := make([]any, len(c.Args))
		shown := make([]string, len(c.Args))
		for j, a := range c.Args {
			args[j] = decodeTagged(t, a)
			shown[j] = canonical(args[j])
		}
		call := "_." + c.Fn + "(" + strings.Join(shown, ", ") + ")"
		got, err := f.Call(js.Undefined, args)
		if c.Err != nil {
			var jsErr *js.Error
			if !errors.As(err, &jsErr) || js.ToString(jsErr) != *c.Err {
				t.Errorf("case %d: %s error = %v (result %s), want %s", i, call, err, canonical(got), *c.Err)
			}
		} else if err != nil {
			t.Errorf("case %d: %s: unexpected error %v", i, call, err)
			continue
		} else if want := canonical(decodeTagged(t, *c.Out)); canonical(got) != want {
			t.Errorf("case %d: %s = %s, want %s", i, call, canonical(got), want)
		}
		for j, a := range c.After {
			if want := canonical(decodeTagged(t, a)); canonical(args[j]) != want {
				t.Errorf("case %d: %s left argument %d as %s, want %s", i, call, j, canonical(args[j]), want)
			}
		}
	}
}
