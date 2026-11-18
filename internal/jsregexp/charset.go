package jsregexp

import (
	"slices"
	"sort"

	"github.com/gruntjs/grunt-go/internal/jsrt/unicase"
)

// charSet is a set of characters stored as sorted, disjoint, non-adjacent
// inclusive ranges flattened into lo, hi pairs.
type charSet []rune

// maxChar returns the largest character of the matching mode: a code unit
// without u or v, a code point with them.
func maxChar(unicodeMode bool) rune {
	if unicodeMode {
		return 0x10FFFF
	}
	return 0xFFFF
}

// newCharSet builds a normalized set from lo, hi pairs in any order.
func newCharSet(pairs ...rune) charSet {
	type rng struct{ lo, hi rune }
	rs := make([]rng, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		rs = append(rs, rng{pairs[i], pairs[i+1]})
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].lo < rs[j].lo })
	var out charSet
	for _, r := range rs {
		if n := len(out); n > 0 && r.lo <= out[n-1]+1 {
			out[n-1] = max(out[n-1], r.hi)
			continue
		}
		out = append(out, r.lo, r.hi)
	}
	return out
}

// has reports whether c is in the set.
func (s charSet) has(c rune) bool {
	n := len(s) / 2
	i := sort.Search(n, func(i int) bool { return s[2*i+1] >= c })
	return i < n && s[2*i] <= c
}

// union returns s ∪ t.
func (s charSet) union(t charSet) charSet {
	return newCharSet(append(slices.Clone(s), t...)...)
}

// complement returns the characters up to limit that are not in s.
func (s charSet) complement(limit rune) charSet {
	var out charSet
	next := rune(0)
	for i := 0; i < len(s); i += 2 {
		if s[i] > next {
			out = append(out, next, s[i]-1)
		}
		next = s[i+1] + 1
	}
	if next <= limit {
		out = append(out, next, limit)
	}
	return out
}

// intersect returns s ∩ t.
func (s charSet) intersect(t charSet) charSet {
	var out charSet
	i, j := 0, 0
	for i < len(s) && j < len(t) {
		lo, hi := max(s[i], t[j]), min(s[i+1], t[j+1])
		if lo <= hi {
			out = append(out, lo, hi)
		}
		if s[i+1] < t[j+1] {
			i += 2
		} else {
			j += 2
		}
	}
	return out
}

// minus returns s \ t.
func (s charSet) minus(t charSet, limit rune) charSet {
	return s.intersect(t.complement(limit))
}

// caseClosure adds to s the canonical form of each of its members, so that a
// character ch matches the set case-insensitively exactly when
// Canonicalize(ch) is in the result. It relies on Canonicalize being idempotent.
func (s charSet) caseClosure(unicodeMode bool) charSet {
	var extra []rune
	for _, c := range unicase.CaseVariants(unicodeMode) {
		if s.has(c) {
			k := unicase.Canonicalize(c, unicodeMode)
			extra = append(extra, k, k)
		}
	}
	if len(extra) == 0 {
		return s
	}
	return s.union(newCharSet(extra...))
}

// fold maps every member of s to its canonical form, MaybeSimpleCaseFolding of
// the v flag.
func (s charSet) fold() charSet {
	var out []rune
	vs := unicase.CaseVariants(true)
	for i := 0; i < len(s); i += 2 {
		out = append(out, s[i], s[i+1])
	}
	removed := make([]rune, 0)
	for _, c := range vs {
		if s.has(c) {
			k := unicase.Canonicalize(c, true)
			out = append(out, k, k)
			removed = append(removed, c, c)
		}
	}
	return newCharSet(out...).minus(newCharSet(removed...), 0x10FFFF)
}

// canonicalUniverse is AllCharacters for the v and i flags together: the code
// points that are their own canonical form.
func canonicalUniverse() charSet {
	var vs []rune
	for _, c := range unicase.CaseVariants(true) {
		vs = append(vs, c, c)
	}
	return newCharSet(vs...).complement(0x10FFFF)
}

var (
	digitSet = newCharSet('0', '9')
	wordSet  = newCharSet('0', '9', 'A', 'Z', '_', '_', 'a', 'z')
	spaceSet = newCharSet(0x9, 0xD, 0x20, 0x20, 0xA0, 0xA0, 0x1680, 0x1680, 0x2000, 0x200A,
		0x2028, 0x2029, 0x202F, 0x202F, 0x205F, 0x205F, 0x3000, 0x3000, 0xFEFF, 0xFEFF)
)

// wordCharacters is WordCharacters(rer): with u or v and i together it also holds
// every character whose canonical form is a basic word character (U+017F and
// U+212A).
func wordCharacters(unicodeMode, ignoreCase bool) charSet {
	if !unicodeMode || !ignoreCase {
		return wordSet
	}
	var extra []rune
	for _, c := range unicase.CaseVariants(true) {
		if wordSet.has(unicase.Canonicalize(c, true)) {
			extra = append(extra, c, c)
		}
	}
	return wordSet.union(newCharSet(extra...))
}
