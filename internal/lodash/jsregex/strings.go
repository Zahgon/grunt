package jsregex

import (
	"github.com/gruntjs/grunt-go/internal/jsregexp"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// Replace is String.prototype.replace(s, re, replacement): every match when re is
// global, otherwise the first. replacement is a *js.Function called as the original
// calls it (match, p1..pn, offset, string[, groups]) or any other value, whose
// string form is a replacement template with $$, $&, $`, $', $n, $nn and $<name>.
func (re *Regexp) Replace(s string, replacement any) (string, error) {
	fn, isFn := replacement.(*js.Function)
	var template []uint16
	if !isFn {
		template = jsstr.ToUTF16(js.ToString(replacement))
	}
	units := jsstr.ToUTF16(s)
	out, err := re.ReplaceFunc(units, func(caps []int) ([]uint16, error) {
		captures := jsregexp.Captures(units, caps)
		groups := re.NamedGroups(captures)
		if !isFn {
			return jsregexp.Substitute(template, units, caps, groups), nil
		}
		args := append(captures, float64(caps[0]), s)
		if groups != nil {
			args = append(args, groups)
		}
		v, err := fn.Call(js.Undefined, args)
		if err != nil {
			return nil, err
		}
		return jsstr.ToUTF16(js.ToString(v)), nil
	})
	if err != nil {
		return "", err
	}
	return jsstr.FromUTF16(out), nil
}

// ReplaceFunc is the matching loop of Replace with the replacement computed in Go:
// f receives the capture positions of each match (every match when re is global,
// otherwise the first) and returns the code units to put in its place.
func (re *Regexp) ReplaceFunc(units []uint16, f func(caps []int) ([]uint16, error)) ([]uint16, error) {
	var out []uint16
	last := 0
	for from := 0; from <= len(units); {
		caps := re.Exec(units, from)
		if caps == nil {
			break
		}
		rep, err := f(caps)
		if err != nil {
			return nil, err
		}
		out = append(out, units[last:caps[0]]...)
		out = append(out, rep...)
		last = caps[1]
		if !re.Global() {
			break
		}
		from = caps[1]
		if caps[1] == caps[0] {
			from = re.AdvanceStringIndex(units, from)
		}
	}
	return append(out, units[last:]...), nil
}

// ReplaceString is String.prototype.replace with a string pattern: only the first
// occurrence is replaced.
func ReplaceString(s, pattern string, replacement any) (string, error) {
	units, pat := jsstr.ToUTF16(s), jsstr.ToUTF16(pattern)
	start := jsstr.Index(units, pat, 0)
	fn, isFn := replacement.(*js.Function)
	var template []uint16
	if !isFn {
		template = jsstr.ToUTF16(js.ToString(replacement))
	}
	if start < 0 {
		return s, nil
	}
	end := start + len(pat)
	var rep []uint16
	if isFn {
		v, err := fn.Call(js.Undefined, []any{pattern, float64(start), s})
		if err != nil {
			return "", err
		}
		rep = jsstr.ToUTF16(js.ToString(v))
	} else {
		rep = jsregexp.Substitute(template, units, []int{start, end}, nil)
	}
	out := append(append(append([]uint16(nil), units[:start]...), rep...), units[end:]...)
	return jsstr.FromUTF16(out), nil
}

// Split is String.prototype.split(s, re, limit) (RegExp.prototype[@@split]); items
// are strings, or Undefined for captures that did not participate.
func (re *Regexp) Split(s string, limit uint32) []any {
	return jsregexp.Split(re.sticky, s, limit)
}

// MatchAll is String.prototype.match with a global regexp: every match string, or
// nil (JavaScript null) when there is none.
func (re *Regexp) MatchAll(s string) []string {
	units := jsstr.ToUTF16(s)
	var out []string
	for from := 0; from <= len(units); {
		caps := re.Exec(units, from)
		if caps == nil {
			break
		}
		out = append(out, jsstr.FromUTF16(units[caps[0]:caps[1]]))
		from = caps[1]
		if caps[1] == caps[0] {
			from = re.AdvanceStringIndex(units, from)
		}
	}
	return out
}
