package yaml

import (
	"github.com/gruntjs/grunt-go/internal/jsregexp"
	"github.com/gruntjs/grunt-go/js"
)

// newRegExp is new RegExp(pattern, flags) as js/regexp.js calls it: the SyntaxError
// V8 throws for repeated flags or a pattern that does not parse, and otherwise the
// value with V8's escaped source and the flags in canonical order.
func newRegExp(pattern, flags string) (any, error) {
	re, err := jsregexp.Compile(pattern, flags)
	if err != nil {
		return nil, err
	}
	return &js.RegExp{Source: jsregexp.EscapeSource(pattern), Flags: re.Flags()}, nil
}
