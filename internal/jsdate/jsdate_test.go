package jsdate

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/gruntjs/grunt-go/internal/fixture"
)

func init() {
	// The fixtures were recorded by Node with TZ=UTC.
	time.Local = time.UTC
}

// fixtures are recorded from Node 24 with TZ=UTC; null stands for NaN.
type fixtures struct {
	Parse [][2]any          `json:"parse"`
	UTC   [][2]any          `json:"utc"`
	ISO   []json.RawMessage `json:"iso"`
}

func loadFixtures(t *testing.T) fixtures {
	t.Helper()
	var f fixtures
	if err := fixture.DecodeGzipJSON("testdata/node24.json.gz", &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Parse) == 0 || len(f.UTC) == 0 || len(f.ISO) == 0 {
		t.Fatal("empty fixture set")
	}
	return f
}

// number decodes a fixture number: null is NaN and the strings name non-finite values.
func number(v any) float64 {
	switch x := v.(type) {
	case nil:
		return math.NaN()
	case float64:
		return x
	case string:
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			panic(err)
		}
		return f
	}
	panic("unexpected fixture value")
}

func same(a, b float64) bool { return a == b || (math.IsNaN(a) && math.IsNaN(b)) }

func TestParse(t *testing.T) {
	for _, c := range loadFixtures(t).Parse {
		in := c[0].(string)
		want := number(c[1])
		if got := Parse(in); !same(got, want) {
			t.Errorf("Parse(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestUTC(t *testing.T) {
	for _, c := range loadFixtures(t).UTC {
		raw := c[0].([]any)
		a := make([]float64, len(raw))
		for i, v := range raw {
			a[i] = number(v)
		}
		want := number(c[1])
		if got := UTC(a[0], a[1], a[2], a[3], a[4], a[5], a[6]); !same(got, want) {
			t.Errorf("UTC(%v) = %v, want %v", a, got, want)
		}
	}
}

func TestISOStringAndFields(t *testing.T) {
	for _, raw := range loadFixtures(t).ISO {
		var c struct {
			ms     *float64
			iso    *string
			fields []int
		}
		var tuple []json.RawMessage
		if err := json.Unmarshal(raw, &tuple); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(tuple[0], &c.ms); err != nil {
			t.Fatal(err)
		}
		if c.ms == nil {
			if _, ok := ISOString(math.NaN()); ok {
				t.Error("ISOString(NaN) reported a valid date")
			}
			continue
		}
		if err := json.Unmarshal(tuple[1], &c.iso); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(tuple[2], &c.fields); err != nil {
			t.Fatal(err)
		}
		got, ok := ISOString(*c.ms)
		if !ok || got != *c.iso {
			t.Errorf("ISOString(%v) = %q, %v, want %q", *c.ms, got, ok, *c.iso)
		}
		y, mo, d, h, mi, s, ms, wd := Fields(*c.ms)
		gotFields := []int{y, mo, d, h, mi, s, ms, wd}
		for i := range gotFields {
			if gotFields[i] != c.fields[i] {
				t.Errorf("Fields(%v) = %v, want %v", *c.ms, gotFields, c.fields)
				break
			}
		}
	}
}
