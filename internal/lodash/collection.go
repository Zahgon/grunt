package lodash

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsrt"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsnum"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// collectionEntries are lodash's array, collection, object, lang, math and
// utility functions of `_`.
func collectionEntries() []entry {
	return []entry{
		{"chunk", 3, lodashChunk},
		{"compact", 1, func(a []any) (any, error) {
			out := []any{}
			for _, v := range arrayLikeItems(js.Arg(a, 0)) {
				if js.ToBoolean(v) {
					out = append(out, v)
				}
			}
			return &js.Array{Items: out}, nil
		}},
		{"concat", 0, lodashConcat},
		{"difference", 0, func(a []any) (any, error) { return difference(js.Arg(a, 0), restArgs(a, 1)), nil }},
		{"drop", 3, func(a []any) (any, error) { return takeOrDrop(a, false), nil }},
		{"flatten", 1, func(a []any) (any, error) { return flattenItems(arrayLikeItems(js.Arg(a, 0)), 1), nil }},
		{"flattenDeep", 1, func(a []any) (any, error) { return flattenItems(arrayLikeItems(js.Arg(a, 0)), math.Inf(1)), nil }},
		{"head", 1, lodashHead},
		{"first", 1, lodashHead},
		{"indexOf", 3, lodashIndexOf},
		{"initial", 1, func(a []any) (any, error) {
			items := arrayLikeItems(js.Arg(a, 0))
			if len(items) == 0 {
				return js.NewArray(), nil
			}
			return baseSlice(items, 0, -1), nil
		}},
		{"intersection", 0, lodashIntersection},
		{"join", 2, lodashJoin},
		{"last", 1, func(a []any) (any, error) {
			items := arrayLikeItems(js.Arg(a, 0))
			if len(items) == 0 {
				return js.Undefined, nil
			}
			return items[len(items)-1], nil
		}},
		{"reverse", 1, lodashReverse},
		{"tail", 1, func(a []any) (any, error) {
			items := arrayLikeItems(js.Arg(a, 0))
			return baseSlice(items, 1, float64(len(items))), nil
		}},
		{"take", 3, func(a []any) (any, error) { return takeOrDrop(a, true), nil }},
		{"union", 0, func(a []any) (any, error) { return union(a), nil }},
		{"uniq", 1, func(a []any) (any, error) { return uniq(arrayLikeItems(js.Arg(a, 0))), nil }},
		{"without", 0, func(a []any) (any, error) {
			if !isArrayLikeObject(js.Arg(a, 0)) {
				return js.NewArray(), nil
			}
			return baseDifference(arrayLikeItems(a[0]), restArgs(a, 1)), nil
		}},
		{"countBy", 2, aggregator(func(result *js.Object, key string, _ any) {
			n, ok := result.Get(key).(float64)
			if !ok {
				n = 0
			}
			result.Set(key, n+1)
		})},
		{"every", 3, func(a []any) (any, error) { return everyOrSome(a, true) }},
		{"filter", 2, func(a []any) (any, error) { return filter(a, true) }},
		{"find", 3, lodashFind},
		{"groupBy", 2, aggregator(func(result *js.Object, key string, value any) {
			if group, ok := result.Get(key).(*js.Array); ok && result.Has(key) {
				group.Items = append(group.Items, value)
				return
			}
			result.Set(key, js.NewArray(value))
		})},
		{"includes", 4, lodashIncludes},
		{"keyBy", 2, aggregator(func(result *js.Object, key string, value any) { result.Set(key, value) })},
		{"map", 2, lodashMap},
		{"orderBy", 4, lodashOrderBy},
		{"reduce", 3, lodashReduce},
		{"reject", 2, func(a []any) (any, error) { return filter(a, false) }},
		{"size", 1, lodashSize},
		{"some", 3, func(a []any) (any, error) { return everyOrSome(a, false) }},
		{"sortBy", 0, lodashSortBy},
		{"once", 1, func(a []any) (any, error) {
			f, ok := js.Arg(a, 0).(*js.Function)
			if !ok {
				return nil, js.NewTypeError("Expected a function")
			}
			return Once(f), nil
		}},
		{"clone", 1, func(a []any) (any, error) { return Clone(js.Arg(a, 0)), nil }},
		{"cloneDeep", 1, func(a []any) (any, error) { return CloneDeep(js.Arg(a, 0)), nil }},
		{"isArray", 1, func(a []any) (any, error) { _, ok := js.Arg(a, 0).(*js.Array); return ok, nil }},
		{"isBoolean", 1, func(a []any) (any, error) { _, ok := js.Arg(a, 0).(bool); return ok, nil }},
		{"isEmpty", 1, func(a []any) (any, error) { return isEmpty(js.Arg(a, 0)), nil }},
		{"isEqual", 2, func(a []any) (any, error) { return IsEqual(js.Arg(a, 0), js.Arg(a, 1)), nil }},
		{"isNil", 1, func(a []any) (any, error) { return js.IsNullish(js.Arg(a, 0)), nil }},
		{"isNull", 1, func(a []any) (any, error) { return js.Arg(a, 0) == nil, nil }},
		{"isNumber", 1, func(a []any) (any, error) { _, ok := js.Arg(a, 0).(float64); return ok, nil }},
		{"isPlainObject", 1, func(a []any) (any, error) { _, ok := js.Arg(a, 0).(*js.Object); return ok, nil }},
		{"isString", 1, func(a []any) (any, error) { _, ok := js.Arg(a, 0).(string); return ok, nil }},
		{"isUndefined", 1, func(a []any) (any, error) { return js.Arg(a, 0) == js.Undefined, nil }},
		{"toArray", 1, func(a []any) (any, error) { return ToArray(js.Arg(a, 0)), nil }},
		{"ceil", 2, createRound(math.Ceil)},
		{"floor", 2, createRound(math.Floor)},
		{"max", 1, func(a []any) (any, error) { return extremum(arrayLikeItems(js.Arg(a, 0)), true), nil }},
		{"mean", 1, lodashMean},
		{"min", 1, func(a []any) (any, error) { return extremum(arrayLikeItems(js.Arg(a, 0)), false), nil }},
		{"round", 2, createRound(jsnum.Round)},
		{"sum", 1, func(a []any) (any, error) {
			items := arrayLikeItems(js.Arg(a, 0))
			if len(items) == 0 {
				return 0.0, nil
			}
			return baseSum(items)
		}},
		{"clamp", 3, lodashClamp},
		{"inRange", 3, lodashInRange},
		{"assign", 0, assigner(false)},
		{"assignIn", 0, assigner(true)},
		{"extend", 0, assigner(true)},
		{"defaults", 0, func(a []any) (any, error) { return defaults(a), nil }},
		{"get", 3, func(a []any) (any, error) { return get(js.Arg(a, 0), js.Arg(a, 1), js.Arg(a, 2)), nil }},
		{"has", 2, func(a []any) (any, error) {
			object := js.Arg(a, 0)
			return !js.IsNullish(object) && hasPath(object, js.Arg(a, 1), js.HasOwn), nil
		}},
		{"keys", 1, func(a []any) (any, error) { return stringArray(js.OwnKeys(js.Arg(a, 0))), nil }},
		{"merge", 0, lodashMerge},
		{"omit", 0, lodashOmit},
		{"pick", 0, lodashPick},
		{"values", 1, func(a []any) (any, error) { return &js.Array{Items: ownValues(js.Arg(a, 0))}, nil }},
		{"constant", 1, func(a []any) (any, error) {
			v := js.Arg(a, 0)
			return &js.Function{Call: func(any, []any) (any, error) { return v, nil }}, nil
		}},
		{"identity", 1, func(a []any) (any, error) { return js.Arg(a, 0), nil }},
		{"noop", 0, func([]any) (any, error) { return js.Undefined, nil }},
		{"range", 3, lodashRange},
		{"times", 2, lodashTimes},
	}
}

