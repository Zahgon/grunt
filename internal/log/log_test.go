package log

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gruntjs/grunt-go/js"
)

// The fixtures in testdata/cases.json are recorded from grunt-legacy-log 3.0.1 and
// util.format under Node 24 by testdata/gen.js.

type fixtures struct {
	Format []struct {
		Args []json.RawMessage `json:"args"`
		Out  string            `json:"out"`
	} `json:"format"`
	Markup []struct {
		In    string `json:"in"`
		Color bool   `json:"color"`
		Out   string `json:"out"`
	} `json:"markup"`
	Wrap []struct {
		Width int    `json:"width"`
		Text  string `json:"text"`
		Out   string `json:"out"`
	} `json:"wrap"`
	Table []struct {
		Widths []int    `json:"widths"`
		Texts  []string `json:"texts"`
		Out    string   `json:"out"`
	} `json:"table"`
	Log []logCase `json:"log"`
}

type logCase struct {
	Grunt bool `json:"grunt"`
	Color bool `json:"color"`
	Opts  struct {
		Verbose bool            `json:"verbose"`
		Debug   json.RawMessage `json:"debug"`
		Color   bool            `json:"color"`
		MaxCols int             `json:"maxCols"`
	} `json:"opts"`
	Ops []struct {
		Muted  *bool             `json:"muted"`
		Set    string            `json:"set"`
		Val    bool              `json:"val"`
		T      string            `json:"t"`
		M      string            `json:"m"`
		A      []json.RawMessage `json:"a"`
		Prefix string            `json:"prefix"`
		Throws string            `json:"throws"`
		Widths []int             `json:"widths"`
		Texts  []string          `json:"texts"`
		Ret    string            `json:"ret"`
	} `json:"ops"`
	Out        string `json:"out"`
	ErrorCount int    `json:"errorcount"`
	HasLogged  bool   `json:"hasLogged"`
}

func loadFixtures(t *testing.T) *fixtures {
	t.Helper()
	data, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixtures
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	return &f
}

// decoder rebuilds values encoded by gen.js's encode.
type decoder struct {
	t    *testing.T
	refs map[int]any
}

func decodeValue(t *testing.T, raw json.RawMessage) any {
	d := &decoder{t: t, refs: map[int]any{}}
	return d.decode(raw)
}

func (d *decoder) decode(raw json.RawMessage) any {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		d.t.Fatal(err)
	}
	switch x := v.(type) {
	case nil, bool, string:
		return x
	case map[string]any:
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			d.t.Fatal(err)
		}
		return d.decodeTagged(x, m)
	}
	d.t.Fatalf("unexpected encoded value %s", raw)
	return nil
}

func (d *decoder) id(m map[string]json.RawMessage, v any) {
	if raw, ok := m["id"]; ok {
		var id int
		if err := json.Unmarshal(raw, &id); err != nil {
			d.t.Fatal(err)
		}
		d.refs[id] = v
	}
}

func (d *decoder) pairs(raw json.RawMessage) *js.Object {
	var pairs [][2]json.RawMessage
	if err := json.Unmarshal(raw, &pairs); err != nil {
		d.t.Fatal(err)
	}
	o := js.NewObject()
	for _, p := range pairs {
		var k string
		if err := json.Unmarshal(p[0], &k); err != nil {
			d.t.Fatal(err)
		}
		o.Set(k, d.decode(p[1]))
	}
	return o
}

func (d *decoder) decodeTagged(x map[string]any, m map[string]json.RawMessage) any {
	switch {
	case x["u"] != nil:
		return js.Undefined
	case x["n"] != nil:
		return js.StringToNumber(x["n"].(string))
	case x["fn"] != nil:
		return &js.Function{Name: x["fn"].(string)}
	case x["b"] != nil:
		b, err := hex.DecodeString(x["b"].(string))
		if err != nil {
			d.t.Fatal(err)
		}
		return b
	case x["d"] != nil:
		return js.Date{Ms: js.StringToNumber(x["d"].(string))}
	case x["r"] != nil:
		return &js.RegExp{Source: x["r"].(string), Flags: x["f"].(string)}
	case x["ref"] != nil:
		return d.refs[int(x["ref"].(float64))]
	case x["a"] != nil:
		a := js.NewArray()
		d.id(m, a)
		var items []json.RawMessage
		if err := json.Unmarshal(m["a"], &items); err != nil {
			d.t.Fatal(err)
		}
		for _, it := range items {
			a.Items = append(a.Items, d.decode(it))
		}
		return a
	case x["o"] != nil:
		o := js.NewObject()
		d.id(m, o)
		// Decode in place so references to o resolve while its members decode.
		var pairs [][2]json.RawMessage
		if err := json.Unmarshal(m["o"], &pairs); err != nil {
			d.t.Fatal(err)
		}
		for _, p := range pairs {
			var k string
			if err := json.Unmarshal(p[0], &k); err != nil {
				d.t.Fatal(err)
			}
			o.Set(k, d.decode(p[1]))
		}
		return o
	case x["e"] != nil:
		e := &js.Error{Name: x["e"].(string), Message: x["m"].(string)}
		d.id(m, e)
		if p := d.pairs(m["p"]); p.Len() > 0 {
			e.Props = p
		}
		return e
	}
	d.t.Fatalf("unexpected tagged value %v", x)
	return nil
}

