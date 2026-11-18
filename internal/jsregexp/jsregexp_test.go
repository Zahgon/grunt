package jsregexp

import (
	"encoding/json"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
)

// TestNodeFixtures replays testdata/exec.json, recorded from Node 24: each case
// is [pattern, flags, input, start, result] where result is the flattened
// group offsets (-1 for undefined), null for no match, or "E:" and the
// SyntaxError message of new RegExp(pattern, flags).
func TestNodeFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/exec.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases [][5]json.RawMessage
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	failed := 0
	for _, c := range cases {
		pattern, flags, input := jsString(t, c[0]), jsString(t, c[1]), jsString(t, c[2])
		var start int
		var want any
		if err := json.Unmarshal(c[3], &start); err != nil {
			t.Fatal(err)
		}
		if c[4][0] == '"' {
			want = jsString(t, c[4])
		} else if err := json.Unmarshal(c[4], &want); err != nil {
			t.Fatal(err)
		}
		re, err := Compile(pattern, flags)
		got := any(nil)
		if err != nil {
			got = "E:" + err.Error()
		} else if m := re.Exec(jsstr.ToUTF16(input), start); m != nil {
			got = m
		}
		if !sameResult(got, want) {
			failed++
			if failed <= 40 {
				t.Errorf("/%s/%s on %q from %d: got %v, want %v", pattern, flags, input, start, got, want)
			}
		}
	}
	t.Logf("%d cases, %d failed", len(cases), failed)
}

// jsString decodes a JSON string literal into a JavaScript string in jsstr's
// WTF-8 form. encoding/json would turn escaped unpaired surrogates into U+FFFD.
func jsString(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	s := string(raw)
	if len(s) < 2 || s[0] != '"' {
		t.Fatalf("not a JSON string: %s", s)
	}
	var units []uint16
	for i := 1; i < len(s)-1; {
		if s[i] != '\\' {
			r, n := utf8.DecodeRuneInString(s[i:])
			units = utf16.AppendRune(units, r)
			i += n
			continue
		}
		switch s[i+1] {
		case 'u':
			v, err := strconv.ParseUint(s[i+2:i+6], 16, 16)
			if err != nil {
				t.Fatal(err)
			}
			units = append(units, uint16(v))
			i += 6
			continue
		case 'n':
			units = append(units, '\n')
		case 'r':
			units = append(units, '\r')
		case 't':
			units = append(units, '\t')
		case 'b':
			units = append(units, '\b')
		case 'f':
			units = append(units, '\f')
		default:
			units = append(units, uint16(s[i+1]))
		}
		i += 2
	}
	return jsstr.FromUTF16(units)
}

func sameResult(got, want any) bool {
	switch w := want.(type) {
	case nil:
		return got == nil
	case string:
		g, ok := got.(string)
		return ok && g == w
	case []any:
		g, ok := got.([]int)
		if !ok {
			return false
		}
		ws := make([]int, len(w))
		for i, x := range w {
			ws[i] = int(x.(float64))
		}
		return slices.Equal(g, ws)
	}
	return false
}

func TestEscapeSource(t *testing.T) {
	cases := map[string]string{
		"": "(?:)", "/": `\/`, "a/b": `a\/b`, "[/]": "[/]", `\/`: `\/`, "\n": `\n`, "\\\n": `\n`,
		"a\u2028b\u2029": "a\\u2028b\\u2029", "\r": `\r`, `[\]/]/`: `[\]/]\/`, "[[/]]/": `[[/]]\/`,
	}
	for in, want := range cases {
		if got := EscapeSource(in); got != want {
			t.Errorf("EscapeSource(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		literal       bool
		source, flags string
		want          string
	}{
		{false, "(", "ig", "Invalid regular expression: /(/gi: Unterminated group"},
		{true, "(", "ig", "Invalid regular expression: /(/ig: Unterminated group"},
		{false, "a", "gg", "Invalid flags supplied to RegExp constructor 'gg'"},
		{false, "a", "uv", "Invalid flags supplied to RegExp constructor 'uv'"},
		{true, "a", "gig", "Invalid regular expression flags"},
		{true, "a", "x", "Invalid regular expression flags"},
		{false, `\p{RGI_Emoji}`, "v", `unsupported: the property of strings \p{RGI_Emoji} is not supported`},
		{false, `\p{RGI_Emoji}`, "u", `Invalid regular expression: /\p{RGI_Emoji}/u: Invalid property name`},
	}
	for _, c := range cases {
		compile := Compile
		if c.literal {
			compile = CompileLiteral
		}
		_, err := compile(c.source, c.flags)
		if err == nil || err.Error() != c.want {
			t.Errorf("/%s/%s literal=%v: got %v, want %q", c.source, c.flags, c.literal, err, c.want)
		}
	}
}

func TestAccessors(t *testing.T) {
	re, err := Compile(`(?<a>x)|(?<a>y)(z)`, "ysmidg")
	if err != nil {
		t.Fatal(err)
	}
	if re.Flags() != "dgimsy" || !re.Global() || !re.IgnoreCase() || !re.Multiline() || !re.DotAll() || !re.Sticky() || !re.HasIndices() || re.Unicode() || re.UnicodeSets() {
		t.Errorf("flags %q", re.Flags())
	}
	if re.NumCaptures() != 3 || !slices.Equal(re.GroupNames(), []string{"", "a", "a", ""}) || !re.HasNamedGroups() {
		t.Errorf("groups %d %q", re.NumCaptures(), re.GroupNames())
	}
	if re.Source() != `(?<a>x)|(?<a>y)(z)` {
		t.Errorf("source %q", re.Source())
	}
	if m := re.Exec(jsstr.ToUTF16("xx"), 3); m != nil {
		t.Errorf("start past the end matched: %v", m)
	}
}

// TestLongInput makes sure deep backtracking runs on the explicit stack rather
// than the Go stack.
func TestLongInput(t *testing.T) {
	long := jsstr.ToUTF16(strings.Repeat("ab", 200000) + "c")
	for _, p := range []string{`(a|b)*c`, `(?:a|b)*?c`, `^(?:(a)|b)+c$`, `.*c`, `[ab]*?c`, `(?=(a|b)*c)`} {
		re, err := Compile(p, "")
		if err != nil {
			t.Fatal(err)
		}
		if m := re.Exec(long, 0); m == nil {
			t.Errorf("%s: no match on the long input", p)
		}
	}
}
