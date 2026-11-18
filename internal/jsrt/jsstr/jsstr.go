// Package jsstr converts between Go strings and the UTF-16 code-unit sequences that
// JavaScript strings are.
//
// A JavaScript string may hold unpaired surrogates, which UTF-8 cannot express. Go
// strings produced by this package therefore use WTF-8: valid UTF-8 everywhere, except
// that an unpaired surrogate is written as its three-byte generalized-UTF-8 form
// (ED A0 80 to ED BF BF). Converting back restores the surrogate, so every JavaScript
// string survives a round trip. Other invalid bytes decode as U+FFFD.
package jsstr

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// ToUTF16 returns the UTF-16 code units of s.
func ToUTF16(s string) []uint16 {
	out := make([]uint16, 0, len(s))
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			out = append(out, uint16(c))
			i++
			continue
		}
		if u, ok := surrogateAt(s, i); ok {
			out = append(out, u)
			i += 3
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		if r >= 0x10000 {
			r1, r2 := utf16.EncodeRune(r)
			out = append(out, uint16(r1), uint16(r2))
		} else {
			out = append(out, uint16(r))
		}
	}
	return out
}

// surrogateAt decodes a WTF-8 encoded surrogate at s[i:].
func surrogateAt(s string, i int) (uint16, bool) {
	if i+2 < len(s) && s[i] == 0xED && s[i+1] >= 0xA0 && s[i+1] <= 0xBF && s[i+2] >= 0x80 && s[i+2] <= 0xBF {
		return 0xD000 | uint16(s[i+1]&0x3F)<<6 | uint16(s[i+2]&0x3F), true
	}
	return 0, false
}

// FromUTF16 returns the string made of the code units u, pairing surrogates and
// writing unpaired ones in WTF-8.
func FromUTF16(u []uint16) string {
	var b strings.Builder
	b.Grow(len(u))
	for i := 0; i < len(u); i++ {
		c := u[i]
		switch {
		case c < utf8.RuneSelf:
			b.WriteByte(byte(c))
		case c >= 0xD800 && c < 0xDC00 && i+1 < len(u) && u[i+1] >= 0xDC00 && u[i+1] < 0xE000:
			b.WriteRune(utf16.DecodeRune(rune(c), rune(u[i+1])))
			i++
		case c >= 0xD800 && c < 0xE000:
			b.WriteByte(0xED)
			b.WriteByte(0x80 | byte(c>>6)&0x3F)
			b.WriteByte(0x80 | byte(c)&0x3F)
		default:
			b.WriteRune(rune(c))
		}
	}
	return b.String()
}

// IsASCII reports whether s holds only ASCII, in which case byte offsets and
// code-unit indexes coincide.
func IsASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// Length is the JavaScript length of s: its number of UTF-16 code units.
func Length(s string) int {
	if IsASCII(s) {
		return len(s)
	}
	return len(ToUTF16(s))
}

// ToWellFormed replaces every unpaired surrogate in s with U+FFFD, which is what
// encoding a JavaScript string as UTF-8 (Buffer.from, fs.writeFileSync) does.
func ToWellFormed(s string) string {
	if IsASCII(s) || !strings.Contains(s, "\xED") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if _, ok := surrogateAt(s, i); ok {
			b.WriteRune(utf8.RuneError)
			i += 3
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// CodePointBefore returns the code point that ends just before index i of u and
// the number of code units it occupies: a surrogate pair is read whole.
func CodePointBefore(u []uint16, i int) (rune, int) {
	c := u[i-1]
	if c >= 0xDC00 && c < 0xE000 && i >= 2 && u[i-2] >= 0xD800 && u[i-2] < 0xDC00 {
		return utf16.DecodeRune(rune(u[i-2]), rune(c)), 2
	}
	return rune(c), 1
}

// CodePointAt returns the code point at index i of u and the number of code units
// it occupies, as String.prototype.codePointAt reads it.
func CodePointAt(u []uint16, i int) (rune, int) {
	c := u[i]
	if c >= 0xD800 && c < 0xDC00 && i+1 < len(u) && u[i+1] >= 0xDC00 && u[i+1] < 0xE000 {
		return utf16.DecodeRune(rune(c), rune(u[i+1])), 2
	}
	return rune(c), 1
}