func TestFormat(t *testing.T) {
	f := loadFixtures(t)
	failed := 0
	for i, c := range f.Format {
		args := make([]any, len(c.Args))
		d := &decoder{t: t, refs: map[int]any{}}
		for j, a := range c.Args {
			args[j] = d.decode(a)
		}
		if got := Format(args...); got != c.Out {
			failed++
			if failed <= 10 {
				t.Errorf("format case %d %s:\n got %q\nwant %q", i, c.Args, got, c.Out)
			}
		}
	}
	t.Logf("format: %d/%d pass", len(f.Format)-failed, len(f.Format))
}

func TestMarkup(t *testing.T) {
	f := loadFixtures(t)
	for i, c := range f.Markup {
		if got := markup(c.In, c.Color); got != c.Out {
			t.Errorf("markup case %d %q color=%v:\n got %q\nwant %q", i, c.In, c.Color, got, c.Out)
		}
	}
}

func TestWraptext(t *testing.T) {
	f := loadFixtures(t)
	for i, c := range f.Wrap {
		if got := Wraptext(c.Width, c.Text); got != c.Out {
			t.Errorf("wraptext case %d (%d, %q):\n got %q\nwant %q", i, c.Width, c.Text, got, c.Out)
		}
	}
}

func TestTable(t *testing.T) {
	f := loadFixtures(t)
	for i, c := range f.Table {
		if got := Table(c.Widths, c.Texts); got != c.Out {
			t.Errorf("table case %d (%v, %q):\n got %q\nwant %q", i, c.Widths, c.Texts, got, c.Out)
		}
	}
}

func TestLogSequences(t *testing.T) {
	f := loadFixtures(t)
	for i, c := range f.Log {
		var out bytes.Buffer
		errorCount := 0
		data := map[string]any{"verbose": c.Opts.Verbose, "debug": decodeValue(t, c.Opts.Debug), "color": c.Opts.Color}
		opts := Options{Out: &out, MaxCols: c.Opts.MaxCols, ColorCodes: c.Color}
		if c.Grunt {
			opts.ErrorCount = &errorCount
			opts.Option = func(name string) any {
				if rest, ok := strings.CutPrefix(name, "no-"); ok {
					return data[rest] == false
				}
				if v, ok := data[name]; ok {
					return v
				}
				return js.Undefined
			}
		} else {
			opts.Verbose = c.Opts.Verbose
			opts.Debug = js.ToBoolean(data["debug"])
			opts.Color = c.Opts.Color
		}
		l := New(opts)
		for j, op := range c.Ops {
			switch {
			case op.Muted != nil:
				l.SetMuted(*op.Muted)
				continue
			case op.Set != "":
				if c.Grunt {
					data[op.Set] = op.Val
					continue
				}
				o := l.Options()
				switch op.Set {
				case "verbose":
					o.Verbose = op.Val
				case "debug":
					o.Debug = op.Val
				case "color":
					o.Color = op.Val
				}
				continue
			}
			tl := target(l, op.T)
			args := make([]any, len(op.A))
			d := &decoder{t: t, refs: map[int]any{}}
			for k, a := range op.A {
				args[k] = d.decode(a)
			}
			switch op.M {
			case "writeflags":
				err := tl.Writeflags(args[0], op.Prefix)
				got := ""
				if err != nil {
					got = err.Error()
				}
				if got != op.Throws {
					t.Errorf("log case %d op %d writeflags error %q, want %q", i, j, got, op.Throws)
				}
			case "writetableln":
				tl.Writetableln(op.Widths, op.Texts)
			case "wordlist":
				if got := tl.Wordlist(args); got != op.Ret {
					t.Errorf("log case %d op %d wordlist %q, want %q", i, j, got, op.Ret)
				}
			default:
				call(t, tl, op.M, args)
			}
		}
		if out.String() != c.Out {
			t.Errorf("log case %d output:\n got %q\nwant %q", i, out.String(), c.Out)
		}
		if c.Grunt && errorCount != c.ErrorCount {
			t.Errorf("log case %d errorcount %d, want %d", i, errorCount, c.ErrorCount)
		}
		if l.HasLogged() != c.HasLogged {
			t.Errorf("log case %d hasLogged %v, want %v", i, l.HasLogged(), c.HasLogged)
		}
		if err := l.Err(); err != nil {
			t.Errorf("log case %d: %v", i, err)
		}
	}
}

func target(l *Log, name string) *Log {
	for _, part := range strings.Split(name, ".") {
		switch part {
		case "verbose":
			l = l.Verbose
		case "notverbose":
			l = l.NotVerbose
		case "or":
			l = l.Or
		case "always":
			l = l.Always
		}
	}
	return l
}

func call(t *testing.T, l *Log, method string, args []any) {
	t.Helper()
	methods := map[string]func(...any) *Log{
		"write": l.Write, "writeln": l.Writeln, "warn": l.Warn, "error": l.Error, "ok": l.Ok,
		"errorlns": l.Errorlns, "oklns": l.Oklns, "success": l.Success, "fail": l.Fail,
		"header": l.Header, "subhead": l.Subhead, "debug": l.Debug, "writelns": l.Writelns,
	}
	m, ok := methods[method]
	if !ok {
		t.Fatalf("unknown method %s", method)
	}
	m(args...)
}

type failingWriter struct{ n int }

func (w *failingWriter) Write(p []byte) (int, error) {
	w.n++
	return 0, os.ErrClosed
}

func TestWriteErrorIsKept(t *testing.T) {
	w := &failingWriter{}
	l := New(Options{Out: w})
	l.Writeln("a").Writeln("b")
	if l.Err() != os.ErrClosed {
		t.Fatalf("Err() = %v, want %v", l.Err(), os.ErrClosed)
	}
	if w.n != 1 {
		t.Fatalf("writes after the error: %d attempts, want 1", w.n)
	}
}
