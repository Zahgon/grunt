package jsrt

import (
	"math"
	"slices"
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// newArrayFromArgs is the Array constructor: one numeric argument is a length.
func newArrayFromArgs(args []any) (any, error) {
	if len(args) == 1 {
		if n, ok := args[0].(float64); ok {
			if n < 0 || n != math.Trunc(n) || n > js.MaxArrayLength {
				return nil, js.InvalidArrayLength()
			}
			if n > 1<<26 {
				// Arrays have no holes here, so huge sparse arrays cannot be built.
				return nil, js.InvalidArrayLength()
			}
			a := js.NewArray()
			a.SetLength(int(n))
			return a, nil
		}
	}
	return js.NewArray(append([]any(nil), args...)...), nil
}

// thisArray returns the receiver of an Array.prototype method. A real array is
// used in place; any other object is read as an array-like into a copy, so
// mutating methods applied to array-likes do not write back.
func (r *Realm) thisArray(this any, method string) (*js.Array, error) {
	if a, ok := this.(*js.Array); ok {
		return a, nil
	}
	if js.IsNullish(this) {
		return nil, js.NewTypeError("Array.prototype." + method + " called on null or undefined")
	}
	if !js.IsObject(this) {
		if s, ok := this.(string); ok {
			return js.NewArray(stringUnits(s)...), nil
		}
		return js.NewArray(), nil
	}
	items, err := r.arrayLike(this)
	if err != nil {
		return nil, err
	}
	return &js.Array{Items: items}, nil
}

// stringUnits splits s into one-code-unit strings, as indexing does.
func stringUnits(s string) []any {
	u := jsstr.ToUTF16(s)
	out := make([]any, len(u))
	for i := range u {
		out[i] = jsstr.FromUTF16(u[i : i+1])
	}
	return out
}

// callbackArg checks the callback argument of the iteration methods.
func (r *Realm) callbackArg(args []any) (*js.Function, error) {
	f, ok := js.Arg(args, 0).(*js.Function)
	if !ok {
		return nil, js.NewTypeError(describeValue(js.Arg(args, 0)) + " is not a function")
	}
	return f, nil
}

// relIndex converts a relative index argument (negative counts from the end).
func (r *Realm) relIndex(args []any, i int, def float64, n int) (int, error) {
	v, err := r.intArg(args, i, def)
	if err != nil {
		return 0, err
	}
	return js.RelativeIndex(v, n), nil
}

func (r *Realm) setupArray() {
	ap := r.arrayProto
	arr := r.constructor("Array", 1, ap, func(_ any, args []any) (any, error) { return newArrayFromArgs(args) }, newArrayFromArgs)
	r.method(arr, "isArray", 1, func(_ any, args []any) (any, error) {
		_, ok := js.Arg(args, 0).(*js.Array)
		return ok, nil
	})
	r.method(arr, "of", 0, func(_ any, args []any) (any, error) {
		return js.NewArray(append([]any(nil), args...)...), nil
	})
	r.method(arr, "from", 1, func(_ any, args []any) (any, error) {
		src := js.Arg(args, 0)
		if js.IsNullish(src) {
			return nil, js.NewTypeError(js.ToString(src) + " is not iterable (cannot read property Symbol(Symbol.iterator))")
		}
		mapFn := js.Arg(args, 1)
		if !isUndefined(mapFn) {
			if _, ok := mapFn.(*js.Function); !ok {
				return nil, js.NewTypeError(r.safeString(mapFn) + " is not a function")
			}
		}
		items, err := r.iterate(src, func() error { return nil })
		if err != nil {
			return nil, err
		}
		if items == nil {
			if !js.IsObject(src) {
				return js.NewArray(), nil
			}
			if items, err = r.arrayLike(src); err != nil {
				return nil, err
			}
		}
		if f, ok := mapFn.(*js.Function); ok {
			for i, v := range items {
				if items[i], err = r.call(f, js.Arg(args, 2), []any{v, float64(i)}); err != nil {
					return nil, err
				}
			}
		}
		return &js.Array{Items: items}, nil
	})

	// m installs an Array.prototype method on the receiver as an array.
	m := func(name string, length int, fn func(a *js.Array, args []any) (any, error)) {
		r.method(ap, name, length, func(this any, args []any) (any, error) {
			a, err := r.thisArray(this, name)
			if err != nil {
				return nil, err
			}
			return fn(a, args)
		})
	}
	// each calls f(item, index, array) for the indexes present when iteration
	// started that still exist, stopping when visit says so.
	each := func(a *js.Array, args []any, visit func(i int, v, res any) (bool, error)) error {
		f, err := r.callbackArg(args)
		if err != nil {
			return err
		}
		n := len(a.Items)
		for i := 0; i < n; i++ {
			if i >= len(a.Items) {
				continue
			}
			v := a.Items[i]
			res, err := r.call(f, js.Arg(args, 1), []any{v, float64(i), a})
			if err != nil {
				return err
			}
			if stop, err := visit(i, v, res); stop || err != nil {
				return err
			}
		}
		return nil
	}
	m("forEach", 1, func(a *js.Array, args []any) (any, error) {
		return js.Undefined, each(a, args, func(int, any, any) (bool, error) { return false, nil })
	})
	m("map", 1, func(a *js.Array, args []any) (any, error) {
		out := &js.Array{Items: make([]any, len(a.Items))}
		for i := range out.Items {
			out.Items[i] = js.Undefined
		}
		err := each(a, args, func(i int, _, res any) (bool, error) {
			out.Items[i] = res
			return false, nil
		})
		return out, err
	})
	m("filter", 1, func(a *js.Array, args []any) (any, error) {
		out := js.NewArray()
		err := each(a, args, func(_ int, v, res any) (bool, error) {
			if js.ToBoolean(res) {
				out.Items = append(out.Items, v)
			}
			return false, nil
		})
		return out, err
	})
	m("some", 1, func(a *js.Array, args []any) (any, error) {
		found := false
		err := each(a, args, func(_ int, _, res any) (bool, error) {
			found = js.ToBoolean(res)
			return found, nil
		})
		return found, err
	})
	m("every", 1, func(a *js.Array, args []any) (any, error) {
		all := true
		err := each(a, args, func(_ int, _, res any) (bool, error) {
			all = js.ToBoolean(res)
			return !all, nil
		})
		return all, err
	})
	// find visits every index up to the starting length, including ones removed
	// meanwhile (which read undefined).
	find := func(a *js.Array, args []any, fromEnd bool, wantIndex bool) (any, error) {
		f, err := r.callbackArg(args)
		if err != nil {
			return nil, err
		}
		n := len(a.Items)
		for k := 0; k < n; k++ {
			i := k
			if fromEnd {
				i = n - 1 - k
			}
			var v any = js.Undefined
			if i < len(a.Items) {
				v = a.Items[i]
			}
			res, err := r.call(f, js.Arg(args, 1), []any{v, float64(i), a})
			if err != nil {
				return nil, err
			}
			if js.ToBoolean(res) {
				if wantIndex {
					return float64(i), nil
				}
				return v, nil
			}
		}
		if wantIndex {
			return -1.0, nil
		}
		return js.Undefined, nil
	}
	m("find", 1, func(a *js.Array, args []any) (any, error) { return find(a, args, false, false) })
	m("findIndex", 1, func(a *js.Array, args []any) (any, error) { return find(a, args, false, true) })
	m("findLast", 1, func(a *js.Array, args []any) (any, error) { return find(a, args, true, false) })
	m("findLastIndex", 1, func(a *js.Array, args []any) (any, error) { return find(a, args, true, true) })
	reduce := func(a *js.Array, args []any, fromEnd bool) (any, error) {
		f, err := r.callbackArg(args)
		if err != nil {
			return nil, err
		}
		n := len(a.Items)
		k := 0
		var acc any
		if len(args) >= 2 {
			acc = args[1]
		} else {
			if n == 0 {
				return nil, js.NewTypeError("Reduce of empty array with no initial value")
			}
			acc = a.Items[index(fromEnd, n, 0)]
			k = 1
		}
		for ; k < n; k++ {
			i := index(fromEnd, n, k)
			if i >= len(a.Items) {
				continue
			}
			if acc, err = r.call(f, js.Undefined, []any{acc, a.Items[i], float64(i), a}); err != nil {
				return nil, err
			}
		}
		return acc, nil
	}
	m("reduce", 1, func(a *js.Array, args []any) (any, error) { return reduce(a, args, false) })
	m("reduceRight", 1, func(a *js.Array, args []any) (any, error) { return reduce(a, args, true) })
	m("join", 1, func(a *js.Array, args []any) (any, error) {
		sep := ","
		if s := js.Arg(args, 0); !isUndefined(s) {
			var err error
			if sep, err = r.toString(s); err != nil {
				return nil, err
			}
		}
		return r.join(a, sep)
	})
	r.method(ap, "toString", 0, func(this any, _ []any) (any, error) {
		if js.IsNullish(this) {
			return nil, js.NewTypeError("Cannot convert undefined or null to object")
		}
		join, _ := r.lookup(this, "join")
		if f, ok := join.(*js.Function); ok {
			return r.call(f, this, nil)
		}
		return "[object " + r.classOf(this) + "]", nil
	})
	r.method(ap, "toLocaleString", 0, func(this any, _ []any) (any, error) {
		a, err := r.thisArray(this, "toLocaleString")
		if err != nil {
			return nil, err
		}
		return r.join(a, ",")
	})
	m("push", 1, func(a *js.Array, args []any) (any, error) {
		if float64(len(a.Items)+len(args)) > js.MaxArrayLength {
			return nil, js.NewTypeError("Pushing " + js.NumberToString(float64(len(args))) + " elements on an array-like of length " + js.NumberToString(float64(len(a.Items))) + " is disallowed, as the total surpasses 2**53-1")
		}
		a.Items = append(a.Items, args...)
		return float64(len(a.Items)), nil
	})
	m("pop", 0, func(a *js.Array, _ []any) (any, error) {
		if len(a.Items) == 0 {
			return js.Undefined, nil
		}
		v := a.Items[len(a.Items)-1]
		a.Items = a.Items[:len(a.Items)-1]
		return v, nil
	})
	m("shift", 0, func(a *js.Array, _ []any) (any, error) {
		if len(a.Items) == 0 {
			return js.Undefined, nil
		}
		v := a.Items[0]
		a.Items = append(a.Items[:0:0], a.Items[1:]...)
		return v, nil
	})
	m("unshift", 1, func(a *js.Array, args []any) (any, error) {
		a.Items = append(append([]any(nil), args...), a.Items...)
		return float64(len(a.Items)), nil
	})
	m("slice", 2, func(a *js.Array, args []any) (any, error) {
		n := len(a.Items)
		from, err := r.relIndex(args, 0, 0, n)
		if err != nil {
			return nil, err
		}
		to, err := r.relIndex(args, 1, float64(n), n)
		if err != nil {
			return nil, err
		}
		if from >= to {
			return js.NewArray(), nil
		}
		return js.NewArray(append([]any(nil), a.Items[from:to]...)...), nil
	})
	m("splice", 2, func(a *js.Array, args []any) (any, error) {
		n := len(a.Items)
		start, err := r.relIndex(args, 0, 0, n)
		if err != nil {
			return nil, err
		}
		del := 0
		switch {
		case len(args) == 0:
		case len(args) == 1:
			del = n - start
		default:
			dc, err := r.intArg(args, 1, 0)
			if err != nil {
				return nil, err
			}
			del = int(math.Min(math.Max(dc, 0), float64(n-start)))
		}
		var insert []any
		if len(args) > 2 {
			insert = args[2:]
		}
		removed := js.NewArray(append([]any(nil), a.Items[start:start+del]...)...)
		rest := append(append([]any(nil), insert...), a.Items[start+del:]...)
		a.Items = append(a.Items[:start], rest...)
		return removed, nil
	})
	m("reverse", 0, func(a *js.Array, _ []any) (any, error) {
		slices.Reverse(a.Items)
		return a, nil
	})
	m("toReversed", 0, func(a *js.Array, _ []any) (any, error) {
		out := append([]any(nil), a.Items...)
		slices.Reverse(out)
		return &js.Array{Items: out}, nil
	})
	m("concat", 1, func(a *js.Array, args []any) (any, error) {
		out := append([]any(nil), a.Items...)
		for _, v := range args {
			if b, ok := v.(*js.Array); ok {
				out = append(out, b.Items...)
			} else {
				out = append(out, v)
			}
		}
		return &js.Array{Items: out}, nil
	})
	m("indexOf", 1, func(a *js.Array, args []any) (any, error) {
		n := len(a.Items)
		from, err := r.relIndex(args, 1, 0, n)
		if err != nil {
			return nil, err
		}
		for i := from; i < len(a.Items); i++ {
			if js.StrictEquals(a.Items[i], js.Arg(args, 0)) {
				return float64(i), nil
			}
		}
		return -1.0, nil
	})
	m("lastIndexOf", 1, func(a *js.Array, args []any) (any, error) {
		n := len(a.Items)
		from := n - 1
		if len(args) > 1 {
			v, err := r.intArg(args, 1, 0)
			if err != nil {
				return nil, err
			}
			if v < 0 {
				v += float64(n)
			}
			from = int(math.Min(v, float64(n-1)))
		}
		for i := from; i >= 0; i-- {
			if js.StrictEquals(a.Items[i], js.Arg(args, 0)) {
				return float64(i), nil
			}
		}
		return -1.0, nil
	})
	m("includes", 1, func(a *js.Array, args []any) (any, error) {
		n := len(a.Items)
		from, err := r.relIndex(args, 1, 0, n)
		if err != nil {
			return nil, err
		}
		for i := from; i < n; i++ {
			if js.SameValueZero(a.Items[i], js.Arg(args, 0)) {
				return true, nil
			}
		}
		return false, nil
	})
	m("at", 1, func(a *js.Array, args []any) (any, error) {
		i, err := r.intArg(args, 0, 0)
		if err != nil {
			return nil, err
		}
		if i < 0 {
			i += float64(len(a.Items))
		}
		if i < 0 || i >= float64(len(a.Items)) {
			return js.Undefined, nil
		}
		return a.Items[int(i)], nil
	})
	m("fill", 1, func(a *js.Array, args []any) (any, error) {
		n := len(a.Items)
		from, err := r.relIndex(args, 1, 0, n)
		if err != nil {
			return nil, err
		}
		to, err := r.relIndex(args, 2, float64(n), n)
		if err != nil {
			return nil, err
		}
		for i := from; i < to; i++ {
			a.Items[i] = js.Arg(args, 0)
		}
		return a, nil
	})
	m("flat", 0, func(a *js.Array, args []any) (any, error) {
		depth, err := r.intArg(args, 0, 1)
		if err != nil {
			return nil, err
		}
		return &js.Array{Items: js.Flatten(nil, a.Items, depth)}, nil
	})
	m("flatMap", 1, func(a *js.Array, args []any) (any, error) {
		var out []any
		err := each(a, args, func(_ int, _, res any) (bool, error) {
			out = js.Flatten(out, []any{res}, 1)
			return false, nil
		})
		return &js.Array{Items: out}, err
	})
	sortArgs := func(args []any) (*js.Function, error) {
		cmp := js.Arg(args, 0)
		if isUndefined(cmp) {
			return nil, nil
		}
		f, ok := cmp.(*js.Function)
		if !ok {
			return nil, js.NewTypeError("The comparison function must be either a function or undefined: " + r.safeString(cmp))
		}
		return f, nil
	}
	m("sort", 1, func(a *js.Array, args []any) (any, error) {
		cmp, err := sortArgs(args)
		if err != nil {
			return nil, err
		}
		sorted, err := r.sortItems(a.Items, cmp)
		if err != nil {
			return nil, err
		}
		copy(a.Items, sorted)
		return a, nil
	})
	m("toSorted", 1, func(a *js.Array, args []any) (any, error) {
		cmp, err := sortArgs(args)
		if err != nil {
			return nil, err
		}
		sorted, err := r.sortItems(a.Items, cmp)
		return &js.Array{Items: sorted}, err
	})
	m("with", 2, func(a *js.Array, args []any) (any, error) {
		i, err := r.intArg(args, 0, 0)
		if err != nil {
			return nil, err
		}
		if i < 0 {
			i += float64(len(a.Items))
		}
		if i < 0 || i >= float64(len(a.Items)) {
			return nil, js.NewRangeError("Invalid index : " + js.NumberToString(i))
		}
		out := append([]any(nil), a.Items...)
		out[int(i)] = js.Arg(args, 1)
		return &js.Array{Items: out}, nil
	})
	// The iterators snapshot the array when created; changes made while
	// iterating are not seen.
	m("keys", 0, func(a *js.Array, _ []any) (any, error) {
		out := make([]any, len(a.Items))
		for i := range out {
			out[i] = float64(i)
		}
		return &iterator{items: out, tag: "Array Iterator"}, nil
	})
	m("values", 0, func(a *js.Array, _ []any) (any, error) {
		return &iterator{items: append([]any(nil), a.Items...), tag: "Array Iterator"}, nil
	})
	m("entries", 0, func(a *js.Array, _ []any) (any, error) {
		out := make([]any, len(a.Items))
		for i, v := range a.Items {
			out[i] = js.NewArray(float64(i), v)
		}
		return &iterator{items: out, tag: "Array Iterator"}, nil
	})
}

// index maps step k of a forward or backward walk over n items to an index.
func index(fromEnd bool, n, k int) int {
	if fromEnd {
		return n - 1 - k
	}
	return k
}

// join is Array.prototype.join. An array already being joined (a cycle) joins as
// the empty string, as V8 does.
func (r *Realm) join(a *js.Array, sep string) (any, error) {
	if slices.Contains(r.joining, a) {
		return "", nil
	}
	r.joining = append(r.joining, a)
	defer func() { r.joining = r.joining[:len(r.joining)-1] }()
	var b strings.Builder
	for i := 0; i < len(a.Items); i++ {
		if i > 0 {
			b.WriteString(sep)
		}
		v := a.Items[i]
		if js.IsNullish(v) {
			continue
		}
		s, err := r.toString(v)
		if err != nil {
			return nil, err
		}
		b.WriteString(s)
		if b.Len() > js.MaxStringLength {
			return nil, js.InvalidStringLength()
		}
	}
	return b.String(), nil
}

// sortItems sorts a copy of items as V8 does: undefined values go last and the
// rest are ordered by V8's TimSort. Without a comparator elements compare as
// strings by UTF-16 code units; a comparator result is converted with ToNumber,
// NaN counting as 0.
func (r *Realm) sortItems(items []any, cmp *js.Function) ([]any, error) {
	var defined []any
	undefs := 0
	for _, v := range items {
		if isUndefined(v) {
			undefs++
		} else {
			defined = append(defined, v)
		}
	}
	compare := func(x, y any) (float64, error) {
		if cmp == nil {
			xs, err := r.toString(x)
			if err != nil {
				return 0, err
			}
			ys, err := r.toString(y)
			if err != nil {
				return 0, err
			}
			return float64(jsstr.Compare(xs, ys)), nil
		}
		res, err := r.call(cmp, js.Undefined, []any{x, y})
		if err != nil {
			return 0, err
		}
		n, err := r.toNumber(res)
		if err != nil || math.IsNaN(n) {
			return 0, err
		}
		return n, nil
	}
	if err := Sort(defined, compare); err != nil {
		return nil, err
	}
	for range undefs {
		defined = append(defined, js.Undefined)
	}
	return defined, nil
}
