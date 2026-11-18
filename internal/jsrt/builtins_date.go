package jsrt

import (
	"math"

	"github.com/gruntjs/grunt-go/internal/jsdate"
	"github.com/gruntjs/grunt-go/js"
)

// Date. Local time is the host's time zone (see package jsdate). js.Date is an
// immutable value, so the setters are not provided.

// currentTime is the clock behind Date.now() and new Date().
func (r *Realm) currentTime() (float64, error) {
	if r.now == nil {
		return 0, js.NewError("Date.now is not available: no clock configured for this realm")
	}
	return jsdate.TimeClip(r.now()), nil
}

// dateFromArgs is the Date constructor.
func (r *Realm) dateFromArgs(args []any) (any, error) {
	switch len(args) {
	case 0:
		t, err := r.currentTime()
		return js.Date{Ms: t}, err
	case 1:
		v := args[0]
		if d, ok := v.(js.Date); ok {
			return js.Date{Ms: d.Ms}, nil
		}
		p, err := r.toPrimitive(v, "default")
		if err != nil {
			return nil, err
		}
		if s, ok := p.(string); ok {
			return js.Date{Ms: jsdate.Parse(s)}, nil
		}
		return js.Date{Ms: jsdate.TimeClip(js.ToNumber(p))}, nil
	}
	f, err := r.dateFields(args)
	if err != nil {
		return nil, err
	}
	return js.Date{Ms: jsdate.Local(f[0], f[1], f[2], f[3], f[4], f[5], f[6])}, nil
}

// dateFields converts the year, month[, day, hours, minutes, seconds, ms]
// arguments with the defaults of the Date constructor and Date.UTC.
func (r *Realm) dateFields(args []any) ([7]float64, error) {
	f := [7]float64{math.NaN(), 0, 1, 0, 0, 0, 0}
	for i := 0; i < len(args) && i < 7; i++ {
		n, err := r.toNumber(args[i])
		if err != nil {
			return f, err
		}
		f[i] = n
	}
	return f, nil
}

func (r *Realm) setupDate() {
	dp := r.dateProto
	date := r.constructor("Date", 7, dp, func(_ any, _ []any) (any, error) {
		t, err := r.currentTime()
		if err != nil {
			return nil, err
		}
		return jsdate.ToString(t), nil
	}, r.dateFromArgs)
	r.method(date, "now", 0, func(any, []any) (any, error) { return r.currentTime() })
	r.method(date, "parse", 1, func(_ any, args []any) (any, error) {
		s, err := r.toString(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		return jsdate.Parse(s), nil
	})
	r.method(date, "UTC", 7, func(_ any, args []any) (any, error) {
		f, err := r.dateFields(args)
		if err != nil {
			return nil, err
		}
		return jsdate.UTC(f[0], f[1], f[2], f[3], f[4], f[5], f[6]), nil
	})

	thisDate := func(this any, method string) (float64, error) {
		d, ok := this.(js.Date)
		if !ok {
			return 0, js.NewTypeError("this is not a Date object.")
		}
		return d.Ms, nil
	}
	// strMethod installs a formatting method; invalid dates give "Invalid Date".
	strMethod := func(name string, format func(float64) string) {
		r.method(dp, name, 0, func(this any, _ []any) (any, error) {
			ms, err := thisDate(this, name)
			if err != nil {
				return nil, err
			}
			return format(ms), nil
		})
	}
	strMethod("toString", jsdate.ToString)
	strMethod("toDateString", jsdate.ToDateString)
	strMethod("toTimeString", jsdate.ToTimeString)
	strMethod("toUTCString", jsdate.ToUTCString)
	r.define(dp, "toGMTString", dp.Get("toUTCString"))
	strMethod("toLocaleDateString", jsdate.ToLocaleDateString)
	strMethod("toLocaleTimeString", jsdate.ToLocaleTimeString)
	strMethod("toLocaleString", jsdate.ToLocaleString)
	r.method(dp, "toISOString", 0, func(this any, _ []any) (any, error) {
		d, ok := this.(js.Date)
		if !ok {
			return nil, js.NewTypeError("Method Date.prototype.toISOString called on incompatible receiver " + r.safeString(this))
		}
		iso, ok := jsdate.ISOString(d.Ms)
		if !ok {
			return nil, js.NewRangeError("Invalid time value")
		}
		return iso, nil
	})
	r.method(dp, "toJSON", 1, func(this any, _ []any) (any, error) {
		tv, err := r.toPrimitive(this, "number")
		if err != nil {
			return nil, err
		}
		if n, ok := tv.(float64); ok && (math.IsNaN(n) || math.IsInf(n, 0)) {
			return nil, nil
		}
		m, err := r.getV(this, "toISOString")
		if err != nil {
			return nil, err
		}
		f, ok := m.(*js.Function)
		if !ok {
			return nil, js.NewTypeError("toISOString is not a function")
		}
		return r.call(f, this, nil)
	})
	timeValue := func(this any, _ []any) (any, error) { return thisDate(this, "getTime") }
	r.method(dp, "getTime", 0, timeValue)
	r.method(dp, "valueOf", 0, timeValue)
	r.method(dp, "getTimezoneOffset", 0, func(this any, _ []any) (any, error) {
		ms, err := thisDate(this, "getTimezoneOffset")
		if err != nil || math.IsNaN(ms) {
			return math.NaN(), err
		}
		return jsdate.TimezoneOffset(ms), nil
	})
	// field installs a local getter and its UTC twin.
	field := func(name string, pick func(y, mo, d, h, mi, s, milli, wd int) int) {
		getter := func(method string, fields func(float64) (int, int, int, int, int, int, int, int)) {
			r.method(dp, method, 0, func(this any, _ []any) (any, error) {
				ms, err := thisDate(this, method)
				if err != nil {
					return nil, err
				}
				if math.IsNaN(ms) {
					return math.NaN(), nil
				}
				return float64(pick(fields(ms))), nil
			})
		}
		getter("get"+name, jsdate.LocalFields)
		getter("getUTC"+name, jsdate.Fields)
	}
	field("FullYear", func(y, _, _, _, _, _, _, _ int) int { return y })
	field("Month", func(_, mo, _, _, _, _, _, _ int) int { return mo })
	field("Date", func(_, _, d, _, _, _, _, _ int) int { return d })
	field("Day", func(_, _, _, _, _, _, _, wd int) int { return wd })
	field("Hours", func(_, _, _, h, _, _, _, _ int) int { return h })
	field("Minutes", func(_, _, _, _, mi, _, _, _ int) int { return mi })
	field("Seconds", func(_, _, _, _, _, s, _, _ int) int { return s })
	field("Milliseconds", func(_, _, _, _, _, _, milli, _ int) int { return milli })
	r.method(dp, "getYear", 0, func(this any, _ []any) (any, error) {
		ms, err := thisDate(this, "getYear")
		if err != nil || math.IsNaN(ms) {
			return math.NaN(), err
		}
		y, _, _, _, _, _, _, _ := jsdate.LocalFields(ms)
		return float64(y - 1900), nil
	})
}
