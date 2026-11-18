package unicase

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
	"unicode"
)

// TestNodeFixtures replays testdata/case.json, recorded from Node 24: every
// character with a multi-character full case mapping, plus random strings mixing
// sigmas, case-ignorable characters, special mappings, astral letters and
// unpaired surrogates. Each case is [input, toUpperCase, toLowerCase] as UTF-16
// code units.
func TestNodeFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/case.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases [][3][]uint16
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if got := ToUpper(c[0]); !slices.Equal(got, c[1]) {
			t.Errorf("ToUpper(%x) = %x, want %x", c[0], got, c[1])
		}
		if got := ToLower(c[0]); !slices.Equal(got, c[2]) {
			t.Errorf("ToLower(%x) = %x, want %x", c[0], got, c[2])
		}
	}
}

// TestCanonicalizeIdempotent checks the properties jsre's case-closure of
// character classes relies on: Canonicalize is idempotent and CaseVariants lists
// exactly the characters it changes.
func TestCanonicalizeIdempotent(t *testing.T) {
	for _, u := range []bool{false, true} {
		for c := range rune(unicode.MaxRune + 1) {
			k := Canonicalize(c, u)
			if Canonicalize(k, u) != k {
				t.Fatalf("Canonicalize(%#x, %v) = %#x is not canonical", c, u, k)
			}
		}
		var all []rune
		for c := range rune(unicode.MaxRune + 1) {
			if Canonicalize(c, u) != c {
				all = append(all, c)
			}
		}
		if !slices.Equal(all, CaseVariants(u)) {
			t.Fatalf("CaseVariants(%v) has %d characters, want %d", u, len(CaseVariants(u)), len(all))
		}
	}
}

func TestCanonicalizeExamples(t *testing.T) {
	cases := []struct {
		c       rune
		unicode bool
		want    rune
	}{
		{'a', false, 'A'}, {'ſ', false, 'ſ'}, {'ß', false, 'ß'}, {'ǆ', false, 'Ǆ'}, {0x212A, false, 0x212A},
		{'ſ', true, 'S'}, {0x212A, true, 'K'}, {'k', true, 'K'}, {'ς', true, 'Σ'}, {0x1E9E, true, 'ß'},
	}
	for _, c := range cases {
		if got := Canonicalize(c.c, c.unicode); got != c.want {
			t.Errorf("Canonicalize(%q, %v) = %q, want %q", c.c, c.unicode, got, c.want)
		}
	}
}
