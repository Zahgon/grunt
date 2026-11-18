package lodash

import (
	"math"
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsnum"
	"github.com/gruntjs/grunt-go/js"
)

// maxInteger is lodash's MAX_INTEGER (Number.MAX_VALUE).
const maxInteger = math.MaxFloat64

// lodashToNumber is lodash's toNumber (which underscore.string's toNumber hides on
// `_`, but which lodash's own functions call): binary and octal literals are read,
// a signed hex literal is NaN, and objects go through valueOf then their string form.
func lodashToNumber(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		return stringToNumber(x)
	case js.Date:
		return x.Ms
	}
	if js.IsObject(v) {
		return stringToNumber(js.ToString(v))
	}
	return js.ToNumber(v)
}

func stringToNumber(s string) float64 {
	s = strings.TrimFunc(s, js.IsSpace)
	if len(s) > 2 && s[0] == '0' {
		base := 0
		switch s[1] {
		case 'b', 'B':
			base = 2
		case 'o', 'O':
			base = 8
		}
		if base != 0 && strings.Trim(s[2:], "01234567"[:base]) == "" {
			// freeParseInt(value.slice(2), base)
			return jsnum.ParseInt(s[2:], base)
		}
	}
	if len(s) > 3 && (s[0] == '-' || s[0] == '+') && s[1] == '0' && (s[2] == 'x' || s[2] == 'X') {
		if strings.Trim(strings.ToLower(s[3:]), "0123456789abcdef") == "" {
			return math.NaN()
		}
	}
	return js.StringToNumber(s)
}

// toFinite is lodash's toFinite: falsy values are 0 (keeping -0), infinities clamp
// to ±Number.MAX_VALUE and NaN is 0.
func toFinite(v any) float64 {
	if !js.ToBoolean(v) {
		if f, ok := v.(float64); ok && f == 0 {
			return f
		}
		return 0
	}
	f := lodashToNumber(v)
	switch {
	case math.IsInf(f, 1):
		return maxInteger
	case math.IsInf(f, -1):
		return -maxInteger
	case math.IsNaN(f):
		return 0
	}
	return f
}

// toInteger is lodash's toInteger: toFinite without the fraction.
func toInteger(v any) float64 {
	f := toFinite(v)
	if rem := math.Mod(f, 1); rem != 0 {
		return f - rem
	}
	return f
}

// lodashToString is lodash's toString: "" for null and undefined, arrays converted
// item by item (so null items read "null"), and -0 as "-0".
func lodashToString(v any) string {
	if js.IsNullish(v) {
		return ""
	}
	return baseToString(v)
}

func baseToString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case *js.Array:
		parts := make([]string, len(x.Items))
		for i, it := range x.Items {
			parts[i] = baseToString(it)
		}
		return strings.Join(parts, ",")
	case float64:
		if x == 0 && math.Signbit(x) {
			return "-0"
		}
	}
	return js.ToString(v)
}
