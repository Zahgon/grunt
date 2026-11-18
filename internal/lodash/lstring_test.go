package lodash

import (
	"math"
	"testing"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsnum"
)

// TestLodashStringsNode24 replays lodash string calls recorded from Node 24.
func TestLodashStringsNode24(t *testing.T) {
	runFixtures(t, "testdata/lstring.json.gz")
}

// TestDeburrCoversLatin checks that every character of lodash's reLatin range has
// a deburred form, so deburrLetter never yields undefined.
func TestDeburrCoversLatin(t *testing.T) {
	for r := rune(0xc0); r <= 0x17f; r++ {
		if _, ok := deburredLetters[r]; isLatinLetter(r) && !ok {
			t.Errorf("%U has no deburred form", r)
		}
	}
}

func TestParseIntRadixRounding(t *testing.T) {
	cases := []struct {
		s     string
		radix int
		want  float64
	}{
		{"zzzzzzzzzzzzzzzzzz", 36, 1.0314424798490537e+28},
		{"123456789012345678901234567890", 36, 1.436287679432363e+45},
		{"-0", 0, math.Copysign(0, -1)},
	}
	for _, c := range cases {
		if got := jsnum.ParseInt(c.s, c.radix); got != c.want || math.Signbit(got) != math.Signbit(c.want) {
			t.Errorf("parseInt(%q, %d) = %v, want %v", c.s, c.radix, got, c.want)
		}
	}
}
