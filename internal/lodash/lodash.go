// Package lodash implements the `_` that grunt's templates and grunt.util._ expose:
// grunt-legacy-util's lodash 4.18.1 with underscore.string 3.3.6 mixed in. The
// Underscore function builds that object for the template runtime; the exported Go
// functions below are the parts grunt's own code calls. Values follow the js value
// model, and every function reproduces the original's JavaScript semantics,
// coercions and edge cases.
package lodash

import (
	"math"

	"github.com/gruntjs/grunt-go/js"
)

// Merge is _.merge(dst, ...sources): plain objects and arrays of each source are
// merged recursively into dst by key and index, undefined source values do not
// overwrite existing keys, and "__proto__" and function-valued "constructor" keys
// are not followed. It returns dst.
func Merge(dst *js.Object, sources ...any) *js.Object {
	for _, source := range sources {
		if js.ToBoolean(source) {
			baseMerge(dst, source, map[any]any{})
		}
	}
	return dst
}

// Extend is _.extend (assignIn): the keys of each truthy source are copied onto
// dst in order. It returns dst.
func Extend(dst *js.Object, sources ...any) *js.Object {
	for _, source := range sources {
		if js.ToBoolean(source) {
			assignKeys(dst, source)
		}
	}
	return dst
}

// Defaults is _.defaults: each source fills the keys dst lacks or holds undefined.
// It returns dst.
func Defaults(dst *js.Object, sources ...any) *js.Object {
	return defaults(append([]any{dst}, sources...)).(*js.Object)
}

// Union is _.union: the unique items (SameValueZero) of the array-like arguments,
// in order of first appearance.
func Union(arrays ...any) *js.Array { return union(arrays) }

// Difference is _.difference: the items of array not found in any of values.
func Difference(array any, values ...any) *js.Array { return difference(array, values) }

// Uniq is _.uniq: the unique items of array (SameValueZero), first occurrence kept.
func Uniq(array any) *js.Array { return uniq(arrayLikeItems(array)) }

// Flatten is _.flatten: array with nested arrays spread one level.
func Flatten(array any) *js.Array { return flattenItems(arrayLikeItems(array), 1) }

// FlattenDeep is _.flattenDeep: array with all nested arrays spread.
func FlattenDeep(array any) *js.Array {
	return flattenItems(arrayLikeItems(array), math.Inf(1))
}

// Once is _.once: a function that calls fn the first time (passing this and the
// arguments through) and afterwards returns that first result.
func Once(fn *js.Function) *js.Function {
	var called bool
	var result any = js.Undefined
	return &js.Function{Call: func(this any, args []any) (any, error) {
		if called {
			return result, nil
		}
		called = true
		r, err := fn.Call(this, args)
		if err != nil {
			return nil, err
		}
		result = r
		return result, nil
	}}
}

// ToArray is _.toArray: the symbols of a string, a copy of an array-like, the
// values of an object, and [] for falsy and other values.
func ToArray(v any) *js.Array {
	if !js.ToBoolean(v) {
		return js.NewArray()
	}
	switch x := v.(type) {
	case string:
		return stringArray(stringToArray(x))
	case *js.Object:
		if isArrayLikeObject(x) {
			return &js.Array{Items: append([]any(nil), arrayLikeItems(x)...)}
		}
		return &js.Array{Items: ownValues(x)}
	case *js.Array, []byte:
		return &js.Array{Items: append([]any(nil), arrayLikeItems(x)...)}
	}
	return js.NewArray()
}

// Clone is _.clone: a shallow copy of arrays, plain objects, dates, regexps and
// Buffers; functions and errors give {}; primitives are returned as they are.
func Clone(v any) any { return cloneValue(v, false, map[any]any{}, true) }

// CloneDeep is _.cloneDeep: Clone applied recursively, preserving shared and
// cyclic references; nested functions and errors are kept by reference.
func CloneDeep(v any) any { return cloneValue(v, true, map[any]any{}, true) }

// IsEqual is _.isEqual: deep equality of arrays, plain objects, dates, regexps and
// errors, with SameValueZero for primitives.
func IsEqual(a, b any) bool { return isEqual(a, b) }

// Pad is underscore.string's pad(str, length, padStr, type), which grunt.util._.pad
// resolves to: str padded with the first character of padStr (default " ") on the
// left, the right ("right") or both sides ("both") up to length.
func Pad(str, length, padStr, typ any) (string, error) { return pad(str, length, padStr, typ) }
