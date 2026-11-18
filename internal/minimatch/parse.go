package minimatch

import (
	"github.com/gruntjs/grunt-go/internal/jsregexp"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
)

// This file ports Minimatch.prototype.parse, which translates one path segment
// into RegExp source. It works on UTF-16 code units, as the original does: the
// negated-extglob rewrite at the end slices the expression at offsets recorded
// before earlier rewrites, and those offsets only line up with JavaScript's if
// they are counted in the same units.

// RegExp fragments from minimatch.js.
const (
	qmark        = `[^/]`   // any single character other than /
	star         = `[^/]*?` // any number of them
	twoStarDot   = `(?:(?!(?:\/|^)(?:\.{1,2})($|\/)).)*?`
	twoStarNoDot = `(?:(?!(?:\/|^)\.).)*?`
)

// reSpecials are the characters that need escaping in a RegExp.
const reSpecials = `().*{}+?[]^$\!`

func isReSpecial(c uint16) bool {
	return c < 0x80 && containsByte(reSpecials, byte(c))
}

func containsByte(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}

// plType holds the RegExp text that opens and closes an extglob pattern list.
type plType struct {
	open, close string
}

var plTypes = map[uint16]plType{
	'!': {open: `(?:(?!(?:`, close: `))[^/]*?)`},
	'?': {open: `(?:`, close: `)?`},
	'+': {open: `(?:`, close: `)+`},
	'*': {open: `(?:`, close: `)*`},
	'@': {open: `(?:`, close: `)`},
}

// patternList is an open or closed extglob group, with offsets into the RegExp
// source being built.
type patternList struct {
	typ     uint16
	reStart int
	reEnd   int
	plType
}

