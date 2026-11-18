package log

import (
	"strings"
	"unicode/utf8"

	"github.com/gruntjs/grunt-go/js"
)

// markup is Log.prototype._markup. It applies, in order,
//
//	str.replace(/(\s|^)_(\S|\S[\s\S]+?\S)_(?=[\s,.!?]|$)/g, '$1' + '$2'.underline)
//	str.replace(/(\s|^)\*(\S|\S[\s\S]+?\S)\*(?=[\s,.!?]|$)/g, '$1' + '$2'.bold)
//
// '$2'.underline is evaluated by colors, so with colour codes off both passes only
// drop the markers. Go's regexp has no lookahead, so the patterns are matched by hand
// with the backtracking order of the JavaScript engine, over UTF-16 code units (an
// astral character is two \S units, so "_😀_" does not match, as in the original).
func markup(s string, colorCodes bool) string {
	s = markupPass(s, '_', Stylize(colorCodes, "\x00", "underline"))
	return markupPass(s, '*', Stylize(colorCodes, "\x00", "bold"))
}

// markedText is a string viewed as code units, with the byte offset of each unit so
// replacements can copy untouched text from the original bytes.
type markedText struct {
	s   string
	u   []uint16
	off []int // off[i] is the byte offset of unit i; off[len(u)] == len(s)
}

func newMarkedText(s string) *markedText {
	t := &markedText{s: s}
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r >= 0x10000 {
			hi, lo := 0xd800+uint16((r-0x10000)>>10), 0xdc00+uint16((r-0x10000)&0x3ff)
			t.u = append(t.u, hi, lo)
			// The low half never starts or ends a match (see markup), so it shares
			// the rune's offset.
			t.off = append(t.off, i, i)
		} else {
			t.u = append(t.u, uint16(r))
			t.off = append(t.off, i)
		}
		i += size
	}
	t.off = append(t.off, len(s))
	return t
}

func (t *markedText) space(i int) bool { return js.IsSpace(rune(t.u[i])) }

// lookahead is (?=[\s,.!?]|$) at unit i.
func (t *markedText) lookahead(i int) bool {
	if i == len(t.u) {
		return true
	}
	switch t.u[i] {
	case ',', '.', '!', '?':
		return true
	}
	return t.space(i)
}

// matchFrom tries (\s|^)M(\S|\S[\s\S]+?\S)M(?=[\s,.!?]|$) at unit p and returns the
// bounds of group 2 and the end of the match.
func (t *markedText) matchFrom(p int, marker uint16) (g2start, g2end, end int, ok bool) {
	n := len(t.u)
	// Group 1 tries \s before ^.
	var starts []int
	if p < n && t.space(p) {
		starts = append(starts, p+1)
	}
	if p == 0 {
		starts = append(starts, 0)
	}
	for _, q := range starts {
		if q >= n || t.u[q] != marker {
			continue
		}
		c := q + 1
		if c >= n || t.space(c) {
			continue
		}
		// First alternative: a single \S.
		if c+1 < n && t.u[c+1] == marker && t.lookahead(c+2) {
			return c, c + 1, c + 2, true
		}
		// Second alternative: \S, then the lazy [\s\S]+?, then \S.
		for k := c + 2; k+1 < n; k++ {
			if !t.space(k) && t.u[k+1] == marker && t.lookahead(k+2) {
				return c, k + 1, k + 2, true
			}
		}
	}
	return 0, 0, 0, false
}

// markupPass replaces every match of the pattern for marker with group 1 followed by
// group 2 wrapped in wrap (wrap's "\x00" stands for group 2, like '$2' in the
// original replacement string).
func markupPass(s string, marker byte, wrap string) string {
	if strings.IndexByte(s, marker) == -1 {
		return s
	}
	openCode, closeCode, _ := strings.Cut(wrap, "\x00")
	t := newMarkedText(s)
	var b strings.Builder
	last := 0 // byte offset copied so far
	for p := 0; p <= len(t.u); {
		g2s, g2e, end, ok := t.matchFrom(p, uint16(marker))
		if !ok {
			p++
			continue
		}
		// Group 1 is the text between the match start and the opening marker.
		b.WriteString(s[last:t.off[g2s-1]])
		b.WriteString(openCode)
		b.WriteString(s[t.off[g2s]:t.off[g2e]])
		b.WriteString(closeCode)
		last = t.off[end]
		p = end
	}
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}
