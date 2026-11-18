package jsrt

import (
	"math"
	"slices"
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/internal/jsrt/unicase"
	"github.com/gruntjs/grunt-go/js"
)

// stringCodePoints splits s into the strings the string iterator yields: code
// points, with unpaired surrogates on their own.
func stringCodePoints(s string) []any {
	u := jsstr.ToUTF16(s)
	out := make([]any, 0, len(u))
	for i := 0; i < len(u); {
		_, n := jsstr.CodePointAt(u, i)
		out = append(out, jsstr.FromUTF16(u[i:i+n]))
		i += n
	}
	return out
}

// thisString coerces the receiver of a String.prototype method.
func (r *Realm) thisString(this any, method string) (string, error) {
	if js.IsNullish(this) {
		return "", js.NewTypeError("String.prototype." + method + " called on null or undefined")
	}
	return r.toString(this)
}

// intArg is ToIntegerOrInfinity(args[i]), with def for undefined.
func (r *Realm) intArg(args []any, i int, def float64) (float64, error) {
	v := js.Arg(args, i)
	if isUndefined(v) {
		return def, nil
	}
	n, err := r.toNumber(v)
	if err != nil {
		return 0, err
	}
	return js.ToIntegerOrInfinity(n), nil
}

func (r *Realm) setupString() {
	sp := r.stringProto
	toStr := func(args []any) (any, error) {
		if len(args) == 0 {
			return "", nil
		}
		return r.toString(args[0])
	}
	str := r.constructor("String", 1, sp, func(_ any, args []any) (any, error) { return toStr(args) }, toStr)
	r.method(str, "fromCharCode", 1, func(_ any, args []any) (any, error) {
		u := make([]uint16, len(args))
		for i, a := range args {
			n, err := r.toNumber(a)
			if err != nil {
				return nil, err
			}
			u[i] = uint16(js.ToUint32(n))
		}
		return jsstr.FromUTF16(u), nil
	})
	r.method(str, "fromCodePoint", 1, func(_ any, args []any) (any, error) {
		var u []uint16
		for _, a := range args {
			n, err := r.toNumber(a)
			if err != nil {
				return nil, err
			}
			if n != math.Trunc(n) || n < 0 || n > 0x10FFFF {
				s, err := r.toString(a)
				if err != nil {
					return nil, err
				}
				return nil, js.NewRangeError("Invalid code point " + s)
			}
			u = jsstr.AppendCodePoint(u, rune(n))
		}
		return jsstr.FromUTF16(u), nil
	})
	r.method(str, "raw", 1, func(_ any, args []any) (any, error) {
		strs, err := r.getV(js.Arg(args, 0), "raw")
		if err != nil {
			return nil, err
		}
		if js.IsNullish(strs) {
			return nil, js.NewTypeError("Cannot convert undefined or null to object")
		}
		parts, err := r.arrayLike(strs)
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		for i, p := range parts {
			s, err := r.toString(p)
			if err != nil {
				return nil, err
			}
			b.WriteString(s)
			if i+1 < len(parts) && i+1 < len(args) {
				s, err := r.toString(args[i+1])
				if err != nil {
					return nil, err
				}
				b.WriteString(s)
			}
		}
		return b.String(), nil
	})

	thisValue := func(this any, method string) (any, error) {
		s, ok := this.(string)
		if !ok {
			return nil, js.NewTypeError("String.prototype." + method + " requires that 'this' be a String")
		}
		return s, nil
	}
	r.method(sp, "toString", 0, func(this any, _ []any) (any, error) { return thisValue(this, "toString") })
	r.method(sp, "valueOf", 0, func(this any, _ []any) (any, error) { return thisValue(this, "valueOf") })

	// m installs a method that receives the coerced receiver as UTF-16.
	m := func(name string, length int, fn func(s string, u []uint16, args []any) (any, error)) {
		r.method(sp, name, length, func(this any, args []any) (any, error) {
			s, err := r.thisString(this, name)
			if err != nil {
				return nil, err
			}
			return fn(s, jsstr.ToUTF16(s), args)
		})
	}
	m("charAt", 1, func(_ string, u []uint16, args []any) (any, error) {
		i, err := r.intArg(args, 0, 0)
		if err != nil || i < 0 || i >= float64(len(u)) {
			return "", err
		}
		return jsstr.FromUTF16(u[int(i) : int(i)+1]), nil
	})
	m("charCodeAt", 1, func(_ string, u []uint16, args []any) (any, error) {
		i, err := r.intArg(args, 0, 0)
		if err != nil || i < 0 || i >= float64(len(u)) {
			return math.NaN(), err
		}
		return float64(u[int(i)]), nil
	})
	m("codePointAt", 1, func(_ string, u []uint16, args []any) (any, error) {
		i, err := r.intArg(args, 0, 0)
		if err != nil || i < 0 || i >= float64(len(u)) {
			return js.Undefined, err
		}
		cp, _ := jsstr.CodePointAt(u, int(i))
		return float64(cp), nil
	})
	m("at", 1, func(_ string, u []uint16, args []any) (any, error) {
		i, err := r.intArg(args, 0, 0)
		if err != nil {
			return nil, err
		}
		if i < 0 {
			i += float64(len(u))
		}
		if i < 0 || i >= float64(len(u)) {
			return js.Undefined, nil
		}
		return jsstr.FromUTF16(u[int(i) : int(i)+1]), nil
	})
	m("concat", 1, func(s string, _ []uint16, args []any) (any, error) {
		var b strings.Builder
		b.WriteString(s)
		for _, a := range args {
			t, err := r.toString(a)
			if err != nil {
				return nil, err
			}
			b.WriteString(t)
		}
		return b.String(), nil
	})
	// searchArg converts the search string of includes/startsWith/endsWith, which
	// refuse regular expressions.
	searchArg := func(name string, args []any) ([]uint16, error) {
		if _, isRE := js.Arg(args, 0).(*js.RegExp); isRE {
			return nil, js.NewTypeError("First argument to String.prototype." + name + " must not be a regular expression")
		}
		t, err := r.toString(js.Arg(args, 0))
		return jsstr.ToUTF16(t), err
	}
	m("includes", 1, func(_ string, u []uint16, args []any) (any, error) {
		search, err := searchArg("includes", args)
		if err != nil {
			return nil, err
		}
		pos, err := r.intArg(args, 1, 0)
		if err != nil {
			return nil, err
		}
		return jsstr.Index(u, search, js.RelativeIndex(math.Max(pos, 0), len(u))) >= 0, nil
	})
	m("startsWith", 1, func(_ string, u []uint16, args []any) (any, error) {
		search, err := searchArg("startsWith", args)
		if err != nil {
			return nil, err
		}
		pos, err := r.intArg(args, 1, 0)
		if err != nil {
			return nil, err
		}
		start := js.RelativeIndex(math.Max(pos, 0), len(u))
		return start+len(search) <= len(u) && slices.Equal(u[start:start+len(search)], search), nil
	})
	m("endsWith", 1, func(_ string, u []uint16, args []any) (any, error) {
		search, err := searchArg("endsWith", args)
		if err != nil {
			return nil, err
		}
		end, err := r.intArg(args, 1, float64(len(u)))
		if err != nil {
			return nil, err
		}
		e := js.RelativeIndex(math.Max(end, 0), len(u))
		start := e - len(search)
		return start >= 0 && slices.Equal(u[start:e], search), nil
	})
	m("indexOf", 1, func(_ string, u []uint16, args []any) (any, error) {
		t, err := r.toString(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		pos, err := r.intArg(args, 1, 0)
		if err != nil {
			return nil, err
		}
		return float64(jsstr.Index(u, jsstr.ToUTF16(t), js.RelativeIndex(math.Max(pos, 0), len(u)))), nil
	})
	m("lastIndexOf", 1, func(_ string, u []uint16, args []any) (any, error) {
		t, err := r.toString(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		search := jsstr.ToUTF16(t)
		n, err := r.toNumber(js.Arg(args, 1))
		if err != nil {
			return nil, err
		}
		pos := math.Inf(1)
		if !math.IsNaN(n) {
			pos = js.ToIntegerOrInfinity(n)
		}
		return float64(jsstr.LastIndex(u, search, js.RelativeIndex(math.Max(pos, 0), len(u)))), nil
	})
	pad := func(atStart bool) func(string, []uint16, []any) (any, error) {
		return func(s string, u []uint16, args []any) (any, error) {
			maxLen, err := r.intArg(args, 0, 0)
			if err != nil {
				return nil, err
			}
			if maxLen <= float64(len(u)) {
				return s, nil
			}
			fill := " "
			if f := js.Arg(args, 1); !isUndefined(f) {
				if fill, err = r.toString(f); err != nil {
					return nil, err
				}
			}
			if fill == "" {
				return s, nil
			}
			if maxLen > js.MaxStringLength {
				return nil, js.InvalidStringLength()
			}
			fu := jsstr.ToUTF16(fill)
			need := int(maxLen) - len(u)
			filler := make([]uint16, 0, need)
			for len(filler) < need {
				filler = append(filler, fu[:min(len(fu), need-len(filler))]...)
			}
			if atStart {
				return jsstr.FromUTF16(filler) + s, nil
			}
			return s + jsstr.FromUTF16(filler), nil
		}
	}
	m("padStart", 1, pad(true))
	m("padEnd", 1, pad(false))
	m("repeat", 1, func(s string, _ []uint16, args []any) (any, error) {
		n, err := r.toNumber(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		count := js.ToIntegerOrInfinity(n)
		if count < 0 || math.IsInf(count, 1) {
			return nil, js.NewRangeError("Invalid count value: " + js.NumberToString(count))
		}
		if s == "" || count == 0 {
			return "", nil
		}
		if float64(jsstr.Length(s))*count > js.MaxStringLength {
			return nil, js.InvalidStringLength()
		}
		return strings.Repeat(s, int(count)), nil
	})
	m("slice", 2, func(_ string, u []uint16, args []any) (any, error) {
		start, err := r.intArg(args, 0, 0)
		if err != nil {
			return nil, err
		}
		end, err := r.intArg(args, 1, float64(len(u)))
		if err != nil {
			return nil, err
		}
		from, to := js.RelativeIndex(start, len(u)), js.RelativeIndex(end, len(u))
		if from >= to {
			return "", nil
		}
		return jsstr.FromUTF16(u[from:to]), nil
	})
	m("substring", 2, func(_ string, u []uint16, args []any) (any, error) {
		start, err := r.intArg(args, 0, 0)
		if err != nil {
			return nil, err
		}
		end, err := r.intArg(args, 1, float64(len(u)))
		if err != nil {
			return nil, err
		}
		from := js.RelativeIndex(math.Max(start, 0), len(u))
		to := js.RelativeIndex(math.Max(end, 0), len(u))
		if from > to {
			from, to = to, from
		}
		return jsstr.FromUTF16(u[from:to]), nil
	})
	m("substr", 2, func(_ string, u []uint16, args []any) (any, error) {
		start, err := r.intArg(args, 0, 0)
		if err != nil {
			return nil, err
		}
		length, err := r.intArg(args, 1, math.Inf(1))
		if err != nil {
			return nil, err
		}
		from := js.RelativeIndex(start, len(u))
		n := math.Min(math.Max(length, 0), float64(len(u)-from))
		if n <= 0 {
			return "", nil
		}
		return jsstr.FromUTF16(u[from : from+int(n)]), nil
	})
	lower := func(_ string, u []uint16, _ []any) (any, error) { return jsstr.FromUTF16(unicase.ToLower(u)), nil }
	upper := func(_ string, u []uint16, _ []any) (any, error) { return jsstr.FromUTF16(unicase.ToUpper(u)), nil }
	m("toLowerCase", 0, lower)
	m("toLocaleLowerCase", 0, lower)
	m("toUpperCase", 0, upper)
	m("toLocaleUpperCase", 0, upper)
	trim := func(left, right bool) func(string, []uint16, []any) (any, error) {
		return func(s string, _ []uint16, _ []any) (any, error) {
			if left {
				s = strings.TrimLeftFunc(s, js.IsSpace)
			}
			if right {
				s = strings.TrimRightFunc(s, js.IsSpace)
			}
			return s, nil
		}
	}
	m("trim", 0, trim(true, true))
	trimStart := trim(true, false)
	trimEnd := trim(false, true)
	m("trimStart", 0, trimStart)
	m("trimEnd", 0, trimEnd)
	// trimLeft and trimRight are the same function objects as trimStart/trimEnd.
	r.define(sp, "trimLeft", sp.Get("trimStart"))
	r.define(sp, "trimRight", sp.Get("trimEnd"))
	m("split", 2, func(s string, u []uint16, args []any) (any, error) {
		return r.split(s, u, args)
	})
	m("match", 1, func(s string, _ []uint16, args []any) (any, error) {
		return r.stringMatch(s, js.Arg(args, 0))
	})
	m("matchAll", 1, func(s string, _ []uint16, args []any) (any, error) {
		return r.stringMatchAll(s, js.Arg(args, 0))
	})
	m("search", 1, func(s string, _ []uint16, args []any) (any, error) {
		return r.stringSearch(s, js.Arg(args, 0))
	})
	m("replace", 2, func(s string, _ []uint16, args []any) (any, error) {
		return r.stringReplace(s, js.Arg(args, 0), js.Arg(args, 1), false)
	})
	m("replaceAll", 2, func(s string, _ []uint16, args []any) (any, error) {
		return r.stringReplace(s, js.Arg(args, 0), js.Arg(args, 1), true)
	})
}

// split is String.prototype.split for a string or regular expression separator.
func (r *Realm) split(s string, u []uint16, args []any) (any, error) {
	sep, limitArg := js.Arg(args, 0), js.Arg(args, 1)
	if re, ok := sep.(*js.RegExp); ok {
		return r.regexpSplit(re, s, limitArg)
	}
	limit := uint32(math.MaxUint32)
	if !isUndefined(limitArg) {
		n, err := r.toNumber(limitArg)
		if err != nil {
			return nil, err
		}
		limit = js.ToUint32(n)
	}
	if isUndefined(sep) {
		if limit == 0 {
			return js.NewArray(), nil
		}
		return js.NewArray(s), nil
	}
	sepStr, err := r.toString(sep)
	if err != nil {
		return nil, err
	}
	out := js.NewArray()
	for _, piece := range jsstr.Split(s, sepStr, limit) {
		out.Items = append(out.Items, piece)
	}
	return out, nil
}
