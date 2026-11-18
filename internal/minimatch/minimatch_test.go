package minimatch

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/gruntjs/grunt-go/internal/jsregexp"
)

// The fixtures under testdata were recorded from minimatch 3.1.5 and
// brace-expansion 1.1.22 on Node 24 by testdata/gen/minimatch.js.

func loadFixture(t *testing.T, name string, v any) {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}

func TestBraceExpandFixtures(t *testing.T) {
	var cases []struct {
		Pattern    string   `json:"pattern"`
		Expansions []string `json:"expansions"`
	}
	loadFixture(t, "braces.json", &cases)
	for _, c := range cases {
		got := expandBraces(c.Pattern)
		if !slices.Equal(got, c.Expansions) {
			t.Errorf("expand(%q) = %q, want %q", c.Pattern, got, c.Expansions)
		}
	}
}

// A sequence whose bounds round to the same float never advances; the result
// cap stops it.
func TestBraceExpandSequenceCap(t *testing.T) {
	got := expandBraces("{99999999999999999999..99999999999999999998}")
	if len(got) != expansionMax {
		t.Fatalf("got %d results, want %d", len(got), expansionMax)
	}
	if got[0] != "100000000000000000000" || got[len(got)-1] != got[0] {
		t.Fatalf("unexpected values %q, %q", got[0], got[len(got)-1])
	}
}

func TestRegExpFixtures(t *testing.T) {
	var cases []struct {
		Source  string   `json:"source"`
		Flags   string   `json:"flags"`
		Valid   bool     `json:"valid"`
		Inputs  []string `json:"inputs"`
		Results string   `json:"results"`
	}
	loadFixture(t, "regexp.json", &cases)
	for _, c := range cases {
		re, err := jsregexp.Compile(c.Source, c.Flags)
		if (err == nil) != c.Valid {
			t.Errorf("new RegExp(%q, %q): valid = %v, want %v", c.Source, c.Flags, err == nil, c.Valid)
			continue
		}
		if err != nil {
			continue
		}
		for i, in := range c.Inputs {
			if got := re.Test(in); got != (c.Results[i] == '1') {
				t.Errorf("/%s/%s.test(%q) = %v, want %v", c.Source, c.Flags, in, got, !got)
			}
		}
	}
}

// fixtureOptions maps the JavaScript option names of a fixture.
func fixtureOptions(o map[string]bool) Options {
	return Options{
		Dot:        o["dot"],
		NoCase:     o["nocase"],
		NoComment:  o["nocomment"],
		NoNegate:   o["nonegate"],
		FlipNegate: o["flipNegate"],
		NoExt:      o["noext"],
		NoBrace:    o["nobrace"],
		NoGlobstar: o["noglobstar"],
		MatchBase:  o["matchBase"],
		NoNull:     o["nonull"],
		Partial:    o["partial"],
	}
}

func describePart(p Part) string {
	switch {
	case p.Globstar:
		return "G"
	case p.Regexp == nil:
		return "L:" + p.Literal
	}
	if _, ok := p.Regexp.Glob(); !ok {
		return "X"
	}
	return "R:" + p.Regexp.Source()
}

func TestMinimatchFixtures(t *testing.T) {
	var cases []struct {
		Pattern string          `json:"pattern"`
		Options map[string]bool `json:"options"`
		Error   string          `json:"error"`
		Negate  bool            `json:"negate"`
		Comment bool            `json:"comment"`
		Empty   bool            `json:"empty"`
		GlobSet []string        `json:"globSet"`
		Set     [][]string      `json:"set"`
		MakeRe  *string         `json:"makeRe"`
		Paths   []string        `json:"paths"`
		Results string          `json:"results"`
		List    []string        `json:"list"`
	}
	loadFixture(t, "minimatch.json", &cases)
	for _, c := range cases {
		opts := fixtureOptions(c.Options)
		name := c.Pattern
		if len(name) > 80 {
			name = name[:80] + "..."
		}
		m, err := New(c.Pattern, opts)
		if c.Error != "" {
			if err == nil || err.Error() != c.Error {
				t.Errorf("New(%q, %v): err = %v, want %q", name, c.Options, err, c.Error)
			}
			continue
		}
		if err != nil {
			t.Errorf("New(%q, %v): %v", name, c.Options, err)
			continue
		}
		if m.Negate != c.Negate || m.Comment != c.Comment || m.Empty != c.Empty {
			t.Errorf("New(%q, %v): negate/comment/empty = %v/%v/%v, want %v/%v/%v",
				name, c.Options, m.Negate, m.Comment, m.Empty, c.Negate, c.Comment, c.Empty)
		}
		if c.GlobSet != nil && !slices.Equal(m.GlobSet, c.GlobSet) {
			t.Errorf("New(%q, %v): globSet = %q, want %q", name, c.Options, m.GlobSet, c.GlobSet)
		}
		var set [][]string
		for _, parts := range m.Set {
			var d []string
			for _, p := range parts {
				d = append(d, describePart(p))
			}
			set = append(set, d)
		}
		if !slices.EqualFunc(set, c.Set, func(a, b []string) bool { return slices.Equal(a, b) }) {
			t.Errorf("New(%q, %v): set = %q, want %q", name, c.Options, set, c.Set)
		}
		re := m.MakeRe()
		switch {
		case (re == nil) != (c.MakeRe == nil):
			t.Errorf("makeRe(%q, %v) = %v, want %v", name, c.Options, re, c.MakeRe)
		case re != nil && re.Source() != *c.MakeRe:
			t.Errorf("makeRe(%q, %v) = %q, want %q", name, c.Options, re.Source(), *c.MakeRe)
		}
		listFailed := false
		for i, p := range c.Paths {
			got, err := Match(p, c.Pattern, opts)
			want := c.Results[i]
			switch {
			case want == 'E':
				listFailed = true
				if err == nil {
					t.Errorf("Match(%q, %q, %v) = %v, want an error", p, name, c.Options, got)
				}
			case err != nil:
				t.Errorf("Match(%q, %q, %v): %v", p, name, c.Options, err)
			case got != (want == '1'):
				t.Errorf("Match(%q, %q, %v) = %v, want %v", p, name, c.Options, got, !got)
			}
		}
		list, err := MatchList(c.Paths, c.Pattern, opts)
		if listFailed != (err != nil) {
			t.Errorf("MatchList(%q, %v): err = %v", name, c.Options, err)
		} else if !listFailed && !slices.Equal(list, c.List) {
			t.Errorf("MatchList(%q, %v) = %q, want %q", name, c.Options, list, c.List)
		}
	}
}

func TestMatchErrorMessages(t *testing.T) {
	if _, err := Match("a", "a/**/**/**", Options{}); err == nil ||
		err.Error() != "Cannot read properties of undefined (reading 'charAt')" {
		t.Errorf("Match(a, a/**/**/**) error = %v", err)
	}
	_, err := Match("a", strings.Repeat("x", maxPatternLength+1), Options{})
	if err == nil || err.Error() != "pattern is too long" {
		t.Errorf("long pattern error = %v", err)
	}
}
