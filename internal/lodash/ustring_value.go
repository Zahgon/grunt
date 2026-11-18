package lodash

import (
	"math"
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsrt"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/internal/jsrt/unicase"
	"github.com/gruntjs/grunt-go/internal/lodash/jsregex"
	"github.com/gruntjs/grunt-go/js"
)

// propertyOf reads v[key]; reading from null or undefined is V8's TypeError.
func propertyOf(v any, key string) (any, error) {
	if js.IsNullish(v) {
		return nil, readError(v, key)
	}
	return jsrt.GetProperty(v, key), nil
}

// propertyKey is ToPropertyKey of a number or other value used as an index.
func propertyKey(v any) string {
	if f, ok := v.(float64); ok {
		return js.NumberToString(f)
	}
	return js.ToString(v)
}

// toPrimitive is ToPrimitive for the value model, whose objects carry no valueOf
// or toString of their own: with hint number a date gives its time value, and
// otherwise an object gives its string form (Date.prototype.toString for a date).
func toPrimitive(v any, hint string) (any, error) {
	if d, ok := v.(js.Date); ok && hint == "number" {
		return d.Ms, nil
	}
	if js.IsObject(v) {
		return js.ToString(v), nil
	}
	return v, nil
}

// toPrimitiveNumber is toPrimitive with hint number, which cannot fail.
func toPrimitiveNumber(v any) any {
	p, _ := toPrimitive(v, "number")
	return p
}

// lessThan is the < operator on the value model (NaN is never less).
func lessThan(a, b any) bool {
	lt, ok, _ := js.LessThan(a, b, true, toPrimitive)
	return ok && lt
}

// looseEquals is the == operator on the value model.
func looseEquals(a, b any) bool {
	eq, _ := js.LooselyEqual(a, b, toPrimitive)
	return eq
}

// regexpOf compiles a *js.RegExp value with its own flags.
func regexpOf(r *js.RegExp) (*jsregex.Regexp, error) {
	return jsregex.Compile(r.Source, r.Flags)
}

// replaceUnits is String.prototype.replace with a replacement computed in Go from
// the matched code units.
func replaceUnits(re *jsregex.Regexp, u []uint16, f func(match []uint16, caps []int) []uint16) []uint16 {
	// f cannot fail, so neither can ReplaceFunc.
	out, _ := re.ReplaceFunc(u, func(caps []int) ([]uint16, error) { return f(u[caps[0]:caps[1]], caps), nil })
	return out
}

// upperUnits and lowerUnits are toUpperCase and toLowerCase on code units, which
// leave lone surrogates untouched.
func upperUnits(u []uint16) []uint16 { return unicase.ToUpper(u) }
func lowerUnits(u []uint16) []uint16 { return unicase.ToLower(u) }

// trimSpace is String.prototype.trim (and trimStart / trimEnd with left or right
// alone): ECMAScript white space and line terminators.
func trimSpace(s string, left, right bool) string {
	if left {
		s = strings.TrimLeftFunc(s, js.IsSpace)
	}
	if right {
		s = strings.TrimRightFunc(s, js.IsSpace)
	}
	return s
}

// typeTag is Object.prototype.toString's tag, lowercased: "undefined", "null",
// "number", "string", "boolean", "array", "object", "function", "date", "regexp",
// "error" or "uint8array" (a Buffer).
func typeTag(v any) string { return strings.ToLower(js.ClassOf(v)) }

// stringRepeat is String.prototype.repeat(count) for a non-negative integral count.
// A result longer than V8's string limit is a RangeError.
func stringRepeat(s string, count float64) (string, error) {
	if count == 0 || s == "" {
		return "", nil
	}
	if float64(jsstr.Length(s))*count > js.MaxStringLength {
		return "", js.InvalidStringLength()
	}
	return strings.Repeat(s, int(count)), nil
}

func isFinite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }
