package jsregexp

import (
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// The parts of the String.prototype methods that take a regular expression
// (replace, split, matchAll) that do not depend on the caller's object model; both
// the template runtime and the lodash helpers build on them.

// AdvanceStringIndex is AdvanceStringIndex(s, i, fullUnicode): the index after i,
// past a whole surrogate pair when the expression has the u or v flag.
func (re *Regexp) AdvanceStringIndex(s []uint16, i int) int {
	if (re.Unicode() || re.UnicodeSets()) && i+1 < len(s) {
		_, n := jsstr.CodePointAt(s, i)
		return i + n
	}
	return i + 1
}

// Captures returns the strings of a match's groups, from the capture positions caps
// that Exec returns over units: js.Undefined for a group that did not participate.
func Captures(units []uint16, caps []int) []any {
	out := make([]any, len(caps)/2)
	for i := range out {
		if caps[2*i] < 0 {
			out[i] = js.Undefined
		} else {
			out[i] = jsstr.FromUTF16(units[caps[2*i]:caps[2*i+1]])
		}
	}
	return out
}

// NamedGroups builds the groups object of a match from its Captures, or returns nil
// when the expression has no named groups. A name used by several alternatives takes
// the value of the group that participated.
func (re *Regexp) NamedGroups(captures []any) *js.Object {
	if !re.HasNamedGroups() {
		return nil
	}
	o := js.NewObject()
	for i, name := range re.GroupNames() {
		if name == "" {
			continue
		}
		if v := captures[i]; v != js.Undefined || !o.Has(name) {
			o.Set(name, v)
		}
	}
	return o
}

// Substitute is GetSubstitution: the replacement template with $$, $&, $`, $', $n,
// $nn and $<name> expanded for the match at caps (capture positions in subject).
// groups is the match's groups object, nil when the expression has no named groups
// (then $< stays literal).
func Substitute(template, subject []uint16, caps []int, groups *js.Object) []uint16 {
	ncaps := len(caps)/2 - 1
	capture := func(n int) []uint16 {
		if caps[2*n] < 0 {
			return nil
		}
		return subject[caps[2*n]:caps[2*n+1]]
	}
	var out []uint16
	for i := 0; i < len(template); i++ {
		c := template[i]
		if c != '$' || i+1 >= len(template) {
			out = append(out, c)
			continue
		}
		switch n := template[i+1]; {
		case n == '$':
			out = append(out, '$')
			i++
		case n == '&':
			out = append(out, subject[caps[0]:caps[1]]...)
			i++
		case n == '`':
			out = append(out, subject[:caps[0]]...)
			i++
		case n == '\'':
			out = append(out, subject[min(caps[1], len(subject)):]...)
			i++
		case n >= '0' && n <= '9':
			d := int(n - '0')
			if i+2 < len(template) && template[i+2] >= '0' && template[i+2] <= '9' {
				if dd := d*10 + int(template[i+2]-'0'); dd >= 1 && dd <= ncaps {
					out = append(out, capture(dd)...)
					i += 2
					continue
				}
			}
			if d >= 1 && d <= ncaps {
				out = append(out, capture(d)...)
				i++
				continue
			}
			out = append(out, '$')
		case n == '<' && groups != nil:
			end := -1
			for j := i + 2; j < len(template); j++ {
				if template[j] == '>' {
					end = j
					break
				}
			}
			if end < 0 {
				out = append(out, '$')
				continue
			}
			if s, ok := groups.Get(jsstr.FromUTF16(template[i+2 : end])).(string); ok {
				out = append(out, jsstr.ToUTF16(s)...)
			}
			i = end
		default:
			out = append(out, '$')
		}
	}
	return out
}

// Split is RegExp.prototype[@@split] once the splitter exists: sticky is the
// expression compiled with the y flag added, matched at each position of s. The
// result holds at most limit items: the pieces between matches and, after each
// piece, the match's captures (js.Undefined for a group that did not participate).
func Split(sticky *Regexp, s string, limit uint32) []any {
	out := []any{}
	if limit == 0 {
		return out
	}
	u := jsstr.ToUTF16(s)
	if len(u) == 0 {
		if sticky.Exec(u, 0) == nil {
			out = append(out, s)
		}
		return out
	}
	p := 0
	for q := p; q < len(u); {
		caps := sticky.Exec(u, q)
		if caps == nil {
			q = sticky.AdvanceStringIndex(u, q)
			continue
		}
		e := min(caps[1], len(u))
		if e == p {
			q = sticky.AdvanceStringIndex(u, q)
			continue
		}
		out = append(out, jsstr.FromUTF16(u[p:q]))
		if uint32(len(out)) == limit {
			return out
		}
		for _, c := range Captures(u, caps)[1:] {
			out = append(out, c)
			if uint32(len(out)) == limit {
				return out
			}
		}
		p = e
		q = p
	}
	return append(out, jsstr.FromUTF16(u[p:]))
}
