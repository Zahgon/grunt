package jsregex

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntjs/grunt-go/internal/fixture"
	"github.com/gruntjs/grunt-go/internal/jsregexp"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// wellFormed is how the fixtures' JSON decoder sees a string: lone surrogates,
// which the WTF-8 results keep, read as U+FFFD.
func wellFormed(s string) string { return jsstr.ToWellFormed(s) }

// fixtures are recorded from Node 24; null stands for undefined captures.
type fixtures struct {
	Cases []struct {
		Src, Flags string
		Err        string
		Results    [][]json.RawMessage
	}
	StrCases [][]json.RawMessage
}

func load(t *testing.T) fixtures {
	t.Helper()
	var f fixtures
	if err := fixture.DecodeGzipJSON("testdata/node24.json.gz", &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Cases) == 0 || len(f.StrCases) == 0 {
		t.Fatal("empty fixture set")
	}
	return f
}

func decode(t *testing.T, raw json.RawMessage, dst any) {
	t.Helper()
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatal(err)
	}
}

// toJSON is JSON.stringify for the values a replacement callback receives, with
// undefined items written as null as the recorder does.
func toJSON(v any) string {
	switch x := v.(type) {
	case string:
		return quoteJSON(x)
	case float64:
		return js.NumberToString(x)
	case js.UndefinedType, nil:
		return "null"
	case *js.Object:
		var parts []string
		for _, k := range x.Keys() {
			if _, undef := x.Get(k).(js.UndefinedType); undef {
				continue
			}
			parts = append(parts, toJSON(k)+":"+toJSON(x.Get(k)))
		}
		return "{" + strings.Join(parts, ",") + "}"
	}
	panic("unexpected value")
}

// nullable maps captures to the fixtures' form: nil for undefined, well-formed text.
func nullable(items []any) []*string {
	out := make([]*string, len(items))
	for i, it := range items {
		if s, ok := it.(string); ok {
			s = wellFormed(s)
			out[i] = &s
		}
	}
	return out
}

func wellFormedAll(items []string) []string {
	if items == nil {
		return nil
	}
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = wellFormed(s)
	}
	return out
}

func TestNode24(t *testing.T) {
	f := load(t)
	recorder := &js.Function{Call: func(_ any, args []any) (any, error) {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = toJSON(a)
		}
		return "[" + strings.Join(parts, ",") + "]", nil
	}}
	for _, c := range f.Cases {
		re, err := Compile(c.Src, c.Flags)
		if c.Err != "" {
			var jsErr *js.Error
			if !errors.As(err, &jsErr) || js.ToString(jsErr) != c.Err {
				t.Errorf("Compile(%q, %q) error = %v, want %s", c.Src, c.Flags, err, c.Err)
			}
			continue
		}
		if err != nil {
			t.Errorf("Compile(%q, %q): %v", c.Src, c.Flags, err)
			continue
		}
		nonGlobal, err := Compile(c.Src, strings.ReplaceAll(c.Flags, "g", ""))
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range c.Results {
			var s string
			decode(t, r[0], &s)
			var exec []json.RawMessage
			decode(t, r[1], &exec)
			units := jsstr.ToUTF16(s)
			caps := nonGlobal.Exec(units, 0)
			if exec == nil {
				if caps != nil {
					t.Errorf("/%s/%s.exec(%q) matched %v, want null", c.Src, c.Flags, s, caps)
				}
			} else {
				var index int
				decode(t, exec[0], &index)
				var want []*string
				decode(t, json.RawMessage("["+string(joinRaw(exec[1:]))+"]"), &want)
				if caps == nil || caps[0] != index || !reflect.DeepEqual(nullable(jsregexp.Captures(units, caps)), want) {
					t.Errorf("/%s/%s.exec(%q) = %v, want index %d %v", c.Src, c.Flags, s, caps, index, deref(want))
				}
			}
			var wantRep, wantFn string
			decode(t, r[2], &wantRep)
			decode(t, r[3], &wantFn)
			if got, err := re.Replace(s, "[$&|$1|$2|$$|$`|$'|$<y>|$01|$10|$0]"); err != nil || wellFormed(got) != wantRep {
				t.Errorf("%q.replace(/%s/%s, template) = %q, want %q", s, c.Src, c.Flags, got, wantRep)
			}
			if got, err := re.Replace(s, recorder); err != nil || wellFormed(got) != wantFn {
				t.Errorf("%q.replace(/%s/%s, fn) = %q, want %q", s, c.Src, c.Flags, got, wantFn)
			}
			for i, limit := range []uint32{math.MaxUint32, 2} {
				var want []*string
				decode(t, r[4+i], &want)
				if got := nullable(re.Split(s, limit)); !reflect.DeepEqual(got, want) {
					t.Errorf("%q.split(/%s/%s, %d) = %v, want %v", s, c.Src, c.Flags, limit, deref(got), deref(want))
				}
			}
			var wantAll []string
			decode(t, r[6], &wantAll)
			if re.Global() {
				if got := wellFormedAll(re.MatchAll(s)); !reflect.DeepEqual(got, wantAll) {
					t.Errorf("%q.match(/%s/%s) = %q, want %q", s, c.Src, c.Flags, got, wantAll)
				}
			}
		}
	}
	for _, c := range f.StrCases {
		var s, sep, rep string
		var all, one []string
		decode(t, c[0], &s)
		decode(t, c[1], &sep)
		decode(t, c[2], &all)
		decode(t, c[3], &one)
		decode(t, c[4], &rep)
		if got := wellFormedAll(jsstr.Split(s, sep, math.MaxUint32)); !reflect.DeepEqual(got, all) {
			t.Errorf("%q.split(%q) = %q, want %q", s, sep, got, all)
		}
		if got := wellFormedAll(jsstr.Split(s, sep, 1)); !reflect.DeepEqual(got, one) {
			t.Errorf("%q.split(%q, 1) = %q, want %q", s, sep, got, one)
		}
		if got, err := ReplaceString(s, sep, "<$&$1$$>"); err != nil || got != rep {
			t.Errorf("%q.replace(%q) = %q, want %q", s, sep, got, rep)
		}
	}
}

func joinRaw(items []json.RawMessage) []byte {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = string(it)
	}
	return []byte(strings.Join(parts, ","))
}

func deref(items []*string) []string {
	out := make([]string, len(items))
	for i, it := range items {
		if it == nil {
			out[i] = "<undefined>"
		} else {
			out[i] = strconv.Quote(*it)
		}
	}
	return out
}

// quoteJSON is JSON.stringify for a string, which escapes lone surrogates.
func quoteJSON(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	u := jsstr.ToUTF16(s)
	for i := 0; i < len(u); i++ {
		c := u[i]
		switch {
		case c == '"':
			b.WriteString(`\"`)
		case c == '\\':
			b.WriteString(`\\`)
		case c == '\b':
			b.WriteString(`\b`)
		case c == '\f':
			b.WriteString(`\f`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c < 0x20:
			b.WriteString(fmt.Sprintf(`\u%04x`, c))
		case c >= 0xd800 && c < 0xdc00 && i+1 < len(u) && u[i+1] >= 0xdc00 && u[i+1] <= 0xdfff:
			b.WriteString(jsstr.FromUTF16(u[i : i+2]))
			i++
		case c >= 0xd800 && c <= 0xdfff:
			b.WriteString(fmt.Sprintf(`\u%04x`, c))
		default:
			b.WriteString(jsstr.FromUTF16(u[i : i+1]))
		}
	}
	b.WriteByte('"')
	return b.String()
}
