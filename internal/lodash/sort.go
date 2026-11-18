package lodash

import (
	"github.com/gruntjs/grunt-go/internal/jsrt"
	"github.com/gruntjs/grunt-go/js"
)

// sortItem is one entry of lodash's baseOrderBy: the iteratee results, the
// original position and the value.
type sortItem struct {
	criteria []any
	index    int
	value    any
}

func lodashSortBy(a []any) (any, error) {
	collection := js.Arg(a, 0)
	if js.IsNullish(collection) {
		return js.NewArray(), nil
	}
	iteratees := restArgs(a, 1)
	switch {
	case len(iteratees) > 1 && isIterateeCall(collection, iteratees[0], iteratees[1]):
		iteratees = nil
	case len(iteratees) > 2 && isIterateeCall(iteratees[0], iteratees[1], iteratees[2]):
		iteratees = iteratees[:1]
	}
	return baseOrderBy(collection, flattenItems(iteratees, 1).Items, nil)
}

func lodashOrderBy(a []any) (any, error) {
	collection := js.Arg(a, 0)
	if js.IsNullish(collection) {
		return js.NewArray(), nil
	}
	asList := func(v any) []any {
		if arr, ok := v.(*js.Array); ok {
			return arr.Items
		}
		if js.IsNullish(v) {
			return nil
		}
		return []any{v}
	}
	orders := js.Arg(a, 2)
	if js.ToBoolean(js.Arg(a, 3)) {
		orders = js.Undefined
	}
	return baseOrderBy(collection, asList(js.Arg(a, 1)), asList(orders))
}

// baseOrderBy sorts the collection's values by the iteratee results, in the given
// orders ("desc" descends), keeping the original order of ties.
func baseOrderBy(collection any, iteratees, orders []any) (any, error) {
	var funcs []iterateeFunc
	for _, it := range iteratees {
		if path, ok := it.(*js.Array); ok {
			// An array iteratee is a property path; a one-element array is its element.
			var p any = path
			if len(path.Items) == 1 {
				p = path.Items[0]
			}
			funcs = append(funcs, func(args ...any) (any, error) {
				v := js.Arg(args, 0)
				return baseGet(v, castPath(p, v)), nil
			})
			continue
		}
		funcs = append(funcs, getIteratee(it))
	}
	if len(funcs) == 0 {
		funcs = []iterateeFunc{getIteratee(js.Undefined)}
	}
	var items []*sortItem
	err := eachEntry(collection, func(_, value any) (bool, error) {
		item := &sortItem{index: len(items), value: value}
		for _, f := range funcs {
			c, err := f(value)
			if err != nil {
				return false, err
			}
			item.criteria = append(item.criteria, c)
		}
		items = append(items, item)
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	// lodash sorts with Array.prototype.sort; V8's TimSort gives even comparators
	// that are not consistent (compareAscending, between NaN, strings and numbers)
	// the same order as Node.
	sorted := make([]any, len(items))
	for i, it := range items {
		sorted[i] = it
	}
	// The comparator cannot fail, so neither can the sort.
	_ = jsrt.Sort(sorted, func(x, y any) (float64, error) {
		return float64(compareMultiple(x.(*sortItem), y.(*sortItem), orders)), nil
	})
	out := make([]any, len(sorted))
	for i, it := range sorted {
		out[i] = it.(*sortItem).value
	}
	return &js.Array{Items: out}, nil
}

// compareMultiple is lodash's compareMultiple: the first differing criterion
// decides, negated for a "desc" order; ties keep the original order.
func compareMultiple(x, y *sortItem, orders []any) int {
	for i := range x.criteria {
		if result := compareAscending(x.criteria[i], y.criteria[i]); result != 0 {
			if i >= len(orders) {
				return result
			}
			if looseEquals(orders[i], "desc") {
				return -result
			}
			return result
		}
	}
	return x.index - y.index
}

// compareAscending is lodash's compareAscending: undefined sorts last, then NaN,
// then null, and other values compare with < and >.
func compareAscending(value, other any) int {
	if sameRef(value, other) {
		return 0
	}
	valIsDefined, valIsNull, valIsReflexive := value != js.Undefined, value == nil, !isNaNValue(value)
	othIsDefined, othIsNull, othIsReflexive := other != js.Undefined, other == nil, !isNaNValue(other)
	if (!othIsNull && lessThan(other, value)) ||
		(valIsNull && othIsDefined && othIsReflexive) ||
		(!valIsDefined && othIsReflexive) ||
		!valIsReflexive {
		return 1
	}
	if (!valIsNull && lessThan(value, other)) ||
		(othIsNull && valIsDefined && valIsReflexive) ||
		(!othIsDefined && valIsReflexive) ||
		!othIsReflexive {
		return -1
	}
	return 0
}
