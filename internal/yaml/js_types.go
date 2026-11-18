package yaml

import (
	"strings"

	"github.com/gruntjs/grunt-go/js"
)

// The JavaScript types DEFAULT_FULL_SCHEMA adds (lib/js-yaml/type/js).

// undefinedType is js/undefined.js.
var undefinedType = &yamlType{
	tag:       "tag:yaml.org,2002:js/undefined",
	kind:      kindScalar,
	resolve:   func(any) bool { return true },
	construct: func(any) (any, error) { return js.Undefined, nil },
}

// regexpType is js/regexp.js: "/source/flags" with up to three of g, i and m, or a
// bare source.
var regexpType = &yamlType{
	tag:  "tag:yaml.org,2002:js/regexp",
	kind: kindScalar,
	resolve: func(data any) bool {
		if data == nil {
			return false
		}
		s, ok := data.(string)
		if !ok {
			// A non-string has no length and no "/" at index 0.
			return true
		}
		if s == "" {
			return false
		}
		if s[0] == '/' {
			modifiers := regexpModifiers(s)
			if len(modifiers) > 3 {
				return false
			}
			// An expression starting with "/" must be closed by one.
			if s[len(s)-len(modifiers)-1] != '/' {
				return false
			}
		}
		return true
	},
	construct: func(data any) (any, error) {
		s, ok := data.(string)
		if !ok {
			// new RegExp(data) converts the value to a string.
			return newRegExp(js.ToString(data), "")
		}
		modifiers := ""
		if s[0] == '/' {
			modifiers = regexpModifiers(s)
			// String.prototype.slice yields "" when the end precedes the start.
			end := len(s) - len(modifiers) - 1
			s = s[1:max(end, 1)]
		}
		return newRegExp(s, modifiers)
	},
}

// regexpModifiers returns the flags matched by /\/([gim]*)$/ in s, or "". Only the
// last "/" can be followed by flag letters alone.
func regexpModifiers(s string) string {
	i := strings.LastIndexByte(s, '/')
	if i < 0 {
		return ""
	}
	tail := s[i+1:]
	if strings.Trim(tail, "gim") != "" {
		return ""
	}
	return tail
}

// functionType is js/function.js. js-yaml parses the source with esprima and builds
// the function with new Function, which this port cannot do. A null node fails to
// resolve as in js-yaml; a non-string one (a plain scalar resolved to a number or
// boolean) is not a function expression either; for a string, constructing the
// function is reported as unsupported.
var functionType = &yamlType{
	tag:  "tag:yaml.org,2002:js/function",
	kind: kindScalar,
	resolve: func(data any) bool {
		_, ok := data.(string)
		return ok
	},
	construct: func(any) (any, error) {
		return nil, newException("unsupported tag !<tag:yaml.org,2002:js/function>: constructing JavaScript functions is not supported", nil)
	},
}
