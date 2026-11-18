package jsrt

import (
	"math"
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsregexp"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// regexpState is what a *js.RegExp needs beyond its source and flags: the
// compiled matcher (and a sticky twin for split) and the lastIndex property.
type regexpState struct {
	re        *jsregexp.Regexp
	sticky    *jsregexp.Regexp
	err       error
	lastIndex any
}

// regexpState returns the state of x, compiling it on first use. A *js.RegExp built
// by Go code that does not compile keeps its SyntaxError for the first match.
func (r *Realm) regexpState(x *js.RegExp) *regexpState {
	st := r.regexps[x]
	if st == nil {
		st = &regexpState{lastIndex: 0.0}
		st.re, st.err = jsregexp.Compile(x.Source, x.Flags)
		r.regexps[x] = st
	}
	return st
}

// compiled returns the matcher of x or its compile error.
func (r *Realm) compiled(x *js.RegExp) (*jsregexp.Regexp, error) {
	st := r.regexpState(x)
	return st.re, st.err
}

// newRegExp creates a RegExp object (new RegExp(pattern, flags)).
func (r *Realm) newRegExp(pattern, flags string) (*js.RegExp, error) {
	re, err := jsregexp.Compile(pattern, flags)
	if err != nil {
		return nil, err
	}
	x := &js.RegExp{Source: pattern, Flags: re.Flags()}
	r.regexps[x] = &regexpState{re: re, lastIndex: 0.0}
	return x, nil
}

// regexpProp returns the RegExp.prototype accessor values (source, flags, global…).
func (r *Realm) regexpProp(x *js.RegExp, key string) (any, bool) {
	re, err := r.compiled(x)
	if err != nil {
		// A value that never compiled is not a RegExp object, so it has none of
		// these properties.
		return nil, false
	}
	switch key {
	case "source":
		return jsregexp.EscapeSource(x.Source), true
	case "flags":
		return re.Flags(), true
	case "global":
		return re.Global(), true
	case "ignoreCase":
		return re.IgnoreCase(), true
	case "multiline":
		return re.Multiline(), true
	case "dotAll":
		return re.DotAll(), true
	case "unicode":
		return re.Unicode(), true
	case "unicodeSets":
		return re.UnicodeSets(), true
	case "sticky":
		return re.Sticky(), true
	case "hasIndices":
		return re.HasIndices(), true
	}
	return nil, false
}

func (r *Realm) setupRegExp() {
	rp := r.regexpProto
	build := func(args []any, isCall bool) (any, error) {
		pattern, flags := js.Arg(args, 0), js.Arg(args, 1)
		if re, ok := pattern.(*js.RegExp); ok {
			if isCall && isUndefined(flags) {
				return re, nil
			}
			if isUndefined(flags) {
				flags = re.Flags
			}
			pattern = re.Source
		}
		p, f := "", ""
		var err error
		if !isUndefined(pattern) {
			if p, err = r.toString(pattern); err != nil {
				return nil, err
			}
		}
		if !isUndefined(flags) {
			if f, err = r.toString(flags); err != nil {
				return nil, err
			}
		}
		return r.newRegExp(p, f)
	}
	r.constructor("RegExp", 2, rp, func(_ any, args []any) (any, error) { return build(args, true) },
		func(args []any) (any, error) { return build(args, false) })
	thisRegExp := func(this any, method string) (*js.RegExp, error) {
		re, ok := this.(*js.RegExp)
		if !ok {
			return nil, js.NewTypeError("Method RegExp.prototype." + method + " called on incompatible receiver " + r.safeString(this))
		}
		return re, nil
	}
	r.method(rp, "exec", 1, func(this any, args []any) (any, error) {
		re, err := thisRegExp(this, "exec")
		if err != nil {
			return nil, err
		}
		s, err := r.toString(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		return r.regexpExec(re, s)
	})
	r.method(rp, "test", 1, func(this any, args []any) (any, error) {
		re, err := thisRegExp(this, "test")
		if err != nil {
			return nil, err
		}
		s, err := r.toString(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		m, err := r.regexpExec(re, s)
		return m != nil, err
	})
	r.method(rp, "toString", 0, func(this any, _ []any) (any, error) {
		if !js.IsObject(this) {
			return nil, js.NewTypeError("RegExp.prototype.toString requires that 'this' be an Object")
		}
		src, _ := r.lookup(this, "source")
		flags, _ := r.lookup(this, "flags")
		s, err := r.toString(src)
		if err != nil {
			return nil, err
		}
		f, err := r.toString(flags)
		if err != nil {
			return nil, err
		}
		return "/" + s + "/" + f, nil
	})
}

// safeString renders a value for an error message without calling user code.
func (r *Realm) safeString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case *js.Array, *js.Object, *Derived:
		return "#<Object>"
	case *js.Function:
		return r.funcSource(x)
	}
	return js.ToString(v)
}

// lastIndexOf reads ToLength(lastIndex).
func (r *Realm) lastIndexOf(st *regexpState) (int, error) {
	n, err := r.toNumber(st.lastIndex)
	if err != nil {
		return 0, err
	}
	l := toLength(n)
	if l > math.MaxInt32 {
		return math.MaxInt32, nil
	}
	return int(l), nil
}

// execRaw is RegExpBuiltinExec returning code-unit offsets (nil for no match) and
// updating lastIndex for global and sticky expressions.
func (r *Realm) execRaw(x *js.RegExp, u []uint16) ([]int, error) {
	re, err := r.compiled(x)
	if err != nil {
		return nil, err
	}
	st := r.regexps[x]
	last, err := r.lastIndexOf(st)
	if err != nil {
		return nil, err
	}
	useLast := re.Global() || re.Sticky()
	if !useLast {
		last = 0
	}
	if last > len(u) {
		if useLast {
			st.lastIndex = 0.0
		}
		return nil, nil
	}
	caps := re.Exec(u, last)
	if caps == nil {
		if useLast {
			st.lastIndex = 0.0
		}
		return nil, nil
	}
	if useLast {
		st.lastIndex = float64(caps[1])
	}
	return caps, nil
}

// matchArray builds the array exec returns: the match, its captures, and the
// index, input and groups properties.
func (r *Realm) matchArray(x *js.RegExp, u []uint16, s string, caps []int) *js.Array {
	// x has just been executed, so it compiled.
	re, _ := r.compiled(x)
	a := &js.Array{Items: jsregexp.Captures(u, caps)}
	props := r.extraProps(a, true)
	props.Set("index", float64(caps[0]))
	props.Set("input", s)
	props.Set("groups", r.groupsObject(re, a.Items))
	return a
}

// groupsObject is the groups property: undefined without named groups, else an
// object mapping each name to its capture (the participating one for duplicate
// names).
func (r *Realm) groupsObject(re *jsregexp.Regexp, items []any) any {
	own := re.NamedGroups(items)
	if own == nil {
		return js.Undefined
	}
	return &Derived{Own: own, Proto: nil}
}

// regexpExec is RegExp.prototype.exec.
func (r *Realm) regexpExec(x *js.RegExp, s string) (any, error) {
	u := jsstr.ToUTF16(s)
	caps, err := r.execRaw(x, u)
	if err != nil || caps == nil {
		return nil, err
	}
	return r.matchArray(x, u, s, caps), nil
}

// toRegExp converts the argument of match, matchAll and search: regular
// expressions are used as they are, anything else becomes new RegExp(value, flags).
func (r *Realm) toRegExp(v any, flags string) (*js.RegExp, error) {
	if re, ok := v.(*js.RegExp); ok {
		return re, nil
	}
	p := ""
	if !isUndefined(v) {
		s, err := r.toString(v)
		if err != nil {
			return nil, err
		}
		p = s
	}
	return r.newRegExp(p, flags)
}

// globalMatches runs a global expression over u from lastIndex 0, as match,
// replace and matchAll do, returning every match's offsets.
func (r *Realm) globalMatches(x *js.RegExp, u []uint16) ([][]int, error) {
	re, err := r.compiled(x)
	if err != nil {
		return nil, err
	}
	st := r.regexps[x]
	st.lastIndex = 0.0
	var all [][]int
	for {
		caps, err := r.execRaw(x, u)
		if err != nil {
			return nil, err
		}
		if caps == nil {
			return all, nil
		}
		all = append(all, caps)
		if caps[0] == caps[1] {
			last, err := r.lastIndexOf(st)
			if err != nil {
				return nil, err
			}
			st.lastIndex = float64(re.AdvanceStringIndex(u, last))
		}
	}
}

func (r *Realm) stringMatch(s string, v any) (any, error) {
	x, err := r.toRegExp(v, "")
	if err != nil {
		return nil, err
	}
	re, err := r.compiled(x)
	if err != nil {
		return nil, err
	}
	if !re.Global() {
		return r.regexpExec(x, s)
	}
	u := jsstr.ToUTF16(s)
	all, err := r.globalMatches(x, u)
	if err != nil || len(all) == 0 {
		return nil, err
	}
	out := &js.Array{Items: make([]any, len(all))}
	for i, caps := range all {
		out.Items[i] = jsstr.FromUTF16(u[caps[0]:caps[1]])
	}
	return out, nil
}

// stringMatchAll is String.prototype.matchAll. The iterator's matches are
// computed when it is created.
func (r *Realm) stringMatchAll(s string, v any) (any, error) {
	if x, ok := v.(*js.RegExp); ok {
		re, err := r.compiled(x)
		if err != nil {
			return nil, err
		}
		if !re.Global() {
			return nil, js.NewTypeError("String.prototype.matchAll called with a non-global RegExp argument")
		}
		// matchAll works on a copy, leaving the argument's lastIndex alone.
		cp, err := r.newRegExp(x.Source, x.Flags)
		if err != nil {
			return nil, err
		}
		r.regexps[cp].lastIndex = r.regexps[x].lastIndex
		v = cp
	} else {
		x, err := r.toRegExp(v, "g")
		if err != nil {
			return nil, err
		}
		v = x
	}
	x := v.(*js.RegExp)
	u := jsstr.ToUTF16(s)
	st := r.regexps[x]
	re := st.re
	out := js.NewArray()
	for {
		caps, err := r.execRaw(x, u)
		if err != nil {
			return nil, err
		}
		if caps == nil {
			return &iterator{items: out.Items, tag: "RegExp String Iterator"}, nil
		}
		out.Items = append(out.Items, r.matchArray(x, u, s, caps))
		if caps[0] == caps[1] {
			last, err := r.lastIndexOf(st)
			if err != nil {
				return nil, err
			}
			st.lastIndex = float64(re.AdvanceStringIndex(u, last))
		}
	}
}

func (r *Realm) stringSearch(s string, v any) (any, error) {
	x, err := r.toRegExp(v, "")
	if err != nil {
		return nil, err
	}
	if _, err := r.compiled(x); err != nil {
		return nil, err
	}
	st := r.regexps[x]
	prev := st.lastIndex
	st.lastIndex = 0.0
	u := jsstr.ToUTF16(s)
	caps, err := r.execRaw(x, u)
	st.lastIndex = prev
	if err != nil {
		return nil, err
	}
	if caps == nil {
		return -1.0, nil
	}
	return float64(caps[0]), nil
}

// match is one match found by replace: offsets into the subject plus the groups
// object for named captures.
type match struct {
	caps   []int
	groups any
}

func (r *Realm) stringReplace(s string, pattern, replacement any, all bool) (any, error) {
	u := jsstr.ToUTF16(s)
	var matches []match
	if x, ok := pattern.(*js.RegExp); ok {
		re, err := r.compiled(x)
		if err != nil {
			return nil, err
		}
		if all && !re.Global() {
			return nil, js.NewTypeError("String.prototype.replaceAll called with a non-global RegExp argument")
		}
		var found [][]int
		if re.Global() {
			if found, err = r.globalMatches(x, u); err != nil {
				return nil, err
			}
		} else {
			caps, err := r.execRaw(x, u)
			if err != nil {
				return nil, err
			}
			if caps != nil {
				found = [][]int{caps}
			}
		}
		for _, caps := range found {
			m := match{caps: caps, groups: js.Undefined}
			if re.HasNamedGroups() {
				m.groups = r.groupsObject(re, jsregexp.Captures(u, caps))
			}
			matches = append(matches, m)
		}
	} else {
		ps, err := r.toString(pattern)
		if err != nil {
			return nil, err
		}
		pu := jsstr.ToUTF16(ps)
		for pos := jsstr.Index(u, pu, 0); pos >= 0; {
			matches = append(matches, match{caps: []int{pos, pos + len(pu)}, groups: js.Undefined})
			next := pos + max(len(pu), 1)
			if !all || next > len(u) {
				break
			}
			pos = jsstr.Index(u, pu, next)
		}
	}
	fn, isFunc := replacement.(*js.Function)
	var template []uint16
	if !isFunc {
		t, err := r.toString(replacement)
		if err != nil {
			return nil, err
		}
		template = jsstr.ToUTF16(t)
	}
	var out []uint16
	next := 0
	for _, m := range matches {
		if m.caps[0] < next {
			continue
		}
		out = append(out, u[next:m.caps[0]]...)
		if isFunc {
			args := append(jsregexp.Captures(u, m.caps), float64(m.caps[0]), s)
			if !isUndefined(m.groups) {
				args = append(args, m.groups)
			}
			v, err := r.call(fn, js.Undefined, args)
			if err != nil {
				return nil, err
			}
			rs, err := r.toString(v)
			if err != nil {
				return nil, err
			}
			out = append(out, jsstr.ToUTF16(rs)...)
		} else {
			var groups *js.Object
			if d, ok := m.groups.(*Derived); ok {
				groups = d.Own
			}
			out = append(out, jsregexp.Substitute(template, u, m.caps, groups)...)
		}
		next = m.caps[1]
	}
	out = append(out, u[next:]...)
	return jsstr.FromUTF16(out), nil
}

// regexpSplit is RegExp.prototype[Symbol.split]: the separator is matched sticky at
// each position.
func (r *Realm) regexpSplit(x *js.RegExp, s string, limitArg any) (any, error) {
	re, err := r.compiled(x)
	if err != nil {
		return nil, err
	}
	st := r.regexps[x]
	if st.sticky == nil {
		flags := re.Flags()
		if !strings.Contains(flags, "y") {
			flags += "y"
		}
		if st.sticky, err = jsregexp.Compile(x.Source, flags); err != nil {
			return nil, err
		}
	}
	limit := uint32(math.MaxUint32)
	if !isUndefined(limitArg) {
		n, err := r.toNumber(limitArg)
		if err != nil {
			return nil, err
		}
		limit = js.ToUint32(n)
	}
	return js.NewArray(jsregexp.Split(st.sticky, s, limit)...), nil
}