// restArgs returns args[from:] as a fresh slice (lodash's baseRest).
func restArgs(args []any, from int) []any {
	if from >= len(args) {
		return nil
	}
	return append([]any(nil), args[from:]...)
}

// isArrayLikeObject is lodash's isArrayLikeObject: an array, a Buffer or an object
// with a valid length (strings are array-like but not objects).
func isArrayLikeObject(v any) bool {
	if _, ok := v.([]byte); ok {
		return true
	}
	_, ok := arrayLikeLength(v)
	return ok
}

// arrayLikeItems returns the items lodash's array functions see when they index
// value[0..length): array items, string code units, Buffer bytes, or the indexed
// properties of an object with a valid length. Anything else has no items.
func arrayLikeItems(v any) []any {
	switch x := v.(type) {
	case *js.Array:
		return x.Items
	case string:
		u := jsstr.ToUTF16(x)
		items := make([]any, len(u))
		for i := range u {
			items[i] = jsstr.FromUTF16(u[i : i+1])
		}
		return items
	case []byte:
		return arrayItems(x)
	case *js.Object:
		n, ok := arrayLikeLength(x)
		if !ok {
			return nil
		}
		items := make([]any, int(n))
		for i := range items {
			items[i] = x.Get(js.NumberToString(float64(i)))
		}
		return items
	}
	return nil
}

