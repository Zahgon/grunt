// Package jsrt runs the JavaScript that grunt templates contain. It reproduces
// lodash 4.18.1's _.template, which compiles a template into JavaScript source and
// evaluates it with the Function constructor, by building the same source text and
// running it on a tree-walking interpreter for a documented JavaScript subset
// (api/template-js.md) with V8's (Node 24) results and error messages.
package jsrt

import (
	"strconv"
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsregexp"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// Settings are the _.template options grunt controls.
type Settings struct {
	// Escape, Evaluate and Interpolate are the sources of the delimiter regular
	// expressions (lodash's templateSettings.escape, .evaluate and .interpolate).
	// An empty source leaves that kind of tag out, as lodash's reNoMatch does.
	Escape, Evaluate, Interpolate string
	// ESTemplate also accepts ${...} interpolations (lodash's reEsTemplate). lodash
	// does this only when the interpolate option is its own default reInterpolate
	// object, which grunt's template.process never passes: grunt installs freshly
	// built RegExps.
	ESTemplate bool
	// Variable names the data object inside the template; empty wraps the code in
	// with (obj) { ... } so data properties are free variables.
	Variable string
	// Imports are the free variables of the template code besides the data, in
	// parameter order; lodash passes {_: lodash}. The escape tag calls _.escape.
	Imports *js.Object
	// Realm is the global environment the template runs in; nil gives the
	// template a realm of its own without a clock.
	Realm *Realm
}

// regexpSource escapes every character but line terminators with a backslash,
// as grunt's addDelimiters does with str.replace(/(.)/g, '\\$1').
func regexpSource(s string) string {
	u := jsstr.ToUTF16(s)
	out := make([]uint16, 0, 2*len(u))
	for _, c := range u {
		if c != '\n' && c != '\r' && c != 0x2028 && c != 0x2029 {
			out = append(out, '\\')
		}
		out = append(out, c)
	}
	return jsstr.FromUTF16(out)
}

// Delimiters returns the settings grunt.template.addDelimiters builds for an
// opener and closer, e.g. Delimiters("<%", "%>") for grunt's config delimiters.
func Delimiters(opener, closer string) Settings {
	a := regexpSource(opener)
	b := `([\s\S]+?)` + regexpSource(closer)
	return Settings{Evaluate: a + b, Interpolate: a + "=" + b, Escape: a + "-" + b}
}

// LodashDefaults returns lodash's own templateSettings (<%- %>, <% %>, <%= %> and
// ${...}), without imports.
func LodashDefaults() Settings {
	return Settings{
		Escape:      `<%-([\s\S]+?)%>`,
		Evaluate:    `<%([\s\S]+?)%>`,
		Interpolate: `<%=([\s\S]+?)%>`,
		ESTemplate:  true,
	}
}

// Template is a compiled template.
type Template struct {
	// Source is the generated function source, lodash's compiled.source.
	Source string
	realm  *Realm
	fn     *js.Function
}

const (
	reNoMatch                  = `($^)`
	reEsTemplate               = `\$\{([^\\}]*(?:\\.[^\\}]*)*)\}`
	forbiddenIdentifierPattern = `[()=,{}\[\]\/\s]`
)

// CompileTemplate compiles text as lodash 4.18.1's _.template(text, options)
// does. Syntax errors in the generated source are returned as V8 reports them.
func CompileTemplate(text string, s Settings) (*Template, error) {
	r := s.Realm
	if r == nil {
		r = NewRealm(Options{})
	}
	imports := s.Imports
	if imports == nil {
		imports = js.NewObject()
	}
	forbidden, err := jsregexp.Compile(forbiddenIdentifierPattern, "")
	if err != nil {
		return nil, err
	}
	keys := imports.Keys()
	values := make([]any, len(keys))
	for i, k := range keys {
		if forbidden.Test(k) {
			return nil, js.NewError("Invalid `imports` option passed into `_.template`")
		}
		values[i] = imports.Get(k)
	}
	source, err := r.templateSource(text, s)
	if err != nil {
		return nil, err
	}
	if s.Variable != "" && forbidden.Test(s.Variable) {
		return nil, js.NewError("Invalid `variable` option passed into `_.template`")
	}
	r.templateCounter++
	body := "//# sourceURL=lodash.templateSources[" + strconv.Itoa(r.templateCounter) + "]\nreturn " + source
	fi, err := parseFunctionConstructor(keys, body)
	if err != nil {
		return nil, err
	}
	outer := r.makeFunction(fi, r.globalEnv(), "anonymous")
	inner, err := r.call(outer, js.Undefined, values)
	if err != nil {
		return nil, err
	}
	f, ok := inner.(*js.Function)
	if !ok {
		return nil, js.NewTypeError("template source did not produce a function")
	}
	return &Template{Source: source, realm: r, fn: f}, nil
}

// templateSource builds the function source lodash's template generates.
func (r *Realm) templateSource(text string, s Settings) (string, error) {
	orNoMatch := func(src string) string {
		if src == "" {
			return reNoMatch
		}
		return src
	}
	es := reNoMatch
	if s.ESTemplate {
		es = reEsTemplate
	}
	delims, err := jsregexp.Compile(orNoMatch(s.Escape)+"|"+orNoMatch(s.Interpolate)+"|"+es+"|"+orNoMatch(s.Evaluate)+"|$", "g")
	if err != nil {
		return "", err
	}
	u := jsstr.ToUTF16(text)
	group := func(caps []int, n int) string {
		if 2*n+1 >= len(caps) || caps[2*n] < 0 {
			return ""
		}
		return jsstr.FromUTF16(u[caps[2*n]:caps[2*n+1]])
	}
	var src strings.Builder
	src.WriteString("__p += '")
	isEscaping, isEvaluating := false, false
	index := 0
	// String.prototype.replace with a global RegExp: every match from the left,
	// stepping over empty matches.
	for pos := 0; pos <= len(u); {
		caps := delims.Exec(u, pos)
		if caps == nil {
			break
		}
		escapeValue, interpolateValue, esValue, evaluateValue := group(caps, 1), group(caps, 2), group(caps, 3), group(caps, 4)
		if interpolateValue == "" {
			interpolateValue = esValue
		}
		writeEscapedText(&src, u[index:caps[0]])
		if escapeValue != "" {
			isEscaping = true
			src.WriteString("' +\n__e(" + escapeValue + ") +\n'")
		}
		if evaluateValue != "" {
			isEvaluating = true
			src.WriteString("';\n" + evaluateValue + ";\n__p += '")
		}
		if interpolateValue != "" {
			src.WriteString("' +\n((__t = (" + interpolateValue + ")) == null ? '' : __t) +\n'")
		}
		index = caps[1]
		pos = caps[1]
		if caps[0] == caps[1] {
			pos++
		}
	}
	src.WriteString("';\n")
	source := src.String()
	if s.Variable == "" {
		source = "with (obj) {\n" + source + "\n}\n"
	}
	// Strip the empty string concatenations, as lodash does with its
	// reEmptyStringLeading, reEmptyStringMiddle and reEmptyStringTrailing.
	if isEvaluating {
		if source, err = r.regexpReplaceAll(source, `\b__p \+= '';`, ""); err != nil {
			return "", err
		}
	}
	if source, err = r.regexpReplaceAll(source, `\b(__p \+=) '' \+`, "$1"); err != nil {
		return "", err
	}
	if source, err = r.regexpReplaceAll(source, `(__e\(.*?\)|\b__t\)) \+\n'';`, "$1;"); err != nil {
		return "", err
	}
	variable := s.Variable
	if variable == "" {
		variable = "obj"
	}
	var b strings.Builder
	b.WriteString("function(" + variable + ") {\n")
	if s.Variable == "" {
		b.WriteString("obj || (obj = {});\n")
	}
	b.WriteString("var __t, __p = ''")
	if isEscaping {
		b.WriteString(", __e = _.escape")
	}
	if isEvaluating {
		b.WriteString(", __j = Array.prototype.join;\nfunction print() { __p += __j.call(arguments, '') }\n")
	} else {
		b.WriteString(";\n")
	}
	b.WriteString(source + "return __p\n}")
	return b.String(), nil
}

// writeEscapedText appends template text inside a single-quoted string literal,
// escaping the characters lodash's reUnescapedString matches.
func writeEscapedText(b *strings.Builder, u []uint16) {
	var out []uint16
	for _, c := range u {
		switch c {
		case '\'', '\\':
			out = append(out, '\\', c)
		case '\n':
			out = append(out, '\\', 'n')
		case '\r':
			out = append(out, '\\', 'r')
		case 0x2028:
			out = append(out, jsstr.ToUTF16(` `)...)
		case 0x2029:
			out = append(out, jsstr.ToUTF16(` `)...)
		default:
			out = append(out, c)
		}
	}
	b.WriteString(jsstr.FromUTF16(out))
}

// regexpReplaceAll is s.replace(new RegExp(pattern, 'g'), replacement).
func (r *Realm) regexpReplaceAll(s, pattern, replacement string) (string, error) {
	re, err := r.newRegExp(pattern, "g")
	if err != nil {
		return "", err
	}
	v, err := r.stringReplace(s, re, replacement, false)
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

// Call runs the template function on data and returns its result as is: normally
// the rendered string, but evaluate code may return any value. data is the
// template's obj: a *js.Object, a *Derived (WithProto) or any other value.
func (t *Template) Call(data any) (any, error) {
	return t.realm.call(t.fn, js.Undefined, []any{data})
}

// Execute renders the template with data, converting a non-string result with
// ToString. Unpaired surrogates are kept in jsstr's WTF-8 form; apply
// jsstr.ToWellFormed before writing the text as UTF-8.
func (t *Template) Execute(data any) (string, error) {
	v, err := t.Call(data)
	if err != nil {
		return "", err
	}
	return t.realm.toString(v)
}
