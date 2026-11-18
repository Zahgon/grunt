package lodash

import (
	"math"

	"github.com/gruntjs/grunt-go/internal/jsrt"
	"github.com/gruntjs/grunt-go/js"
)

// Comparison flags of lodash's baseIsEqual.
const (
	comparePartial   = 1 << iota // the other value may have extra keys and items
	compareUnordered             // array items may match in any order
)

// eq is lodash's eq: SameValueZero, which also equates NaN with NaN.
func eq(a, b any) bool { return js.SameValueZero(a, b) }

// isObjectLike is lodash's isObjectLike: an object that is not a function.
func isObjectLike(v any) bool {
	_, isFn := v.(*js.Function)
	return js.IsObject(v) && !isFn
}

func isNaNValue(v any) bool {
	f, ok := v.(float64)
	return ok && math.IsNaN(f)
}

// equalStack tracks the pairs being compared, so cyclic structures terminate.
type equalStack map[any]any

// baseIsEqual is lodash's deep equality. With comparePartial, value is the source
// pattern of a _.matches comparison and other may have more.
func baseIsEqual(value, other any, flags int, stack equalStack) bool {
	if sameRef(value, other) {
		return true
	}
	if js.IsNullish(value) || js.IsNullish(other) || (!isObjectLike(value) && !isObjectLike(other)) {
		return isNaNValue(value) && isNaNValue(other)
	}
	return baseIsEqualDeep(value, other, flags, stack)
}

// sameRef is ===, with two Date values of the same time counting as the same
// object: the value model copies dates, so identity cannot be told apart from
// equality, and lodash would find them equal one step later anyway.
func sameRef(a, b any) bool {
	if da, ok := a.(js.Date); ok {
		db, ok := b.(js.Date)
		return ok && da.Ms == db.Ms
	}
	return js.StrictEquals(a, b)
}

func baseIsEqualDeep(object, other any, flags int, stack equalStack) bool {
	objTag, othTag := typeTag(object), typeTag(other)
	if objTag != othTag {
		return false
	}
	switch objTag {
	case "array", "uint8array":
		return equalArrays(arrayItems(object), arrayItems(other), flags, stack, object, other)
	case "date":
		return eq(object.(js.Date).Ms, other.(js.Date).Ms)
	case "regexp":
		return js.ToString(object) == js.ToString(other)
	case "error":
		a, b := object.(*js.Error), other.(*js.Error)
		return a.Name == b.Name && a.Message == b.Message
	case "object":
		return equalObjects(object.(*js.Object), other.(*js.Object), flags, stack)
	}
	return false
}

// arrayItems returns the items of an array or the bytes of a Buffer as numbers.
func arrayItems(v any) []any {
	switch x := v.(type) {
	case *js.Array:
		return x.Items
	case []byte:
		items := make([]any, len(x))
		for i, b := range x {
			items[i] = float64(b)
		}
		return items
	}
	return nil
}

func equalArrays(array, other []any, flags int, stack equalStack, arrayRef, otherRef any) bool {
	partial := flags&comparePartial != 0
	if len(array) != len(other) && !(partial && len(other) > len(array)) {
		return false
	}
	if stacked, ok := stack[arrayRef]; ok {
		if othStacked, ok := stack[otherRef]; ok {
			return stacked == otherRef && othStacked == arrayRef
		}
	}
	stack[arrayRef], stack[otherRef] = otherRef, arrayRef
	defer delete(stack, arrayRef)
	defer delete(stack, otherRef)
	var seen map[int]bool
	if flags&compareUnordered != 0 {
		seen = map[int]bool{}
	}
	for i, arrValue := range array {
		if seen != nil {
			// Each item must match a distinct item of other, in any order.
			found := false
			for j, othValue := range other {
				if !seen[j] && (sameRef(arrValue, othValue) || baseIsEqual(arrValue, othValue, flags, stack)) {
					seen[j] = true
					found = true
					break
				}
			}
			if !found {
				return false
			}
			continue
		}
		othValue := other[i]
		if !sameRef(arrValue, othValue) && !baseIsEqual(arrValue, othValue, flags, stack) {
			return false
		}
	}
	return true
}

func equalObjects(object, other *js.Object, flags int, stack equalStack) bool {
	partial := flags&comparePartial != 0
	keys := object.Keys()
	if len(keys) != other.Len() && !partial {
		return false
	}
	for _, k := range keys {
		if partial && !jsrt.HasProperty(other, k) || !partial && !other.Has(k) {
			return false
		}
	}
	if stacked, ok := stack[object]; ok {
		if othStacked, ok := stack[other]; ok {
			return stacked == other && othStacked == object
		}
	}
	stack[object], stack[other] = other, object
	defer delete(stack, object)
	defer delete(stack, other)
	for _, k := range keys {
		objValue, othValue := object.Get(k), jsrt.GetProperty(other, k)
		if !sameRef(objValue, othValue) && !baseIsEqual(objValue, othValue, flags, stack) {
			return false
		}
	}
	// Plain objects share the Object constructor, so lodash's constructor check
	// passes; an own "constructor" key was compared above like any other.
	return true
}

// isEqual is _.isEqual.
func isEqual(a, b any) bool { return baseIsEqual(a, b, 0, equalStack{}) }

// cloneValue is lodash's baseClone for the value model: arrays, plain objects,
// dates and regexps are copied (deep copies their contents too); functions and
// errors cannot be cloned, so at the top level they become {} and inside another
// value they are kept by reference. Buffers are copied.
func cloneValue(v any, deep bool, stack map[any]any, top bool) any {
	if !js.IsObject(v) {
		return v
	}
	if c, ok := stack[v]; ok {
		return c
	}
	switch x := v.(type) {
	case *js.Array:
		c := &js.Array{Items: make([]any, len(x.Items))}
		stack[x] = c
		for i, it := range x.Items {
			if deep {
				it = cloneValue(it, deep, stack, false)
			}
			c.Items[i] = it
		}
		return c
	case *js.Object:
		c := js.NewObject()
		stack[x] = c
		for _, k := range x.Keys() {
			it := x.Get(k)
			if deep {
				it = cloneValue(it, deep, stack, false)
			}
			c.Set(k, it)
		}
		return c
	case js.Date:
		return js.Date{Ms: x.Ms}
	case *js.RegExp:
		return &js.RegExp{Source: x.Source, Flags: x.Flags}
	case []byte:
		return append([]byte(nil), x...)
	}
	if top {
		return js.NewObject()
	}
	return v
}