// eachEntry calls f with the keys and values lodash's baseEach visits: indexes
// (as numbers) of array-likes, own keys of other objects. f returns false to stop.
func eachEntry(collection any, f func(key, value any) (bool, error)) error {
	if _, ok := collection.(*js.Object); !ok || isArrayLikeObject(collection) {
		for i, v := range arrayLikeItems(collection) {
			if more, err := f(float64(i), v); err != nil || !more {
				return err
			}
		}
		return nil
	}
	o := collection.(*js.Object)
	for _, k := range o.Keys() {
		if more, err := f(k, o.Get(k)); err != nil || !more {
			return err
		}
	}
	return nil
}

// ownValues is lodash's values.
func ownValues(v any) []any {
	var out []any
	for _, k := range js.OwnKeys(v) {
		out = append(out, jsrt.GetProperty(v, k))
	}
	if out == nil {
		out = []any{}
	}
	return out
}

// baseSlice is lodash's baseSlice over items, with start and end already integers.
func baseSlice(items []any, start, end float64) *js.Array {
	length := float64(len(items))
	if start < 0 {
		if -start > length {
			start = 0
		} else {
			start += length
		}
	}
	if end > length {
		end = length
	}
	if end < 0 {
		end += length
	}
	if start > end {
		return js.NewArray()
	}
	out := make([]any, int(end-start))
	copy(out, items[int(start):int(end)])
	return &js.Array{Items: out}
}

func lodashHead(a []any) (any, error) {
	items := arrayLikeItems(js.Arg(a, 0))
	if len(items) == 0 {
		return js.Undefined, nil
	}
	return items[0], nil
}

// takeOrDrop is _.take (take) or _.drop: n defaults to 1, and so does any n when
// the guard argument is given (use as an iteratee).
func takeOrDrop(a []any, take bool) *js.Array {
	items := arrayLikeItems(js.Arg(a, 0))
	if len(items) == 0 {
		return js.NewArray()
	}
	n := 1.0
	if nArg := js.Arg(a, 1); !js.ToBoolean(js.Arg(a, 2)) && nArg != js.Undefined {
		n = toInteger(nArg)
	}
	n = math.Max(n, 0)
	if take {
		return baseSlice(items, 0, n)
	}
	return baseSlice(items, n, float64(len(items)))
}

func lodashChunk(a []any) (any, error) {
	array, sizeArg, guard := js.Arg(a, 0), js.Arg(a, 1), js.Arg(a, 2)
	size := 1.0
	useDefault := sizeArg == js.Undefined
	if js.ToBoolean(guard) {
		useDefault = isIterateeCall(array, sizeArg, guard)
	}
	if !useDefault {
		size = math.Max(toInteger(sizeArg), 0)
	}
	items := arrayLikeItems(array)
	if len(items) == 0 || size < 1 {
		return js.NewArray(), nil
	}
	out := js.NewArray()
	for i := 0.0; i < float64(len(items)); i += size {
		out.Items = append(out.Items, baseSlice(items, i, i+size))
	}
	return out, nil
}

// lodashConcat is _.concat: the first argument (copied when it is an array) followed
// by the rest, with array arguments spread one level.
func lodashConcat(a []any) (any, error) {
	if len(a) == 0 {
		return js.NewArray(), nil
	}
	var out []any
	if arr, ok := a[0].(*js.Array); ok {
		out = append(out, arr.Items...)
	} else {
		out = append(out, a[0])
	}
	out = append(out, flattenItems(a[1:], 1).Items...)
	return &js.Array{Items: out}, nil
}

// flattenItems is lodash's baseFlatten: arrays are spread up to depth levels.
func flattenItems(items []any, depth float64) *js.Array {
	return &js.Array{Items: js.Flatten([]any{}, items, depth)}
}

