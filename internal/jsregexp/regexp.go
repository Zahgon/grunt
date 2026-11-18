// Package jsregexp is an ECMAScript regular expression engine with the semantics of
// V8 in Node 24: patterns are parsed with V8's grammar (including the Annex B
// web-compatibility syntax when neither the u nor the v flag is given), errors
// carry V8's SyntaxError messages, and matching is backtracking over UTF-16 code
// units (code points with u or v), exactly as RegExpBuiltinExec observes it.
//
// Supported: alternation, greedy and lazy quantifiers with the empty-check rule
// and per-iteration capture reset, capturing, non-capturing and named groups
// (duplicate names in different alternatives), numbered and named
// backreferences, lookahead and lookbehind (matched right to left), modifier
// groups (?ims-ims:...), character classes and class escapes, \p{...} and
// \P{...} property escapes (General_Category, Script, Script_Extensions and every
// binary property of ECMA-262 table 67), the flags d g i m s u v y, and with the
// v flag nested classes, set intersection (&&), subtraction (--) and \q{...}
// string literals.
//
// Like V8, and unlike the letter of the specification, a u or v pattern is tried
// at every code-unit index, including the middle of a surrogate pair, where no
// character can be read but zero-width patterns match (see Exec).
//
// Not supported: the properties of strings that only the v flag accepts
// (Basic_Emoji, Emoji_Keycap_Sequence, RGI_Emoji, RGI_Emoji_Flag_Sequence,
// RGI_Emoji_Modifier_Sequence, RGI_Emoji_Tag_Sequence, RGI_Emoji_ZWJ_Sequence).
// Compile rejects them with a SyntaxError whose message starts with
// "unsupported:". V8 additionally throws "Maximum call stack size exceeded" when
// its backtracking stack overflows; this engine has no such limit and keeps
// backtracking, as V8 does for patterns within its limit.
package jsregexp

