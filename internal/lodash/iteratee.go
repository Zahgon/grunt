package lodash

import (
	"github.com/gruntjs/grunt-go/internal/jsrt"
	"github.com/gruntjs/grunt-go/js"
)

// iterateeFunc is a resolved iteratee: it receives the arguments lodash passes
// (value, index or key, collection; or accumulator first for reduce).
type iterateeFunc func(args ...any) (any, error)

// getIteratee is lodash's baseIteratee: a function is called as it is, null and
// undefined mean identity, an array [path, value] is _.matchesProperty, any other
// object is _.matches, and anything else (a string or number) is _.property.
func getIteratee(v any) iterateeFunc {
	switch x := v.(type) {
	case *js.Function:
		return func(args ...any) (any, error) { return x.Call(js.Undefined, args) }
	case nil, js.UndefinedType:
		return func(args ...any) (any, error) { return js.Arg(args, 0), nil }
	case *js.Array:
		return matchesProperty(js.Arg(x.Items, 0), js.Arg(x.Items, 1))
	}
	if js.IsObject(v) {
		return matches(v)
	}
	return propertyIteratee(v)
}

// propertyIteratee is _.property(path).
func propertyIteratee(path any) iterateeFunc {
	return func(args ...any) (any, error) {
		object := js.Arg(args, 0)
		if isKey(path, nil) {
			if js.IsNullish(object) {
				return js.Undefined, nil
			}
			return jsrt.GetProperty(object, toKey(path)), nil
		}
		return baseGet(object, castPath(path, object)), nil
	}
}

// isStrictComparable reports whether === suffices to compare with v.
func isStrictComparable(v any) bool { return !isNaNValue(v) && !js.IsObject(v) }

// matchesStrictComparable is the fast path of _.matches and _.matchesProperty.
func matchesStrictComparable(key string, src any) iterateeFunc {
	return func(args ...any) (any, error) {
		object := js.Arg(args, 0)
		if js.IsNullish(object) {
			return false, nil
		}
		return js.StrictEquals(jsrt.GetProperty(object, key), src) && (src != js.Undefined || jsrt.HasProperty(object, key)), nil
	}
}

// matches is lodash's baseMatches(source): a partial deep comparison with source.
func matches(source any) iterateeFunc {
	keys := js.OwnKeys(source)
	if len(keys) == 1 {
		if v := jsrt.GetProperty(source, keys[0]); isStrictComparable(v) {
			return matchesStrictComparable(keys[0], v)
		}
	}
	return func(args ...any) (any, error) {
		object := js.Arg(args, 0)
		if sameRef(object, source) {
			return true, nil
		}
		return baseIsMatch(object, source, keys), nil
	}
}

// baseIsMatch is lodash's baseIsMatch without a customizer.
func baseIsMatch(object, source any, keys []string) bool {
	if js.IsNullish(object) {
		return len(keys) == 0
	}
	for i := len(keys) - 1; i >= 0; i-- {
		src := jsrt.GetProperty(source, keys[i])
		if isStrictComparable(src) {
			if !js.StrictEquals(src, jsrt.GetProperty(object, keys[i])) {
				return false
			}
		} else if !jsrt.HasProperty(object, keys[i]) {
			return false
		}
	}
	for _, k := range keys {
		src, objValue := jsrt.GetProperty(source, k), jsrt.GetProperty(object, k)
		if isStrictComparable(src) {
			if objValue == js.Undefined && !jsrt.HasProperty(object, k) {
				return false
			}
		} else if !baseIsEqual(src, objValue, comparePartial|compareUnordered, equalStack{}) {
			return false
		}
	}
	return true
}

// matchesProperty is lodash's baseMatchesProperty(path, srcValue).
func matchesProperty(path, src any) iterateeFunc {
	if isKey(path, nil) && isStrictComparable(src) {
		return matchesStrictComparable(toKey(path), src)
	}
	return func(args ...any) (any, error) {
		object := js.Arg(args, 0)
		objValue := get(object, path, js.Undefined)
		if objValue == js.Undefined && src == js.Undefined {
			return !js.IsNullish(object) && hasPath(object, path, jsrt.HasProperty), nil
		}
		return baseIsEqual(src, objValue, comparePartial|compareUnordered, equalStack{}), nil
	}
}