// flattenArrayLikeObjects is baseFlatten(values, 1, isArrayLikeObject, true): only
// array-like objects contribute, each spread one level.
func flattenArrayLikeObjects(values []any) []any {
	var out []any
	for _, v := range values {
		if isArrayLikeObject(v) {
			out = append(out, arrayLikeItems(v)...)
		}
	}
	return out
}

// normalizeZero is lodash's `value !== 0 ? value : 0`, which turns -0 into +0 in
// the results of uniq, difference and intersection.
func normalizeZero(v any) any {
	if f, ok := v.(float64); ok && f == 0 {
		return 0.0
	}
	return v
}

func includesValue(items []any, v any) bool {
	for _, it := range items {
		if js.SameValueZero(it, v) {
			return true
		}
	}
	return false
}

// uniq is lodash's baseUniq with SameValueZero.
func uniq(items []any) *js.Array {
	out := []any{}
	for _, v := range items {
		if !includesValue(out, v) {
			out = append(out, normalizeZero(v))
		}
	}
	return &js.Array{Items: out}
}

// union is _.union: the unique items of the array-like arguments, in order.
func union(arrays []any) *js.Array { return uniq(flattenArrayLikeObjects(arrays)) }

// difference is _.difference(array, ...values).
func difference(array any, values []any) *js.Array {
	if !isArrayLikeObject(array) {
		return js.NewArray()
	}
	return baseDifference(arrayLikeItems(array), flattenArrayLikeObjects(values))
}

func baseDifference(items, values []any) *js.Array {
	out := []any{}
	for _, v := range items {
		if !includesValue(values, v) {
			out = append(out, normalizeZero(v))
		}
	}
	return &js.Array{Items: out}
}

// lodashIntersection is _.intersection: the unique items of the first array found
// in every other array; a non-array-like first argument gives [].
func lodashIntersection(a []any) (any, error) {
	if len(a) == 0 || !isArrayLikeObject(a[0]) {
		return js.NewArray(), nil
	}
	others := make([][]any, 0, len(a)-1)
	for _, v := range a[1:] {
		// castArrayLikeObject turns anything else into [], which matches nothing.
		others = append(others, arrayLikeItems(v))
		if !isArrayLikeObject(v) {
			others[len(others)-1] = nil
		}
	}
	out := []any{}
	for _, v := range arrayLikeItems(a[0]) {
		v = normalizeZero(v)
		if includesValue(out, v) {
			continue
		}
		inAll := true
		for _, o := range others {
			if !includesValue(o, v) {
				inAll = false
				break
			}
		}
		if inAll {
			out = append(out, v)
		}
	}
	return &js.Array{Items: out}, nil
}

// lodashJoin is _.join: Array.prototype.join called on the value, with "," as the
// default separator and null and undefined items as "".
func lodashJoin(a []any) (any, error) {
	array := js.Arg(a, 0)
	if js.IsNullish(array) {
		return "", nil
	}
	sep := ","
	if s := js.Arg(a, 1); s != js.Undefined {
		sep = js.ToString(s)
	}
	items := arrayLikeItems(array)
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = makeString(it)
	}
	return strings.Join(parts, sep), nil
}

// lodashReverse is _.reverse: Array.prototype.reverse, which reverses an array (or
// array-like object) in place and returns it. Other values are returned as the
// wrapper object Array.prototype.reverse makes of them, which for the value model
// is an object of their indexed characters.
func lodashReverse(a []any) (any, error) {
	v := js.Arg(a, 0)
	switch x := v.(type) {
	case nil, js.UndefinedType:
		return v, nil
	case *js.Array:
		slices.Reverse(x.Items)
		return x, nil
	case *js.Object:
		n, ok := arrayLikeLength(x)
		if !ok {
			return x, nil
		}
		for lo, hi := 0, int(n)-1; lo < hi; lo, hi = lo+1, hi-1 {
			loKey, hiKey := strconv.Itoa(lo), strconv.Itoa(hi)
			loVal, loOK := x.Get(loKey), x.Has(loKey)
			hiVal, hiOK := x.Get(hiKey), x.Has(hiKey)
			x.Delete(loKey)
			x.Delete(hiKey)
			if hiOK {
				x.Set(loKey, hiVal)
			}
			if loOK {
				x.Set(hiKey, loVal)
			}
		}
		return x, nil
	case string:
		if jsstr.Length(x) > 1 {
			// A String wrapper's indexes are read-only.
			return nil, js.NewTypeError("Cannot assign to read only property '0' of object '[object String]'")
		}
		wrapper := js.NewObject()
		if x != "" {
			wrapper.Set("0", x)
		}
		return wrapper, nil
	}
	if js.IsObject(v) {
		return v, nil
	}
	return js.NewObject(), nil
}

