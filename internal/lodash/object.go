package lodash

import (
	"github.com/gruntjs/grunt-go/internal/jsrt"
	"github.com/gruntjs/grunt-go/js"
)

// toObject is Object(value) for an assignment target: objects as they are, null
// and undefined as a new object. A primitive becomes its wrapper object, which the
// value model represents as a plain object of its own keys: a string's indexed
// characters, nothing for numbers and booleans.
func toObject(v any) any {
	if js.IsObject(v) {
		return v
	}
	o := js.NewObject()
	if s, ok := v.(string); ok {
		for i, c := range arrayLikeItems(s) {
			o.Set(js.NumberToString(float64(i)), c)
		}
	}
	return o
}

// sourceCount applies createAssigner's guard: when the call looks like an
// iteratee call (object, index, collection), only the first source is used.
func sourceCount(sources []any) int {
	if len(sources) > 2 && js.ToBoolean(sources[2]) && isIterateeCall(sources[0], sources[1], sources[2]) {
		return 1
	}
	return len(sources)
}

// assigner builds _.assign (own keys) and _.assignIn/_.extend (own and inherited
// keys, the same in the value model): truthy sources are copied key by key.
func assigner(in bool) func(a []any) (any, error) {
	return func(a []any) (any, error) {
		object := toObject(js.Arg(a, 0))
		sources := restArgs(a, 1)
		for _, source := range sources[:sourceCount(sources)] {
			if js.ToBoolean(source) {
				assignKeys(object, source)
			}
		}
		return object, nil
	}
}

func assignKeys(object, source any) {
	for _, k := range js.OwnKeys(source) {
		js.SetProperty(object, k, jsrt.GetProperty(source, k))
	}
}

// defaults is _.defaults: keys of each source fill in the target's missing or
// undefined keys.
func defaults(a []any) any {
	object := toObject(js.Arg(a, 0))
	sources := restArgs(a, 1)
	for _, source := range sources[:sourceCount(sources)] {
		for _, k := range js.OwnKeys(source) {
			if !js.HasOwn(object, k) || jsrt.GetProperty(object, k) == js.Undefined {
				js.SetProperty(object, k, jsrt.GetProperty(source, k))
			}
		}
	}
	return object
}

func lodashMerge(a []any) (any, error) {
	object := toObject(js.Arg(a, 0))
	sources := restArgs(a, 1)
	for _, source := range sources[:sourceCount(sources)] {
		if js.ToBoolean(source) {
			baseMerge(object, source, map[any]any{})
		}
	}
	return object, nil
}

// safeGet is lodash's safeGet: "__proto__" and an inherited or function-valued
// "constructor" read as undefined.
func safeGet(object any, key string) any {
	if key == "__proto__" {
		return js.Undefined
	}
	v := jsrt.GetProperty(object, key)
	if _, isFn := v.(*js.Function); key == "constructor" && (isFn || !js.HasOwn(object, key)) {
		return js.Undefined
	}
	return v
}

// assignMergeValue is lodash's assignMergeValue: a defined value is stored unless
// equal to the current one; undefined is stored only for a key not yet present.
func assignMergeValue(object any, key string, value any) {
	if value != js.Undefined && !eq(jsrt.GetProperty(object, key), value) || value == js.Undefined && !jsrt.HasProperty(object, key) {
		js.SetProperty(object, key, value)
	}
}

// baseMerge is lodash's baseMerge: plain objects and arrays of the source are
// merged into the matching target values (created when missing), other values are
// assigned. stack maps sources being merged to their targets, for cycles.
func baseMerge(object, source any, stack map[any]any) {
	if sameRef(object, source) {
		return
	}
	for _, key := range js.OwnKeys(source) {
		srcValue := jsrt.GetProperty(source, key)
		if !js.IsObject(srcValue) {
			assignMergeValue(object, key, srcValue)
			continue
		}
		baseMergeDeep(object, source, key, stack)
	}
}