// cat concatenates into a new slice, so no part is aliased.
func cat(parts ...[]uint16) []uint16 {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	out := make([]uint16, 0, n)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// parse compiles one segment of an expanded pattern. It reports false for a
// segment containing "/", which minimatch rejects.
func (m *Minimatch) parse(pattern string) (Part, bool) {
	if pattern == "**" {
		if !m.Options.NoGlobstar {
			return Part{Globstar: true}, true
		}
		pattern = "*"
	}
	if pattern == "" {
		return Part{}, true
	}
	src := jsstr.ToUTF16(pattern)
	re, hasMagic, ok := m.translate(src, false)
	if !ok {
		return Part{}, false
	}
	// A pattern without magic is matched as a literal, unescaped string.
	if !hasMagic {
		return Part{Literal: jsstr.FromUTF16WellFormed(globUnescape(src))}, true
	}
	flags := ""
	if m.Options.NoCase {
		flags = "i"
	}
	full := cat(jsstr.ToUTF16("^"), re, jsstr.ToUTF16("$"))
	compiled, err := jsregexp.Compile(jsstr.FromUTF16WellFormed(full), flags)
	if err != nil {
		// An invalid expression can't match anything; minimatch substitutes
		// new RegExp('$.'), which has no _glob or _src.
		never, _ := jsregexp.Compile("$.", "")
		return Part{Regexp: &Regexp{source: "$.", re: never}}, true
	}
	return Part{Regexp: &Regexp{source: jsstr.FromUTF16WellFormed(full), flags: flags, glob: pattern, src: jsstr.FromUTF16WellFormed(re), hasGlob: true, re: compiled}}, true
}

// subparse is parse(pattern, SUBPARSE), used to re-translate the contents of a
// bracket expression that is not a valid class. For "**" and "" parse returns
// GLOBSTAR or "" before reaching the SUBPARSE return, and the caller's sp[0]
// and sp[1] are then undefined: the string "undefined" lands in the
// expression and no magic is added.
func (m *Minimatch) subparse(pattern []uint16) ([]uint16, bool) {
	s := jsstr.FromUTF16WellFormed(pattern)
	if s == "" || s == "**" && !m.Options.NoGlobstar {
		return jsstr.ToUTF16("undefined"), false
	}
	if s == "**" {
		pattern = jsstr.ToUTF16("*")
	}
	re, hasMagic, _ := m.translate(pattern, true)
	return re, hasMagic
}

// translate is the body of parse: it returns the RegExp source of pattern and
// whether it has magic, or false for a pattern containing "/".
func (m *Minimatch) translate(pattern []uint16, isSub bool) ([]uint16, bool, bool) {
	opts := m.Options
	var re []uint16
	hasMagic := opts.NoCase
	escaping := false
	var patternListStack []*patternList
	var negativeLists []*patternList
	var stateChar uint16 // 0 when there is none
	inClass := false
	reClassStart := -1
	classStart := -1
	// . and .. never match anything that doesn't start with ., even when
	// options.dot is set.
	var patternStart string
	switch {
	case len(pattern) > 0 && pattern[0] == '.':
	case opts.Dot:
		patternStart = `(?!(?:^|\/)\.{1,2}(?:$|\/))`
	default:
		patternStart = `(?!\.)`
	}
	add := func(s string) { re = append(re, jsstr.ToUTF16(s)...) }

	clearStateChar := func() {
		if stateChar == 0 {
			return
		}
		switch stateChar {
		case '*':
			add(star)
			hasMagic = true
		case '?':
			add(qmark)
			hasMagic = true
		default:
			re = append(re, '\\', stateChar)
		}
		stateChar = 0
	}

	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		// Skip over any that are escaped.
		if escaping && isReSpecial(c) {
			re = append(re, '\\', c)
			escaping = false
			continue
		}
		switch c {
		case '/':
			return nil, false, false
		case '\\':
			clearStateChar()
			escaping = true
		case '?', '*', '+', '@', '!':
			// Literals inside a class, except that [!a] means [^a].
			if inClass {
				if c == '!' && i == classStart+1 {
					c = '^'
				}
				re = append(re, c)
				continue
			}
			// Coalesce consecutive non-globstar * characters.
			if c == '*' && stateChar == '*' {
				continue
			}
			clearStateChar()
			stateChar = c
			// With extglob disabled, +(asdf|foo) isn't a thing.
			if opts.NoExt {
				clearStateChar()
			}
		case '(':
			if inClass {
				re = append(re, '(')
				continue
			}
			if stateChar == 0 {
				add(`\(`)
				continue
			}
			pl := &patternList{typ: stateChar, reStart: len(re), plType: plTypes[stateChar]}
			patternListStack = append(patternListStack, pl)
			add(pl.open)
			stateChar = 0
		case ')':
			if inClass || len(patternListStack) == 0 {
				add(`\)`)
				continue
			}
			clearStateChar()
			hasMagic = true
			pl := patternListStack[len(patternListStack)-1]
			patternListStack = patternListStack[:len(patternListStack)-1]
			add(pl.close)
			if pl.typ == '!' {
				negativeLists = append(negativeLists, pl)
			}
			pl.reEnd = len(re)
		case '|':
			if inClass || len(patternListStack) == 0 || escaping {
				add(`\|`)
				escaping = false
				continue
			}
			clearStateChar()
			re = append(re, '|')
		case '[':
			// Swallow any state-tracking char before the [.
			clearStateChar()
			if inClass {
				add(`\[`)
				continue
			}
			inClass = true
			classStart = i
			reClassStart = len(re)
			re = append(re, c)
		case ']':
			// A ] first in the list, or outside a class, is literal. Unlike the
			// other literals it does not flush a pending * or ? first.
			if i == classStart+1 || !inClass {
				add(`\]`)
				escaping = false
				continue
			}
			// minimatch asks the RegExp constructor whether "[" + cs + "]" is a
			// valid class. If not, the would-be class is re-translated as
			// literal text.
			cs := pattern[classStart+1 : i]
			if _, err := jsregexp.Compile("["+jsstr.FromUTF16WellFormed(cs)+"]", ""); err != nil {
				sp, spMagic := m.subparse(cs)
				re = cat(sliceParts(re, 0, reClassStart), jsstr.ToUTF16(`\[`), sp, jsstr.ToUTF16(`\]`))
				hasMagic = hasMagic || spMagic
				inClass = false
				continue
			}
			hasMagic = true
			inClass = false
			re = append(re, c)
		default:
			// Swallow any state char that wasn't consumed.
			clearStateChar()
			if escaping {
				escaping = false
			} else if isReSpecial(c) && !(c == '^' && inClass) {
				re = append(re, '\\')
			}
			re = append(re, c)
		}
	}

	// A class left open, as in "[abc", is literal: escape the [ and
	// re-translate what followed it.
	if inClass {
		sp, spMagic := m.subparse(pattern[classStart+1:])
		re = cat(sliceParts(re, 0, reClassStart), jsstr.ToUTF16(`\[`), sp)
		hasMagic = hasMagic || spMagic
	}

	// A pattern list left open, as in "+(a|b", is literal too: escape its
	// opener and every | it passed through.
	for len(patternListStack) > 0 {
		pl := patternListStack[len(patternListStack)-1]
		patternListStack = patternListStack[:len(patternListStack)-1]
		tail := escapeUnescapedPipes(sliceParts(re, pl.reStart+len(pl.open), len(re)))
		var t string
		switch pl.typ {
		case '*':
			t = star
		case '?':
			t = qmark
		default:
			t = `\` + string(rune(pl.typ))
		}
		hasMagic = true
		re = cat(sliceParts(re, 0, pl.reStart), jsstr.ToUTF16(t+`\(`), tail)
	}

	// Handle trailing things that only matter at the very end.
	clearStateChar()
	if escaping {
		add(`\\`)
	}

	// Only apply the nodot start if the re starts with something that could
	// conceivably capture a dot.
	addPatternStart := len(re) > 0 && (re[0] == '[' || re[0] == '.' || re[0] == '(')

	// JavaScript has no lookbehind in minimatch's target engines, so for a
	// pattern like *.!(x).!(y|z) the first negative lookahead has to look all
	// the way to the end of the pattern. The offsets used here were recorded
	// while parsing and are not adjusted for the rewrites above.
	for n := len(negativeLists) - 1; n >= 0; n-- {
		nl := negativeLists[n]
		nlBefore := sliceParts(re, 0, nl.reStart)
		nlFirst := sliceParts(re, nl.reStart, nl.reEnd-8)
		nlLast := sliceParts(re, nl.reEnd-8, nl.reEnd)
		nlAfter := sliceParts(re, nl.reEnd, len(re))
		nlLast = cat(nlLast, nlAfter)
		// For nested lists like *(*.js|!(*.json)), open parens before the
		// negation mean the ) that closes them is not part of "after".
		openParensBefore := 0
		for _, u := range nlBefore {
			if u == '(' {
				openParensBefore++
			}
		}
		cleanAfter := nlAfter
		for i := 0; i < openParensBefore; i++ {
			cleanAfter = removeFirstCloseParen(cleanAfter)
		}
		nlAfter = cleanAfter
		var dollar []uint16
		if len(nlAfter) == 0 && !isSub {
			dollar = jsstr.ToUTF16("$")
		}
		re = cat(nlBefore, nlFirst, nlAfter, dollar, nlLast)
	}

	// A non-empty expression must not match an empty path part, or a/* would
	// match a/.
	if len(re) > 0 && hasMagic {
		re = cat(jsstr.ToUTF16("(?=.)"), re)
	}
	if addPatternStart {
		re = cat(jsstr.ToUTF16(patternStart), re)
	}
	return re, hasMagic, true
}

// escapeUnescapedPipes is
//
//	tail.replace(/((?:\\{2}){0,64})(\\?)\|/g, (_, $1, $2) => $1 + $1 + ($2 || '\\') + '|')
//
// Each match is a run of at most 129 backslashes ending at a |: the run is
// doubled, except that a final odd backslash stays single (it already
// escapes the |), and an even run gains one.
func escapeUnescapedPipes(tail []uint16) []uint16 {
	var out []uint16
	last := 0
	for i, u := range tail {
		if u != '|' {
			continue
		}
		start := i
		for start > last && tail[start-1] == '\\' {
			start--
		}
		// A longer run cannot match from its first backslash; the leftmost
		// match starts where at most 129 remain.
		start = max(start, i-129)
		run := i - start
		pairs := run / 2
		out = append(out, tail[last:start]...)
		for j := 0; j < 4*pairs; j++ {
			out = append(out, '\\')
		}
		out = append(out, '\\', '|')
		last = i + 1
	}
	return append(out, tail[last:]...)
}

// removeFirstCloseParen is s.replace(/\)[+*?]?/, "").
func removeFirstCloseParen(s []uint16) []uint16 {
	for i, u := range s {
		if u != ')' {
			continue
		}
		end := i + 1
		if end < len(s) && (s[end] == '+' || s[end] == '*' || s[end] == '?') {
			end++
		}
		return cat(s[:i], s[end:])
	}
	return s
}

// globUnescape is s.replace(/\\(.)/g, '$1'): drop a backslash before any
// code unit that is not a line terminator.
func globUnescape(s []uint16) []uint16 {
	out := make([]uint16, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && !jsstr.IsLineTerminator(rune(s[i+1])) {
			i++
		}
		out = append(out, s[i])
	}
	return out
}