// lodashIndexOf is _.indexOf: === search (NaN finds NaN) from a fromIndex that may
// count from the end.
func lodashIndexOf(a []any) (any, error) {
	items := arrayLikeItems(js.Arg(a, 0))
	if len(items) == 0 {
		return -1.0, nil
	}
	from := 0.0
	if f := js.Arg(a, 2); !js.IsNullish(f) {
		from = toInteger(f)
	}
	if from < 0 {
		from = math.Max(float64(len(items))+from, 0)
	}
	return float64(indexOfFrom(items, js.Arg(a, 1), from)), nil
}

func indexOfFrom(items []any, v any, from float64) int {
	for i := from; i < float64(len(items)); i++ {
		if js.SameValueZero(items[int(i)], v) {
			return int(i)
		}
	}
	return -1
}

// lodashIncludes is _.includes: a substring test for strings, a SameValueZero
// search in array-likes, and a search of the values of other objects.
func lodashIncludes(a []any) (any, error) {
	collection, value := js.Arg(a, 0), js.Arg(a, 1)
	from := 0.0
	if f := js.Arg(a, 2); js.ToBoolean(f) && !js.ToBoolean(js.Arg(a, 3)) {
		from = toInteger(f)
	}
	if s, ok := collection.(string); ok {
		u := jsstr.ToUTF16(s)
		if from < 0 {
			from = math.Max(float64(len(u))+from, 0)
		}
		if from > float64(len(u)) {
			return false, nil
		}
		return jsstr.Index(u, jsstr.ToUTF16(js.ToString(value)), int(from)) > -1, nil
	}
	var items []any
	if _, isObj := collection.(*js.Object); isObj && !isArrayLikeObject(collection) {
		items = ownValues(collection)
	} else {
		items = arrayLikeItems(collection)
	}
	if from < 0 {
		from = math.Max(float64(len(items))+from, 0)
	}
	return len(items) > 0 && indexOfFrom(items, value, from) > -1, nil
}

// lodashSize is _.size: the symbol count of a string, the length of an array-like
// and the number of own keys of anything else.
func lodashSize(a []any) (any, error) {
	v := js.Arg(a, 0)
	switch x := v.(type) {
	case nil, js.UndefinedType:
		return 0.0, nil
	case string:
		return float64(stringSize(x)), nil
	}
	if n, ok := arrayLikeLength(v); ok {
		return n, nil
	}
	if b, ok := v.([]byte); ok {
		return float64(len(b)), nil
	}
	return float64(len(js.OwnKeys(v))), nil
}

func lodashMap(a []any) (any, error) {
	collection := js.Arg(a, 0)
	f := getIteratee(js.Arg(a, 1))
	out := []any{}
	err := eachEntry(collection, func(key, value any) (bool, error) {
		v, err := f(value, key, collection)
		out = append(out, v)
		return true, err
	})
	if err != nil {
		return nil, err
	}
	return &js.Array{Items: out}, nil
}

// filter is _.filter (keep) or _.reject.
func filter(a []any, keep bool) (any, error) {
	collection := js.Arg(a, 0)
	f := getIteratee(js.Arg(a, 1))
	out := []any{}
	err := eachEntry(collection, func(key, value any) (bool, error) {
		v, err := f(value, key, collection)
		if js.ToBoolean(v) == keep {
			out = append(out, value)
		}
		return true, err
	})
	if err != nil {
		return nil, err
	}
	return &js.Array{Items: out}, nil
}

// everyOrSome is _.every (every) or _.some; the guard argument drops the predicate.
func everyOrSome(a []any, every bool) (any, error) {
	collection, predicate := js.Arg(a, 0), js.Arg(a, 1)
	if guard := js.Arg(a, 2); js.ToBoolean(guard) && isIterateeCall(collection, predicate, guard) {
		predicate = js.Undefined
	}
	f := getIteratee(predicate)
	result := every
	err := eachEntry(collection, func(key, value any) (bool, error) {
		v, err := f(value, key, collection)
		if js.ToBoolean(v) != every {
			result = !every
			return false, err
		}
		return true, err
	})
	return result, err
}

