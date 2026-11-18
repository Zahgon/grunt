package yaml

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/gruntjs/grunt-go/js"
)

// fixture is one line of testdata/fixtures.jsonl, recorded from js-yaml 3.15.2 on
// Node 24 by testdata/record.js.
type fixture struct {
	Name   string          `json:"name"`
	Schema string          `json:"schema"`
	Src    string          `json:"src"`
	Want   json.RawMessage `json:"want"`
}

// encode mirrors encode() in record.js: a JSON tree that keeps undefined, -0, NaN,
// dates, buffers, regular expressions, key order and shared references distinct.
func encode(v any) any {
	seen := map[any]int{}
	var enc func(v any) any
	enc = func(v any) any {
		switch x := v.(type) {
		case js.UndefinedType:
			return map[string]any{"u": 1}
		case nil, bool, string:
			return x
		case float64:
			if x == 0 && 1/x < 0 {
				return map[string]any{"n": "-0"}
			}
			return map[string]any{"n": js.NumberToString(x)}
		case []byte:
			return map[string]any{"b": hex.EncodeToString(x)}
		case js.Date:
			return map[string]any{"d": js.NumberToString(x.Ms)}
		case *js.RegExp:
			return map[string]any{"r": x.Source, "f": x.Flags}
		case *js.Array:
			if id, ok := seen[x]; ok {
				return map[string]any{"ref": id}
			}
			seen[x] = len(seen)
			items := make([]any, len(x.Items))
			for i, item := range x.Items {
				items[i] = enc(item)
			}
			return map[string]any{"a": items}
		case *js.Object:
			if id, ok := seen[x]; ok {
				return map[string]any{"ref": id}
			}
			seen[x] = len(seen)
			pairs := []any{}
			for _, k := range x.Keys() {
				pairs = append(pairs, []any{k, enc(x.Get(k))})
			}
			return map[string]any{"o": pairs}
		}
		return map[string]any{"unexpected": true}
	}
	return enc(v)
}

// outcome renders a load result as canonical JSON comparable with a fixture's want.
func outcome(t *testing.T, v any, err error) string {
	t.Helper()
	var tree any
	if err != nil {
		var jsErr *js.Error
		if !errors.As(err, &jsErr) {
			t.Fatalf("error %v is not a *js.Error", err)
		}
		tree = map[string]any{"error": map[string]any{"name": jsErr.Name, "message": jsErr.Message}}
	} else {
		tree = encode(v)
	}
	b, merr := json.Marshal(tree)
	if merr != nil {
		t.Fatal(merr)
	}
	return string(b)
}

// canonical re-marshals recorded JSON so that both sides compare byte for byte.
func canonical(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/fixtures.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	total, failed := 0, 0
	for sc.Scan() {
		var fx fixture
		if err := json.Unmarshal(sc.Bytes(), &fx); err != nil {
			t.Fatal(err)
		}
		total++
		load := SafeLoad
		if fx.Schema == "full" {
			load = Load
		}
		v, err := load(fx.Src)
		got, want := outcome(t, v, err), canonical(t, fx.Want)
		if want == `{"fn":1}` {
			// Node builds the function; the port reports !!js/function as unsupported.
			want = `{"error":{"message":"unsupported tag !\u003ctag:yaml.org,2002:js/function\u003e: constructing JavaScript functions is not supported","name":"YAMLException"}}`
		}
		if got != want {
			failed++
			if failed <= 30 {
				t.Errorf("%s (%s) src %q\n got %s\nwant %s", fx.Name, fx.Schema, fx.Src, got, want)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if failed > 0 {
		t.Errorf("%d of %d fixtures differ", failed, total)
	}
	t.Logf("%d fixtures", total)
}
