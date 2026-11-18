package nopt

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"testing"

	"github.com/gruntjs/grunt-go/js"
)

// testdata/cases.json is recorded from nopt 5.0.0 under Node 24 by testdata/gen.js.

type fixture struct {
	Cwd    string `json:"cwd"`
	Home   string `json:"home"`
	Tables []struct {
		Name    string               `json:"name"`
		Known   [][2]json.RawMessage `json:"known"`
		Aliases []json.RawMessage    `json:"aliases"`
	} `json:"tables"`
	Cases []struct {
		Table    string               `json:"table"`
		Slice    int                  `json:"slice"`
		Args     []string             `json:"args"`
		Options  [][2]json.RawMessage `json:"options"`
		Remain   []string             `json:"remain"`
		Cooked   []string             `json:"cooked"`
		Original []string             `json:"original"`
	} `json:"cases"`
}

type table struct {
	known   map[string]Type
	aliases map[string][]string
}

func mustUnmarshal(t *testing.T, raw []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatal(err)
	}
}

func decodeType(t *testing.T, raw json.RawMessage) Type {
	var name string
	if json.Unmarshal(raw, &name) == nil {
		return map[string]Type{"Boolean": Boolean, "String": String, "Number": Number, "path": Path, "Array": Array}[name]
	}
	var tagged struct {
		Lit  json.RawMessage   `json:"lit"`
		List []json.RawMessage `json:"list"`
	}
	mustUnmarshal(t, raw, &tagged)
	if tagged.List != nil {
		var ts []Type
		for _, x := range tagged.List {
			ts = append(ts, decodeType(t, x))
		}
		return OneOf(ts...)
	}
	return Literal(decodeValue(t, tagged.Lit))
}

func decodeValue(t *testing.T, raw json.RawMessage) any {
	var v any
	mustUnmarshal(t, raw, &v)
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	switch {
	case m["u"] != nil:
		return js.Undefined
	case m["n"] != nil:
		return js.StringToNumber(m["n"].(string))
	}
	var arr struct {
		A []json.RawMessage `json:"a"`
	}
	mustUnmarshal(t, raw, &arr)
	items := make([]any, len(arr.A))
	for i, x := range arr.A {
		items[i] = decodeValue(t, x)
	}
	return js.NewArray(items...)
}

// sameValue compares decoded values, with NaN equal to itself.
func sameValue(a, b any) bool {
	if x, ok := a.(float64); ok {
		y, ok := b.(float64)
		return ok && (x == y && math.Signbit(x) == math.Signbit(y) || x != x && y != y)
	}
	if x, ok := a.(*js.Array); ok {
		y, ok := b.(*js.Array)
		if !ok || len(x.Items) != len(y.Items) {
			return false
		}
		for i := range x.Items {
			if !sameValue(x.Items[i], y.Items[i]) {
				return false
			}
		}
		return true
	}
	return a == b
}

func TestParseFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	mustUnmarshal(t, data, &f)
	tables := map[string]table{}
	for _, tb := range f.Tables {
		tt := table{known: map[string]Type{}, aliases: map[string][]string{}}
		for _, kv := range tb.Known {
			var k string
			mustUnmarshal(t, kv[0], &k)
			tt.known[k] = decodeType(t, kv[1])
		}
		for _, raw := range tb.Aliases {
			var pair []json.RawMessage
			mustUnmarshal(t, raw, &pair)
			var k string
			var v []string
			mustUnmarshal(t, pair[0], &k)
			mustUnmarshal(t, pair[1], &v)
			tt.aliases[k] = v
		}
		tables[tb.Name] = tt
	}
	failed := 0
	for i, c := range f.Cases {
		tb := tables[c.Table]
		r := ParseIn(f.Cwd, f.Home, tb.known, tb.aliases, c.Args, c.Slice)
		ok := reflect.DeepEqual(r.Remain, nonNil(c.Remain)) &&
			reflect.DeepEqual(r.Cooked, nonNil(c.Cooked)) &&
			reflect.DeepEqual(r.Original, nonNil(c.Original))
		keys := r.Options.Keys()
		if len(keys) != len(c.Options) {
			ok = false
		} else {
			for j, kv := range c.Options {
				var k string
				mustUnmarshal(t, kv[0], &k)
				if keys[j] != k || !sameValue(r.Options.Get(k), decodeValue(t, kv[1])) {
					ok = false
				}
			}
		}
		if !ok {
			failed++
			if failed <= 15 {
				got := []any{}
				for _, k := range keys {
					got = append(got, k, r.Options.Get(k))
				}
				t.Errorf("case %d %s %q:\n got options %v remain %q cooked %q original %q\nwant options %s remain %q cooked %q original %q",
					i, c.Table, c.Args, got, r.Remain, r.Cooked, r.Original, c.Options, c.Remain, c.Cooked, c.Original)
			}
		}
	}
	t.Logf("nopt: %d/%d pass", len(f.Cases)-failed, len(f.Cases))
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func TestAbbrev(t *testing.T) {
	got := abbrev([]string{"verbose", "version", "help", "help"})
	want := map[string]string{
		"verbose": "verbose", "verbos": "verbose", "verbo": "verbose", "verb": "verbose",
		"version": "version", "versio": "version", "versi": "version", "vers": "version",
		"help": "help", "hel": "help", "he": "help", "h": "help",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("abbrev = %v, want %v", got, want)
	}
}

func TestParseResolvesAgainstProcess(t *testing.T) {
	t.Setenv("HOME", "/home/someone")
	r, err := Parse(map[string]Type{"base": Path}, nil, []string{"node", "grunt", "--base", "~/x"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Options.Get("base"); got != "/home/someone/x" {
		t.Errorf("base = %v, want /home/someone/x", got)
	}
}