// lodashFind is _.find: the first value the predicate accepts, searching from
// fromIndex (which may count from the end).
func lodashFind(a []any) (any, error) {
	collection := js.Arg(a, 0)
	f := getIteratee(js.Arg(a, 1))
	var keys, values []any
	if err := eachEntry(collection, func(key, value any) (bool, error) {
		keys = append(keys, key)
		values = append(values, value)
		return true, nil
	}); err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return js.Undefined, nil
	}
	from := 0.0
	if fi := js.Arg(a, 2); !js.IsNullish(fi) {
		from = toInteger(fi)
	}
	if from < 0 {
		from = math.Max(float64(len(keys))+from, 0)
	}
	for i := from; i < float64(len(keys)); i++ {
		v, err := f(values[int(i)], keys[int(i)], collection)
		if err != nil {
			return nil, err
		}
		if js.ToBoolean(v) {
			return values[int(i)], nil
		}
	}
	return js.Undefined, nil
}

// lodashReduce is _.reduce; without an accumulator argument the first value seeds it.
func lodashReduce(a []any) (any, error) {
	collection := js.Arg(a, 0)
	f := getIteratee(js.Arg(a, 1))
	acc := js.Arg(a, 2)
	initAccum := len(a) < 3
	err := eachEntry(collection, func(key, value any) (bool, error) {
		if initAccum {
			acc, initAccum = value, false
			return true, nil
		}
		var err error
		acc, err = f(acc, value, key, collection)
		return true, err
	})
	if err != nil {
		return nil, err
	}
	return acc, nil
}

// aggregator builds _.groupBy, _.countBy and _.keyBy: set stores each value under
// the string form of its iteratee result.
func aggregator(set func(result *js.Object, key string, value any)) func(a []any) (any, error) {
	return func(a []any) (any, error) {
		collection := js.Arg(a, 0)
		f := getIteratee(js.Arg(a, 1))
		result := js.NewObject()
		err := eachEntry(collection, func(key, value any) (bool, error) {
			k, err := f(value)
			if err == nil {
				set(result, propertyKey(k), value)
			}
			return true, err
		})
		if err != nil {
			return nil, err
		}
		return result, nil
	}
}

// isEmpty is _.isEmpty: true for null, undefined, empty array-likes and objects
// without own enumerable keys (numbers, booleans, functions and dates included).
func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil, js.UndefinedType:
		return true
	case *js.Array:
		return len(x.Items) == 0
	case string:
		return x == ""
	case []byte:
		return len(x) == 0
	case *js.Object:
		return x.Len() == 0
	}
	return true
}

// extremum is lodash's baseExtremum with > (max) or < (min): the first value is the
// first non-null, non-NaN one, replaced by any later value that compares beyond it.
func extremum(items []any, max bool) any {
	var computed any = js.Undefined
	var result any = js.Undefined
	for _, v := range items {
		if js.IsNullish(v) {
			continue
		}
		better := false
		if computed == js.Undefined {
			better = !isNaNValue(v)
		} else if max {
			better = lessThan(computed, v)
		} else {
			better = lessThan(v, computed)
		}
		if better {
			computed, result = v, v
		}
	}
	return result
}

