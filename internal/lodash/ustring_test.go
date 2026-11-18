package lodash

import (
	"testing"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsnum"
)

// TestUnderscoreString replays calls of underscore.string 3.3.6 (and the
// sprintf-js 1.1.3 behind its sprintf) recorded on Node 24 through grunt.util._.
func TestUnderscoreString(t *testing.T) {
	runFixtures(t, "testdata/ustring.json.gz")
}

// TestNumberFormatting pins the jsnum conversions that numberFormat, toNumber and
// sprintf rely on, where Go's formatting differs: ties round up, and
// Math.pow(10, 23) is not the literal 1e23.
func TestNumberFormatting(t *testing.T) {
	str := func(s string, err error) string {
		if err != nil {
			return "error: " + err.Error()
		}
		return s
	}
	cases := []struct{ got, want string }{
		{str(jsnum.ToFixed(2.5, 0)), "3"},
		{str(jsnum.ToFixed(1.005, 2)), "1.00"},
		{str(jsnum.ToFixed(-0.0001, 2)), "-0.00"},
		{str(jsnum.ToFixed(1e21, 2)), "1e+21"},
		{str(jsnum.ToExponential(125, 1, true)), "1.3e+2"},
		{str(jsnum.ToExponential(0.000123, 0, false)), "1.23e-4"},
		{str(jsnum.ToPrecision(123.456, 4)), "123.5"},
		{str(jsnum.ToPrecision(0.00000123, 2)), "0.0000012"},
		{str(jsnum.ToPrecision(1e21, 3)), "1.00e+21"},
		{jsnum.ToStringRadix(255, 16), "ff"},
		{jsnum.ToStringRadix(-0.5, 2), "-0.1"},
	}
	for i, c := range cases {
		if c.got != c.want {
			t.Errorf("case %d: got %q, want %q", i, c.got, c.want)
		}
	}
	if got := jsnum.Pow10(23); got != 1.0000000000000001e23 {
		t.Errorf("jsnum.Pow10(23) = %v", got)
	}
	if got := jsnum.ParseInt("  -0x1F", 0); got != -31 {
		t.Errorf("ParseInt = %v", got)
	}
}
