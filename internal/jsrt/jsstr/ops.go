package jsstr

import (
	"slices"
	"unicode/utf16"
)

// FromUTF16WellFormed returns the string made of the code units u with every
// unpaired surrogate replaced by U+FFFD: what writing the JavaScript string as UTF-8
// (to a stream, a file or a Buffer) produces. It is ToWellFormed(FromUTF16(u)).
func FromUTF16WellFormed(u []uint16) string {
	return string(utf16.Decode(u))
}

// AppendCodePoint appends the code point r to u as one code unit, or as a
// surrogate pair above U+FFFF. Unlike utf16.AppendRune it keeps a surrogate code
// point as the code unit it is.
func AppendCodePoint(u []uint16, r rune) []uint16 {
	if r >= 0x10000 {
		hi, lo := utf16.EncodeRune(r)
		return append(u, uint16(hi), uint16(lo))
	}
	return append(u, uint16(r))
}

// DigitValue is the value of the ASCII digit or letter c as a digit in radix 36
// (0-9, then a or A as 10 up to z or Z as 35), or -1 for any other character.
func DigitValue(c rune) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	}
	return -1
}

// HexDigitValue is the value of the hexadecimal digit c (0-9, a-f or A-F), or -1
// for any other character.
func HexDigitValue(c rune) int {
	if d := DigitValue(c); d < 16 {
		return d
	}
	return -1
}

// Compare orders two strings by their UTF-16 code units, as JavaScript's relational
// operators and the default Array.prototype.sort do (Go's byte order differs for
// characters outside the Basic Multilingual Plane). It returns -1, 0 or 1.
func Compare(a, b string) int {
	if IsASCII(a) && IsASCII(b) {
		switch {
		case a < b:
			return -1
		case a > b:
			return 1
		}
		return 0
	}
	return slices.Compare(ToUTF16(a), ToUTF16(b))
}

// Index is String.prototype.indexOf over code units: the first index at or after
// from (clamped to the string) where sub occurs in s, or -1.
func Index(s, sub []uint16, from int) int {
	for i := max(0, min(from, len(s))); i+len(sub) <= len(s); i++ {
		if slices.Equal(s[i:i+len(sub)], sub) {
			return i
		}
	}
	return -1
}

// LastIndex is String.prototype.lastIndexOf over code units: the last index at or
// before from where sub occurs in s, or -1.
func LastIndex(s, sub []uint16, from int) int {
	for i := max(0, min(from, len(s)-len(sub))); i >= 0; i-- {
		if i+len(sub) <= len(s) && slices.Equal(s[i:i+len(sub)], sub) {
			return i
		}
	}
	return -1
}

// Split is String.prototype.split(sep, limit) with a string separator: at most
// limit pieces of s, or each code unit for an empty separator.
func Split(s, sep string, limit uint32) []string {
	out := []string{}
	if limit == 0 {
		return out
	}
	units, sepUnits := ToUTF16(s), ToUTF16(sep)
	if len(sepUnits) == 0 {
		for i := 0; i < len(units) && uint32(i) < limit; i++ {
			out = append(out, FromUTF16(units[i:i+1]))
		}
		return out
	}
	p := 0
	for {
		q := Index(units, sepUnits, p)
		if q < 0 {
			break
		}
		out = append(out, FromUTF16(units[p:q]))
		if uint32(len(out)) == limit {
			return out
		}
		p = q + len(sepUnits)
	}
	return append(out, FromUTF16(units[p:]))
}

// IsWhiteSpace reports whether r is ECMAScript WhiteSpace: tab, vertical tab, form
// feed, the byte order mark and the space separators (Zs).
func IsWhiteSpace(r rune) bool {
	switch r {
	case '\t', '\v', '\f', ' ', 0xa0, 0x1680, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return 0x2000 <= r && r <= 0x200a
}

// IsLineTerminator reports whether r is an ECMAScript LineTerminator.
func IsLineTerminator(r rune) bool {
	return r == '\n' || r == '\r' || r == 0x2028 || r == 0x2029
}
