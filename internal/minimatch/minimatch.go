// Package minimatch is a port of the minimatch 3.1.5 npm package, together with
// the brace-expansion 1.1.22 module it depends on, for POSIX paths (path.sep is
// "/").
//
// minimatch compiles each path segment of a glob into a JavaScript RegExp. The
// translation is reproduced here string for string, and the resulting
// expressions run on the shared ECMAScript regular-expression engine
// (internal/jsregexp), so quirks of the original, such as
// "*[" matching the text "[undefined", carry over unchanged.
package minimatch

import (
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsregexp"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// Options are minimatch's options. A field is set when the corresponding
// JavaScript option is truthy.
type Options struct {
	Dot        bool // dot
	NoCase     bool // nocase
	NoComment  bool // nocomment
	NoNegate   bool // nonegate
	FlipNegate bool // flipNegate
	NoExt      bool // noext
	NoBrace    bool // nobrace
	NoGlobstar bool // noglobstar
	MatchBase  bool // matchBase
	NoNull     bool // nonull: MatchList returns the pattern when nothing matches
	Partial    bool // partial: a path may match a leading part of the pattern
	// MaxGlobstarRecursion bounds the nesting of ** segments explored by one
	// match (maxGlobstarRecursion). Nil means the default, 200.
	MaxGlobstarRecursion *int
}

const (
	maxPatternLength            = 1024 * 64
	defaultMaxGlobstarRecursion = 200
)

// TypeError is a TypeError thrown by minimatch. Message is the JavaScript
// error message.
type TypeError struct {
	Message string
}

// Error returns the JavaScript error message.
func (e *TypeError) Error() string { return e.Message }

// errUndefinedCharAt is what minimatch 3.1.5 throws when a pattern with
// consecutive ** segments runs past the end of a path (see matchBodySections).
var errUndefinedCharAt = &TypeError{Message: "Cannot read properties of undefined (reading 'charAt')"}

func assertValidPattern(pattern string) error {
	if jsstr.Length(pattern) > maxPatternLength {
		return &TypeError{Message: "pattern is too long"}
	}
	return nil
}

// Part is one compiled path segment of a pattern: the ** marker (GLOBSTAR), a
// literal segment, or a regular expression.
type Part struct {
	// Globstar is set for a ** segment.
	Globstar bool
	// Literal is the unescaped text of a segment without magic. It is used when
	// Globstar is false and Regexp is nil; the empty string is a valid literal.
	Literal string
	// Regexp matches a segment with magic.
	Regexp *Regexp
}

// Regexp is a RegExp built by minimatch.
type Regexp struct {
	source  string
	flags   string
	glob    string
	src     string
	hasGlob bool
	re      *jsregexp.Regexp
}

// Source is RegExp#source: the pattern the RegExp was constructed from, with
// unescaped slashes outside classes and line terminators escaped as V8 does.
func (r *Regexp) Source() string { return jsregexp.EscapeSource(r.source) }

// Flags is "i" for a nocase expression and "" otherwise.
func (r *Regexp) Flags() string { return r.flags }

// Glob is the segment the expression was made from (minimatch's _glob). It
// reports false for the never-matching /$./ that replaces an expression the
// RegExp constructor rejected, which carries no _glob.
func (r *Regexp) Glob() (string, bool) { return r.glob, r.hasGlob }

// MatchString reports whether s contains a match, as String#match does.
func (r *Regexp) MatchString(s string) bool { return r.re.Test(s) }

// Minimatch is a compiled pattern (minimatch.Minimatch).
type Minimatch struct {
	// Pattern is the trimmed pattern without its leading negation.
	Pattern string
	// Options are the options the pattern was compiled with.
	Options Options
	// Set holds one compiled segment list per brace expansion of Pattern.
	Set [][]Part
	// GlobSet holds the brace expansions of Pattern.
	GlobSet []string
	// GlobParts holds GlobSet split on runs of slashes.
	GlobParts [][]string
	// Negate is set for a pattern with an odd number of leading '!'.
	Negate bool
	// Comment is set for a pattern starting with '#'; it matches nothing.
	Comment bool
	// Empty is set for an empty pattern; it matches only the empty path.
	Empty bool

	maxGlobstarRecursion int
	regexp               *Regexp
	regexpDone           bool
}

// New compiles pattern (new Minimatch(pattern, options)).
func New(pattern string, opts Options) (*Minimatch, error) {
	if err := assertValidPattern(pattern); err != nil {
		return nil, err
	}
	m := &Minimatch{
		Pattern:              strings.TrimFunc(pattern, js.IsSpace),
		Options:              opts,
		Set:                  [][]Part{},
		maxGlobstarRecursion: defaultMaxGlobstarRecursion,
	}
	if opts.MaxGlobstarRecursion != nil {
		m.maxGlobstarRecursion = *opts.MaxGlobstarRecursion
	}
	if err := m.make(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Minimatch) make() error {
	pattern := m.Pattern
	if !m.Options.NoComment && strings.HasPrefix(pattern, "#") {
		m.Comment = true
		return nil
	}
	if pattern == "" {
		m.Empty = true
		return nil
	}
	m.parseNegate()
	set, err := BraceExpand(m.Pattern, m.Options)
	if err != nil {
		return err
	}
	m.GlobSet = set
	m.GlobParts = make([][]string, len(set))
	for i, s := range set {
		m.GlobParts[i] = splitSlashes(s)
	}
	for _, parts := range m.GlobParts {
		compiled := make([]Part, 0, len(parts))
		valid := true
		for _, s := range parts {
			part, ok := m.parse(s)
			if !ok {
				valid = false
				break
			}
			compiled = append(compiled, part)
		}
		// Keep only the expansions whose every segment compiled.
		if valid {
			m.Set = append(m.Set, compiled)
		}
	}
	return nil
}

func (m *Minimatch) parseNegate() {
	if m.Options.NoNegate {
		return
	}
	n := 0
	for n < len(m.Pattern) && m.Pattern[n] == '!' {
		m.Negate = !m.Negate
		n++
	}
	m.Pattern = m.Pattern[n:]
}

// BraceExpand expands the braces of pattern (minimatch.braceExpand). Patterns
// without a {...} group, and every pattern under NoBrace, come back unchanged.
func BraceExpand(pattern string, opts Options) ([]string, error) {
	if err := assertValidPattern(pattern); err != nil {
		return nil, err
	}
	if opts.NoBrace || !hasBraceGroup(pattern) {
		return []string{pattern}, nil
	}
	return expandBraces(pattern), nil
}

// hasBraceGroup is /\{(?:(?!\{).)*\}/.test(pattern): a { followed by a } with
// no { or line terminator in between.
func hasBraceGroup(pattern string) bool {
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '{' {
			continue
		}
		for _, r := range pattern[i+1:] {
			if r == '}' {
				return true
			}
			if r == '{' || jsstr.IsLineTerminator(r) {
				break
			}
		}
	}
	return false
}

// splitSlashes is s.split(/\/+/).
func splitSlashes(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); {
		if s[i] != '/' {
			i++
			continue
		}
		out = append(out, s[start:i])
		for i < len(s) && s[i] == '/' {
			i++
		}
		start = i
	}
	return append(out, s[start:])
}

