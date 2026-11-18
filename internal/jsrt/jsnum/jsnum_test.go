package jsnum

import (
	"math"
	"testing"
)

// The expected values were printed by Node 24.
func TestFormatting(t *testing.T) {
	fixed := map[[2]float64]string{{1.005, 2}: "1.00", {2.5, 0}: "3", {-1.5, 0}: "-2", {1e21, 2}: "1e+21", {0.000001234, 10}: "0.0000012340"}
	for in, want := range fixed {
		if got, err := ToFixed(in[0], in[1]); err != nil || got != want {
			t.Errorf("(%v).toFixed(%v) = %q, %v; want %q", in[0], in[1], got, err, want)
		}
	}
	radix := map[[2]float64]string{{255, 16}: "ff", {0.5, 2}: "0.1", {0.1, 3}: "0.0022002200220022002200220022002201", {1e-7, 36}: "0.000061oezo085tl"}
	for in, want := range radix {
		if got := ToStringRadix(in[0], int(in[1])); got != want {
			t.Errorf("(%v).toString(%v) = %q, want %q", in[0], in[1], got, want)
		}
	}
	if _, err := ToFixed(1, 101); err == nil {
		t.Error("toFixed(101) did not fail")
	}
}

func TestMath(t *testing.T) {
	cases := []struct {
		got, want float64
	}{
		{Pow(7, 255), 3.162279142922926e+215},
		{Pow(2, 10), 1024},
		{Pow(-8, 1.0/3), math.NaN()},
		{Pow(1, math.Inf(1)), math.NaN()},
		{Cbrt(1.005), 1.001663896579312},
		{Cbrt(9007199254740993), 208063.83068357586},
		{Hypot([]float64{3, 4}), 5},
		{Hypot([]float64{math.NaN(), math.Inf(-1)}), math.Inf(1)},
	}
	for i, c := range cases {
		if c.got != c.want && !(math.IsNaN(c.got) && math.IsNaN(c.want)) {
			t.Errorf("case %d: got %v, want %v", i, c.got, c.want)
		}
	}
}
