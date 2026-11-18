package jsdate

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"
)

// zoneFixture is one time zone of testdata/zones.json, recorded from Node 24 by
// testdata/gen_zones.js.
type zoneFixture struct {
	Zone     string
	Instants [][5]json.RawMessage // time value, String(date), getTimezoneOffset(), local fields, toLocaleString()
	Locals   []struct {
		Args []float64
		Want *float64 // new Date(...args).getTime(); null is NaN
	}
	Parse []struct {
		In   string
		Want *float64
	}
}

func (z *zoneFixture) UnmarshalJSON(b []byte) error {
	var raw struct {
		Zone     string
		Instants [][5]json.RawMessage
		Locals   [][2]json.RawMessage
		Parse    [][2]json.RawMessage
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	z.Zone, z.Instants = raw.Zone, raw.Instants
	for _, l := range raw.Locals {
		var c struct {
			Args []float64
			Want *float64
		}
		if err := json.Unmarshal(l[0], &c.Args); err != nil {
			return err
		}
		if err := json.Unmarshal(l[1], &c.Want); err != nil {
			return err
		}
		z.Locals = append(z.Locals, c)
	}
	for _, p := range raw.Parse {
		var c struct {
			In   string
			Want *float64
		}
		if err := json.Unmarshal(p[0], &c.In); err != nil {
			return err
		}
		if err := json.Unmarshal(p[1], &c.Want); err != nil {
			return err
		}
		z.Parse = append(z.Parse, c)
	}
	return nil
}

func orNaN(p *float64) float64 {
	if p == nil {
		return math.NaN()
	}
	return *p
}

// TestLocalTimeZones replays Date's local-time behaviour recorded from Node in
// several time zones, with time.Local set to each.
func TestLocalTimeZones(t *testing.T) {
	data, err := os.ReadFile("testdata/zones.json")
	if err != nil {
		t.Fatal(err)
	}
	var zones []zoneFixture
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
		t.Run(z.Zone, func(t *testing.T) {
			for _, c := range z.Instants {
				var ms, offset float64
				var str, locale string
				var fields [8]int
				for i, dst := range []any{&ms, &str, &offset, &fields, &locale} {
					if err := json.Unmarshal(c[i], dst); err != nil {
						t.Fatal(err)
					}
				}
				if got := ToString(ms); got != str {
					t.Errorf("ToString(%v) = %q, want %q", ms, got, str)
				}
				if got := TimezoneOffset(ms); got != offset {
					t.Errorf("TimezoneOffset(%v) = %v, want %v", ms, got, offset)
				}
				y, mo, d, h, mi, s, milli, wd := LocalFields(ms)
				if got := [8]int{y, mo, d, h, mi, s, milli, wd}; got != fields {
					t.Errorf("LocalFields(%v) = %v, want %v", ms, got, fields)
				}
				if got := ToLocaleString(ms); got != locale {
					t.Errorf("ToLocaleString(%v) = %q, want %q", ms, got, locale)
				}
			}
			for _, c := range z.Locals {
				// Every recorded call passes at least year, month and day.
				a := append(append([]float64{}, c.Args...), 0, 0, 0, 0)
				if got, want := Local(a[0], a[1], a[2], a[3], a[4], a[5], a[6]), orNaN(c.Want); !same(got, want) {
					t.Errorf("Local(%v) = %v, want %v", c.Args, got, want)
				}
			}
			for _, c := range z.Parse {
				if got, want := Parse(c.In), orNaN(c.Want); !same(got, want) {
					t.Errorf("Parse(%q) = %v, want %v", c.In, got, want)
				}
			}
		})
	}
}
