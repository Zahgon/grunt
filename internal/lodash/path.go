package lodash

import (
	"math"

	"github.com/gruntjs/grunt-go/internal/jsregexp"
	"github.com/gruntjs/grunt-go/internal/jsrt"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/internal/lodash/jsregex"
	"github.com/gruntjs/grunt-go/js"
)

// Property paths, as lodash's get, has, pick, omit and the property iteratee
// shorthand resolve them.

var (
	// reIsDeepProp and reIsPlainProp are lodash's tests for a path that needs parsing.
	reIsDeepProp  = jsregex.MustCompile(`\.|\[(?:[^[\]]*|(["'])(?:(?!\1)[^\\]|\\.)*?\1)\]`, "")
	reIsPlainProp = jsregex.MustCompile(`^\w*$`, "")
	// rePropName matches the parts of a property path string.
	rePropName = jsregex.MustCompile(`[^.[\]]+|\[(?:(-?\d+(?:\.\d+)?)|(["'])((?:(?!\2)[^\\]|\\.)*?)\2)\]|(?=(?:\.|\[\])(?:\.|\[\]|$))`, "g")
	// reEscapeChar matches backslash escapes inside a quoted path part.
	reEscapeChar = jsregex.MustCompile(`\\(\\)?`, "g")
)

// isKey is lodash's isKey: whether value names a single property rather than a path.
func isKey(value, object any) bool {
	switch v := value.(type) {
	case *js.Array:
		return false
	case float64, bool, nil, js.UndefinedType:
		return true
	case string:
		return reIsPlainProp.Test(v) || !reIsDeepProp.Test(v) || (!js.IsNullish(object) && jsrt.HasProperty(object, v))
	}
	s := js.ToString(value)
	return reIsPlainProp.Test(s) || !reIsDeepProp.Test(s) || (!js.IsNullish(object) && jsrt.HasProperty(object, s))
}

// castPath is lodash's castPath: an array path as is, a key as a one-element path,
// and anything else parsed by stringToPath.
func castPath(value, object any) []any {
	if arr, ok := value.(*js.Array); ok {
		return arr.Items
	}
	if isKey(value, object) {
		return []any{value}
	}
	return stringToPath(lodashToString(value))
}

// stringToPath splits a path such as `a[0].b["c"]` into its keys.
func stringToPath(s string) []any {
	var result []any
	if len(s) > 0 && s[0] == '.' {
		result = append(result, "")
	}
	u := jsstr.ToUTF16(s)
	for from := 0; from <= len(u); {
		caps := rePropName.Exec(u, from)
		if caps == nil {
			break
		}
		groups := jsregexp.Captures(u, caps)
		switch {
		case groups[2] != js.Undefined:
			// A quoted part loses its escaping backslashes; the replacement cannot fail.
			part, _ := reEscapeChar.Replace(groups[3].(string), "$1")
			result = append(result, part)
		case groups[1] != js.Undefined && groups[1] != "":
			result = append(result, groups[1])
		default:
			result = append(result, groups[0])
		}
		from = caps[1]
		if caps[1] == caps[0] {
			from++
		}
	}
	return result
}

// toKey is lodash's toKey: strings as they are, -0 as "-0", the string form of the rest.
func toKey(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return baseToString(v)
}

// baseGet reads the value at path, or Undefined when the walk meets null or undefined.
func baseGet(object any, path []any) any {
	for _, key := range path {
		if js.IsNullish(object) {
			return js.Undefined
		}
		object = jsrt.GetProperty(object, toKey(key))
	}
	if len(path) == 0 {
		return js.Undefined
	}
	return object
}

// get is lodash's get.
func get(object, path, defaultValue any) any {
	var result any = js.Undefined
	if !js.IsNullish(object) {
		result = baseGet(object, castPath(path, object))
	}
	if result == js.Undefined {
		return defaultValue
	}
	return result
}

// hasPath is lodash's hasPath with js.HasOwn (has) or jsrt.HasProperty (hasIn) as
// the test. lodash's fallback for a missing last key, which accepts an index inside
// a sparse array, never applies: arrays of the value model have no holes.
func hasPath(object, path any, test func(object any, key string) bool) bool {
	keys := castPath(path, object)
	for _, k := range keys {
		key := toKey(k)
		if js.IsNullish(object) || !test(object, key) {
			return false
		}
		object = jsrt.GetProperty(object, key)
	}
	return len(keys) > 0
}

// reIsUint is lodash's test for an unsigned integer string.
var reIsUint = jsregex.MustCompile(`^(?:0|[1-9]\d*)$`, "")

// isIndexValue is lodash's isIndex without a length: a number, or a string of
// digits without leading zeros, that is a non-negative integer below
// MAX_SAFE_INTEGER.
func isIndexValue(v any) bool {
	var f float64
	switch x := v.(type) {
	case float64:
		f = x
	case string:
		if !reIsUint.Test(x) {
			return false
		}
		f = js.StringToNumber(x)
	default:
		return false
	}
	return f > -1 && f == math.Trunc(f) && f < maxSafeInteger
}