// Match tests one path against a pattern (minimatch(path, pattern, options)).
func Match(path, pattern string, opts Options) (bool, error) {
	if err := assertValidPattern(pattern); err != nil {
		return false, err
	}
	if !opts.NoComment && strings.HasPrefix(pattern, "#") {
		return false, nil
	}
	m, err := New(pattern, opts)
	if err != nil {
		return false, err
	}
	return m.Match(path)
}

// MatchList returns the paths of list that match pattern (minimatch.match).
// With NoNull, a pattern that matches nothing is returned as the only element.
func MatchList(list []string, pattern string, opts Options) ([]string, error) {
	m, err := New(pattern, opts)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, f := range list {
		ok, err := m.Match(f)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, f)
		}
	}
	if opts.NoNull && len(out) == 0 {
		out = append(out, pattern)
	}
	return out, nil
}

// MakeRe compiles pattern into a single regular expression
// (minimatch.makeRe). It returns nil where minimatch returns false: for a
// comment, an empty pattern, or an expression the RegExp constructor rejects.
func MakeRe(pattern string, opts Options) (*Regexp, error) {
	m, err := New(pattern, opts)
	if err != nil {
		return nil, err
	}
	return m.MakeRe(), nil
}

// MakeRe joins the compiled set into one expression; see the package-level
// MakeRe. The result is cached.
func (m *Minimatch) MakeRe() *Regexp {
	if m.regexpDone {
		return m.regexp
	}
	m.regexpDone = true
	if len(m.Set) == 0 {
		return nil
	}
	twoStar := twoStarNoDot
	switch {
	case m.Options.NoGlobstar:
		twoStar = star
	case m.Options.Dot:
		twoStar = twoStarDot
	}
	alts := make([]string, len(m.Set))
	for i, parts := range m.Set {
		segs := make([]string, len(parts))
		for j, p := range parts {
			switch {
			case p.Globstar:
				segs[j] = twoStar
			case p.Regexp == nil:
				segs[j] = regExpEscape(p.Literal)
			default:
				// The /$./ fallback has no _src; join turns undefined into "".
				segs[j] = p.Regexp.src
			}
		}
		alts[i] = strings.Join(segs, `\/`)
	}
	re := "^(?:" + strings.Join(alts, "|") + ")$"
	if m.Negate {
		re = "^(?!" + re + ").*$"
	}
	flags := ""
	if m.Options.NoCase {
		flags = "i"
	}
	compiled, err := jsregexp.Compile(re, flags)
	if err != nil {
		// try { this.regexp = new RegExp(re, flags) } catch (ex) { this.regexp = false }
		return nil
	}
	m.regexp = &Regexp{source: re, flags: flags, re: compiled}
	return m.regexp
}

// regExpEscape is s.replace(/[-[\]{}()*+?.,\\^$|#\s]/g, '\\$&').
func regExpEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(`-[]{}()*+?.,\^$|#`, r) || js.IsSpace(r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
