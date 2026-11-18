// Package jsregex provides the String.prototype methods that take regular
// expressions (replace, split, match, test) and their string-pattern forms, as
// underscore.string and lodash call them. Patterns are compiled and matched by the
// shared ECMAScript engine internal/jsregexp; this package adds the iteration,
// replacement templates and callbacks of the String methods. Strings are
// JavaScript strings in jsstr's WTF-8 form, so lone surrogates survive.
package jsregex

import (
	"github.com/gruntjs/grunt-go/internal/jsregexp"
)

// Regexp is a compiled regular expression with its sticky twin, which split needs.
type Regexp struct {
	*jsregexp.Regexp
	sticky *jsregexp.Regexp
}

// Compile parses source with flags as new RegExp(source, flags) does; errors are
// V8's SyntaxErrors.
func Compile(source, flags string) (*Regexp, error) {
	re, err := jsregexp.Compile(source, flags)
	if err != nil {
		return nil, err
	}
	sticky := re
	if !re.Sticky() {
		// @@split builds its splitter with the flags plus "y"; the pattern already
		// compiled, so this cannot fail.
		if sticky, err = jsregexp.Compile(source, flags+"y"); err != nil {
			return nil, err
		}
	}
	return &Regexp{Regexp: re, sticky: sticky}, nil
}

// MustCompile is Compile for patterns known to be valid.
func MustCompile(source, flags string) *Regexp {
	re, err := Compile(source, flags)
	if err != nil {
		panic(err)
	}
	return re
}