// baseSum adds the defined values with the + operator (so strings concatenate);
// it is undefined when every value is undefined.
func baseSum(items []any) (any, error) {
	var result any = js.Undefined
	for _, v := range items {
		if v == js.Undefined {
			continue
		}
		if result == js.Undefined {
			result = v
			continue
		}
		var err error
		if result, err = js.Add(result, v, toPrimitive); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func lodashMean(a []any) (any, error) {
	items := arrayLikeItems(js.Arg(a, 0))
	if len(items) == 0 {
		return math.NaN(), nil
	}
	sum, err := baseSum(items)
	if err != nil {
		return nil, err
	}
	return js.ToNumber(toPrimitiveNumber(sum)) / float64(len(items)), nil
}

// createRound is lodash's createRound for Math.round, floor or ceil: with a
// precision the number is shifted through its exponential string form to avoid
// binary rounding errors.
func createRound(round func(float64) float64) func(a []any) (any, error) {
	return func(a []any) (any, error) {
		number := lodashToNumber(js.Arg(a, 0))
		precision := 0.0
		if p := js.Arg(a, 1); !js.IsNullish(p) {
			precision = math.Min(toInteger(p), 292)
		}
		if precision == 0 || math.IsNaN(number) || math.IsInf(number, 0) {
			return round(number), nil
		}
		shift := func(x, by float64) float64 {
			mantissa, exponent := splitExponent(baseToString(x))
			return js.StringToNumber(mantissa + "e" + js.NumberToString(js.StringToNumber(exponent)+by))
		}
		return shift(round(shift(number, precision)), -precision), nil
	}
}

// splitExponent is (s + 'e').split('e') taking the first two parts; a missing
// exponent is "" (which reads as 0).
func splitExponent(s string) (string, string) {
	for i := 0; i < len(s); i++ {
		if s[i] == 'e' {
			rest := s[i+1:]
			for j := 0; j < len(rest); j++ {
				if rest[j] == 'e' {
					return s[:i], rest[:j]
				}
			}
			return s[:i], rest
		}
	}
	return s, ""
}

func lodashClamp(a []any) (any, error) {
	number, lower, upper := js.Arg(a, 0), js.Arg(a, 1), js.Arg(a, 2)
	if upper == js.Undefined {
		upper, lower = lower, js.Undefined
	}
	toBound := func(v any) any {
		if v == js.Undefined {
			return v
		}
		f := lodashToNumber(v)
		if math.IsNaN(f) {
			return 0.0
		}
		return f
	}
	lower, upper = toBound(lower), toBound(upper)
	n := lodashToNumber(number)
	if !math.IsNaN(n) {
		if u, ok := upper.(float64); ok && !(n <= u) {
			n = u
		}
		if l, ok := lower.(float64); ok && !(n >= l) {
			n = l
		}
	}
	return n, nil
}

func lodashInRange(a []any) (any, error) {
	start, end := toFinite(js.Arg(a, 1)), 0.0
	if e := js.Arg(a, 2); e == js.Undefined {
		start, end = 0, start
	} else {
		end = toFinite(e)
	}
	n := lodashToNumber(js.Arg(a, 0))
	return n >= jsMin(start, end) && n < jsMax(start, end), nil
}

// jsMin and jsMax are Math.min and Math.max of two non-NaN numbers, with -0 below +0.
func jsMin(a, b float64) float64 {
	if a == b {
		if math.Signbit(a) {
			return a
		}
		return b
	}
	return math.Min(a, b)
}

func jsMax(a, b float64) float64 {
	if a == b {
		if math.Signbit(a) {
			return b
		}
		return a
	}
	return math.Max(a, b)
}

// lodashRange is _.range(start, end, step) (createRange without fromRight).
func lodashRange(a []any) (any, error) {
	startArg, endArg, stepArg := js.Arg(a, 0), js.Arg(a, 1), js.Arg(a, 2)
	if _, isNum := stepArg.(float64); js.ToBoolean(stepArg) && !isNum && isIterateeCall(startArg, endArg, stepArg) {
		endArg, stepArg = js.Undefined, js.Undefined
	}
	start := toFinite(startArg)
	var end float64
	if endArg == js.Undefined {
		start, end = 0, start
	} else {
		end = toFinite(endArg)
	}
	var step float64
	if stepArg == js.Undefined {
		step = 1
		if !(start < end) {
			step = -1
		}
	} else {
		step = toFinite(stepArg)
	}
	divisor := step
	if divisor == 0 {
		divisor = 1
	}
	length := math.Max(math.Ceil((end-start)/divisor), 0)
	if length > js.MaxArrayLength {
		return nil, js.InvalidArrayLength()
	}
	out := make([]any, int(length))
	for i := range out {
		out[i] = start
		start += step
	}
	return &js.Array{Items: out}, nil
}

// lodashTimes is _.times(n, iteratee): the iteratee results for 0..n-1 (indexes
// beyond MAX_ARRAY_LENGTH are not collected, as in lodash).
func lodashTimes(a []any) (any, error) {
	n := toInteger(js.Arg(a, 0))
	if n < 1 || n > maxSafeInteger {
		return js.NewArray(), nil
	}
	f := getIteratee(js.Arg(a, 1))
	length := math.Min(n, js.MaxArrayLength)
	out := make([]any, 0, int(math.Min(length, 1<<16)))
	for i := 0.0; i < length; i++ {
		v, err := f(i)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return &js.Array{Items: out}, nil
}
