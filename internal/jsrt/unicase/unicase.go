// Package unicase implements JavaScript's locale-independent case conversion
// (String.prototype.toUpperCase and toLowerCase as Node 24 performs them through
// ICU) and the Canonicalize operation that case-insensitive regular expressions
// use.
//
// Strings are handled as UTF-16 code units: surrogate pairs are converted as the
// code point they encode and unpaired surrogates pass through unchanged.
package unicase

import (
	"slices"
	"sync"
	"unicode"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
)

// ToUpper is String.prototype.toUpperCase: the full Unicode uppercase mapping of
// every code point, so "ß" becomes "SS" and the result may be longer than u.
func ToUpper(u []uint16) []uint16 {
	out := make([]uint16, 0, len(u))
	for i := 0; i < len(u); {
		r, n := jsstr.CodePointAt(u, i)
		i += n
		if s, ok := upperSpecial[r]; ok {
			out = appendString(out, s)
			continue
		}
		out = jsstr.AppendCodePoint(out, unicode.ToUpper(r))
	}
	return out
}

// ToLower is String.prototype.toLowerCase: the full Unicode lowercase mapping of
// every code point, with the Final_Sigma rule of SpecialCasing.txt for U+03A3.
func ToLower(u []uint16) []uint16 {
	out := make([]uint16, 0, len(u))
	for i := 0; i < len(u); {
		r, n := jsstr.CodePointAt(u, i)
		switch {
		case r == 0x3A3 && finalSigma(u, i, i+n):
			out = append(out, 0x3C2)
		case lowerSpecial[r] != "":
			out = appendString(out, lowerSpecial[r])
		default:
			out = jsstr.AppendCodePoint(out, unicode.ToLower(r))
		}
		i += n
	}
	return out
}

// finalSigma reports whether the capital sigma occupying u[start:end] is in the
// Final_Sigma context: preceded by a cased letter and any case-ignorable
// characters, and not followed by case-ignorable characters and a cased letter.
func finalSigma(u []uint16, start, end int) bool {
	before := false
	for i := start; i > 0; {
		r, n := jsstr.CodePointBefore(u, i)
		i -= n
		if IsCaseIgnorable(r) {
			continue
		}
		before = isCased(r)
		break
	}
	if !before {
		return false
	}
	for i := end; i < len(u); {
		r, n := jsstr.CodePointAt(u, i)
		i += n
		if IsCaseIgnorable(r) {
			continue
		}
		return !isCased(r)
	}
	return true
}

// isCased is the Unicode Cased property: Lowercase, Uppercase or Lt.
func isCased(r rune) bool {
	return unicode.In(r, unicode.Lu, unicode.Ll, unicode.Lt, unicode.Other_Lowercase, unicode.Other_Uppercase)
}

// IsCaseIgnorable is the Unicode Case_Ignorable property: Mn, Me, Cf, Lm, Sk and
// the Word_Break values MidLetter, MidNumLet and Single_Quote.
func IsCaseIgnorable(r rune) bool {
	switch r {
	case 0x27, 0x2E, 0x3A, 0xB7, 0x387, 0x55F, 0x5F4, 0x2018, 0x2019, 0x2024, 0x2027,
		0xFE13, 0xFE52, 0xFE55, 0xFF07, 0xFF0E, 0xFF1A:
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk)
}

func appendString(out []uint16, s string) []uint16 {
	for _, r := range s {
		out = jsstr.AppendCodePoint(out, r)
	}
	return out
}

var (
	canonOnce  sync.Once
	canonTable []uint16
)

// Canonicalize is the Canonicalize abstract operation of ECMA-262 (22.2.2.7.3)
// that case-insensitive matching applies to both the pattern and the input.
//
// With unicode set (the u and v flags) it is simple case folding; here every code
// point maps to the smallest member of its case-folding orbit, which yields the
// same equivalence classes as CaseFolding.txt's simple and common mappings.
// Without it c is a UTF-16 code unit and maps to its toUpperCase result, unless
// that result is not a single code unit or would turn a non-ASCII character into
// an ASCII one.
func Canonicalize(c rune, unicodeMode bool) rune {
	if unicodeMode {
		least := c
		for f := unicode.SimpleFold(c); f != c; f = unicode.SimpleFold(f) {
			least = min(least, f)
		}
		return least
	}
	if c < 0x80 {
		if c >= 'a' && c <= 'z' {
			return c - 'a' + 'A'
		}
		return c
	}
	if c > 0xFFFF {
		return c
	}
	canonOnce.Do(buildCanonTable)
	return rune(canonTable[c])
}

// buildCanonTable precomputes the non-unicode Canonicalize of every code unit.
func buildCanonTable() {
	canonTable = make([]uint16, 0x10000)
	for c := range rune(0x10000) {
		canonTable[c] = uint16(c)
		if c >= 0xD800 && c < 0xE000 {
			continue
		}
		if _, ok := upperSpecial[c]; ok {
			// A full mapping is longer than one code unit, so c is kept.
			continue
		}
		u := unicode.ToUpper(c)
		if u > 0xFFFF || (c >= 0x80 && u < 0x80) {
			continue
		}
		canonTable[c] = uint16(u)
	}
}

var (
	variantsOnce sync.Once
	variants     [2][]rune
)

// CaseVariants returns, in ascending order, every character c for which
// Canonicalize(c, unicodeMode) != c. All other characters are their own canonical
// form, so a set of characters is closed under Canonicalize once the images of its
// members in this list are added. The slice is shared and must not be modified.
func CaseVariants(unicodeMode bool) []rune {
	variantsOnce.Do(func() {
		// Only characters with a case mapping or a case-folding orbit can
		// canonicalize to another character: those in unicode.CaseRanges and
		// the cased letters, which include the few orbit members without a
		// mapping of their own (U+0390 folds with U+1FD3).
		candidates := map[rune]bool{}
		for _, r := range unicode.CaseRanges {
			for c := rune(r.Lo); c <= rune(r.Hi); c++ {
				candidates[c] = true
			}
		}
		for _, t := range []*unicode.RangeTable{unicode.Lu, unicode.Ll, unicode.Lt} {
			for _, r := range t.R16 {
				for c := rune(r.Lo); c <= rune(r.Hi); c += rune(r.Stride) {
					candidates[c] = true
				}
			}
			for _, r := range t.R32 {
				for c := rune(r.Lo); c <= rune(r.Hi); c += rune(r.Stride) {
					candidates[c] = true
				}
			}
		}
		for c := range candidates {
			if Canonicalize(c, false) != c {
				variants[0] = append(variants[0], c)
			}
			if Canonicalize(c, true) != c {
				variants[1] = append(variants[1], c)
			}
		}
		slices.Sort(variants[0])
		slices.Sort(variants[1])
	})
	if unicodeMode {
		return variants[1]
	}
	return variants[0]
}