import (
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// Regexp is a compiled regular expression. It is immutable and safe for
// concurrent use; the lastIndex state of a JavaScript RegExp object belongs to
// the caller.
type Regexp struct {
	source string
	flags  flagSet
	prog   []inst
	ncap   int
	names  []string
	nregs  int
}

// flagSet holds the parsed flags of a pattern.
type flagSet struct {
	hasIndices, global, ignoreCase, multiline, dotAll, unicode, unicodeSets, sticky bool
}

// String returns the flags in the canonical order of RegExp.prototype.flags.
func (f flagSet) String() string {
	var b strings.Builder
	for _, x := range []struct {
		on bool
		c  byte
	}{{f.hasIndices, 'd'}, {f.global, 'g'}, {f.ignoreCase, 'i'}, {f.multiline, 'm'}, {f.dotAll, 's'}, {f.unicode, 'u'}, {f.unicodeSets, 'v'}, {f.sticky, 'y'}} {
		if x.on {
			b.WriteByte(x.c)
		}
	}
	return b.String()
}

// parseFlags parses a flags string; it fails on an unknown or repeated flag and
// on u combined with v.
func parseFlags(s string) (flagSet, bool) {
	var f flagSet
	for i := 0; i < len(s); i++ {
		var p *bool
		switch s[i] {
		case 'd':
			p = &f.hasIndices
		case 'g':
			p = &f.global
		case 'i':
			p = &f.ignoreCase
		case 'm':
			p = &f.multiline
		case 's':
			p = &f.dotAll
		case 'u':
			p = &f.unicode
		case 'v':
			p = &f.unicodeSets
		case 'y':
			p = &f.sticky
		default:
			return f, false
		}
		if *p {
			return f, false
		}
		*p = true
	}
	if f.unicode && f.unicodeSets {
		return f, false
	}
	return f, true
}

// Compile parses a pattern as `new RegExp(source, flags)` does. source is a
// JavaScript string in jsstr's WTF-8 form. Errors are *js.Error SyntaxErrors with
// V8's messages, such as "Invalid regular expression: /(/g: Unterminated group"
// (the flags appear in canonical order) or "Invalid flags supplied to RegExp
// constructor 'gg'".
func Compile(source, flags string) (*Regexp, error) {
	f, ok := parseFlags(flags)
	if !ok {
		return nil, js.NewSyntaxError("Invalid flags supplied to RegExp constructor '" + flags + "'")
	}
	return compile(source, f, f.String())
}

// CompileLiteral parses a regular expression literal /source/flags. It differs
// from Compile only in its error texts: bad flags give "Invalid regular
// expression flags", and pattern errors quote the flags as written.
func CompileLiteral(source, flags string) (*Regexp, error) {
	f, ok := parseFlags(flags)
	if !ok {
		return nil, js.NewSyntaxError("Invalid regular expression flags")
	}
	return compile(source, f, flags)
}

func compile(source string, f flagSet, shownFlags string) (*Regexp, error) {
	p := newParser(jsstr.ToUTF16(source), f)
	tree, msg := p.parse()
	if msg != "" {
		if strings.HasPrefix(msg, "unsupported:") {
			return nil, js.NewSyntaxError(msg)
		}
		return nil, js.NewSyntaxError("Invalid regular expression: /" + source + "/" + shownFlags + ": " + msg)
	}
	re := &Regexp{source: source, flags: f, ncap: p.ncap, names: p.names}
	c := compiler{unicode: f.unicode || f.unicodeSets}
	re.prog, re.nregs = c.compileProgram(tree)
	return re, nil
}

// Source returns the pattern as given to Compile.
func (re *Regexp) Source() string { return re.source }

// Flags returns the flags in the canonical order "dgimsuvy" of
// RegExp.prototype.flags.
func (re *Regexp) Flags() string { return re.flags.String() }

// Global reports the g flag.
func (re *Regexp) Global() bool { return re.flags.global }

// IgnoreCase reports the i flag.
func (re *Regexp) IgnoreCase() bool { return re.flags.ignoreCase }

// Multiline reports the m flag.
func (re *Regexp) Multiline() bool { return re.flags.multiline }

// DotAll reports the s flag.
func (re *Regexp) DotAll() bool { return re.flags.dotAll }

// Unicode reports the u flag.
func (re *Regexp) Unicode() bool { return re.flags.unicode }

// UnicodeSets reports the v flag.
func (re *Regexp) UnicodeSets() bool { return re.flags.unicodeSets }

// Sticky reports the y flag.
func (re *Regexp) Sticky() bool { return re.flags.sticky }

// HasIndices reports the d flag.
func (re *Regexp) HasIndices() bool { return re.flags.hasIndices }

// NumCaptures returns the number of capture groups, excluding group 0.
func (re *Regexp) NumCaptures() int { return re.ncap }

// GroupNames returns the name of every group by index, "" for unnamed groups and
// for group 0. With duplicate named groups the same name appears at several
// indexes; at most one of them participates in a match, and the groups object
// takes its value from that one (undefined when none does).
func (re *Regexp) GroupNames() []string { return re.names }

// HasNamedGroups reports whether the pattern has a named group, in which case
// match results carry a groups object.
func (re *Regexp) HasNamedGroups() bool {
	for _, n := range re.names {
		if n != "" {
			return true
		}
	}
	return false
}

// EscapeSource is EscapeRegExpPattern as V8 implements it: the value of
// RegExp.prototype.source and the text between the slashes of toString. An empty
// pattern becomes "(?:)", a "/" outside a character class is escaped, and line
// terminators become \n, \r,   and   (dropping a backslash that
// already escaped them).
func EscapeSource(src string) string {
	if src == "" {
		return "(?:)"
	}
	u := jsstr.ToUTF16(src)
	out := make([]uint16, 0, len(u)+4)
	add := func(s string) {
		for i := 0; i < len(s); i++ {
			out = append(out, uint16(s[i]))
		}
	}
	inClass := false
	for s := 0; s < len(u); {
		c := u[s]
		switch {
		case c == '\\':
			if s+1 < len(u) && jsstr.IsLineTerminator(rune(u[s+1])) {
				s++
				continue
			}
			out = append(out, c)
			s++
			if s == len(u) {
				return jsstr.FromUTF16(out)
			}
		case c == '/' && !inClass:
			out = append(out, '\\')
		case c == '[':
			inClass = true
		case c == ']':
			inClass = false
		case c == '\n':
			add(`\n`)
			s++
			continue
		case c == '\r':
			add(`\r`)
			s++
			continue
		case c == 0x2028:
			add("\\u2028")
			s++
			continue
		case c == 0x2029:
			add("\\u2029")
			s++
			continue
		}
		out = append(out, u[s])
		s++
	}
	return jsstr.FromUTF16(out)
}