func baseMergeDeep(object, source any, key string, stack map[any]any) {
	objValue, srcValue := safeGet(object, key), safeGet(source, key)
	if stacked, ok := stackGet(stack, srcValue); ok {
		assignMergeValue(object, key, stacked)
		return
	}
	newValue := srcValue
	isCommon := true
	switch src := srcValue.(type) {
	case *js.Array, []byte:
		switch {
		case isArrayValue(objValue):
			newValue = objValue
		case isArrayLikeObject(objValue):
			newValue = &js.Array{Items: append([]any(nil), arrayLikeItems(objValue)...)}
		default:
			if b, isBuf := src.([]byte); isBuf {
				// Buffers are cloned rather than merged.
				newValue, isCommon = append([]byte(nil), b...), false
			} else {
				newValue = js.NewArray()
			}
		}
	case *js.Object:
		newValue = objValue
		if _, isFn := objValue.(*js.Function); !js.IsObject(objValue) || isFn {
			newValue = js.NewObject()
		}
	default:
		// Dates, regexps, errors and functions are assigned as they are.
		isCommon = false
	}
	if isCommon {
		stack[srcValue] = newValue
		baseMerge(newValue, srcValue, stack)
		delete(stack, srcValue)
	}
	assignMergeValue(object, key, newValue)
}

func isArrayValue(v any) bool {
	_, ok := v.(*js.Array)
	return ok
}

// stackGet looks a source value up in a merge stack; only reference values can be
// on it.
func stackGet(stack map[any]any, v any) (any, bool) {
	switch v.(type) {
	case *js.Array, *js.Object:
		c, ok := stack[v]
		return c, ok
	}
	return nil, false
}

// lodashPick is _.pick(object, ...paths): the values at the paths that exist
// (inherited ones included), at the same paths of a new object.
func lodashPick(a []any) (any, error) {
	object := js.Arg(a, 0)
	result := js.NewObject()
	if js.IsNullish(object) {
		return result, nil
	}
	for _, path := range flattenItems(restArgs(a, 1), 1).Items {
		value := get(object, path, js.Undefined)
		if hasPath(object, path, jsrt.HasProperty) {
			baseSet(result, castPath(path, object), value)
		}
	}
	return result, nil
}

// baseSet is lodash's baseSet: it creates missing objects (or arrays, before an
// index key) along the path and refuses __proto__, constructor and prototype keys,
// stopping where it meets one.
func baseSet(object any, path []any, value any) {
	nested := object
	for i, k := range path {
		if js.IsNullish(nested) {
			return
		}
		key := toKey(k)
		if key == "__proto__" || key == "constructor" || key == "prototype" {
			return
		}
		newValue := value
		if i != len(path)-1 {
			objValue := jsrt.GetProperty(nested, key)
			switch {
			case js.IsObject(objValue):
				newValue = objValue
			case isIndexValue(path[i+1]):
				newValue = js.NewArray()
			default:
				newValue = js.NewObject()
			}
		}
		js.SetProperty(nested, key, newValue)
		nested = jsrt.GetProperty(nested, key)
	}
}

// lodashOmit is _.omit(object, ...paths): a copy of the object's own and inherited
// keys without the paths. A deep path clones the plain objects on the way (arrays
// stay shared, as lodash's customOmitClone keeps them).
func lodashOmit(a []any) (any, error) {
	object := js.Arg(a, 0)
	var result any = js.NewObject()
	if js.IsNullish(object) {
		return result, nil
	}
	var paths [][]any
	deep := false
	for _, p := range flattenItems(restArgs(a, 1), 1).Items {
		path := castPath(p, object)
		deep = deep || len(path) > 1
		paths = append(paths, path)
	}
	assignKeys(result, object)
	if deep {
		result = clonePlainObjects(result)
	}
	for i := len(paths) - 1; i >= 0; i-- {
		baseUnset(result, paths[i])
	}
	return result, nil
}

// clonePlainObjects deep-clones plain objects and keeps every other value.
func clonePlainObjects(v any) any {
	o, ok := v.(*js.Object)
	if !ok {
		return v
	}
	c := js.NewObject()
	for _, k := range o.Keys() {
		c.Set(k, clonePlainObjects(o.Get(k)))
	}
	return c
}

// baseUnset is lodash 4.18.1's baseUnset: it deletes the last key of path from its
// parent, refusing a path through __proto__ (unless own) or through constructor or
// prototype.
func baseUnset(object any, path []any) {
	for i, k := range path {
		key := toKey(k)
		if key == "__proto__" && !js.HasOwn(object, "__proto__") {
			return
		}
		if (key == "constructor" || key == "prototype") && i < len(path)-1 {
			return
		}
	}
	if len(path) == 0 {
		return
	}
	parent := object
	if len(path) > 1 {
		parent = baseGet(object, path[:len(path)-1])
	}
	if !js.IsNullish(parent) {
		js.DeleteProperty(parent, toKey(path[len(path)-1]))
	}
}
