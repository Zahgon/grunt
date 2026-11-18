package dateformat

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gruntjs/grunt-go/internal/fixture"
	"github.com/gruntjs/grunt-go/js"
)

// value is a JavaScript argument as the fixture recorder encodes it.
type value struct {
	T string          `json:"t"`
	V json.RawMessage `json:"v"`
}

func (v value) decode(t *testing.T) any {
	t.Helper()
	switch v.T {
	case "undef":
		return js.Undefined
	case "null":
		return nil
	case "obj":
		return js.NewObject()
	case "str":
		var s string
		mustUnmarshal(t, v.V, &s)
		return s
	case "bool":
		var b bool
		mustUnmarshal(t, v.V, &b)
		return b
	case "num", "date":
		var raw any
		mustUnmarshal(t, v.V, &raw)
		f, ok := raw.(float64)
		if !ok {
			var err error
			if f, err = strconv.ParseFloat(raw.(string), 64); err != nil {
				t.Fatal(err)
			}
		}
		if v.T == "date" {
			return js.Date{Ms: f}
		}
		return f
	case "arr":
		var items []value
		mustUnmarshal(t, v.V, &items)
		arr := js.NewArray()
		for _, it := range items {
			arr.Items = append(arr.Items, it.decode(t))
		}
		return arr
	}
	t.Fatalf("unknown fixture type %q", v.T)
	return nil
}

func mustUnmarshal(t *testing.T, data []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(data, dst); err != nil {
		t.Fatal(err)
	}
}

// TestFormatNode24 replays calls recorded from dateformat 4.6.3 on Node 24 (TZ=UTC),
// with new Date() pinned to each case's now.
func TestFormatNode24(t *testing.T) {
	var cases []struct {
		Date, Mask value
		UTC, GMT   bool
		Now        float64
		Out, Err   *string
	}
	if err := fixture.DecodeGzipJSON("testdata/node24.json.gz", &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("empty fixture set")
	}
	for i, c := range cases {
		date, mask := c.Date.decode(t), c.Mask.decode(t)
		got, err := Format(date, mask, c.UTC, c.GMT, c.Now)
		if c.Err != nil {
			var jsErr *js.Error
			if !errors.As(err, &jsErr) || js.ToString(jsErr) != *c.Err {
				t.Errorf("case %d: Format(%v, %v) error = %v, want %s", i, date, mask, err, *c.Err)
			}
			continue
		}
		if err != nil || got != *c.Out {
			t.Errorf("case %d: Format(%v, %v, %v, %v) = %q, %v, want %q", i, date, mask, c.UTC, c.GMT, got, err, *c.Out)
		}
	}
}

func TestInvalidNow(t *testing.T) {
	got, err := Format(js.Date{Ms: 0}, "DDDD", false, false, math.NaN())
	if err != nil || got != "Thursday" {
		t.Errorf("got %q, %v", got, err)
	}
}

// TestFormatTimeZones replays calls recorded from dateformat 4.6.3 on Node 24 in
// several time zones (testdata/gen_zones.js), with time.Local set to each.
func TestFormatTimeZones(t *testing.T) {
	data, err := os.ReadFile("testdata/zones.json")
	if err != nil {
		t.Fatal(err)
	}
	var zones []struct {
		Zone  string
		Cases []struct {
			Date, Now float64
			Mask      string
			UTC       bool
			Out       string
		}
	}
	if err := json.Unmarshal(data, &zones); err != nil {
		t.Fatal(err)
	}
	saved := time.Local
	t.Cleanup(func() { time.Local = saved })
	for _, z := range zones {
		loc, err := time.LoadLocation(z.Zone)
		if err != nil {
			t.Fatalf("%s: %v", z.Zone, err)
		}
		time.Local = loc
		for _, c := range z.Cases {
			got, err := Format(js.Date{Ms: c.Date}, c.Mask, c.UTC, false, c.Now)
			if err != nil || got != c.Out {
				t.Errorf("%s: Format(%v, %q, %v) = %q, %v, want %q", z.Zone, c.Date, c.Mask, c.UTC, got, err, c.Out)
			}
		}
	}
}
