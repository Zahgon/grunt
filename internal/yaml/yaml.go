// Package yaml is a port of the js-yaml 3.15.2 loader that grunt's file.readYAML uses:
// yaml.safeLoad by default and yaml.load when the caller asks for {unsafeLoad: true}.
//
// The port follows lib/js-yaml/loader.js function by function, so that results and
// error messages match the original byte for byte. Values use the js package's model:
// mappings are *js.Object, sequences *js.Array, numbers float64, timestamps js.Date,
// !!binary []byte and !!js/regexp *js.RegExp. A failure is reported as the error
// JavaScript would throw: a *js.Error named "YAMLException" whose Message equals
// e.message, or, where a js-yaml type constructor throws, the TypeError or SyntaxError
// it raises.
//
// js-yaml works on UTF-16 code units, and so does this port: line and column numbers,
// error snippets and every character test count code units. Strings are converted to
// Go strings when a node is complete; a lone surrogate (only reachable through a
// "\uD800"-style escape) becomes U+FFFD, which is also what Node prints for it.
//
// Behaviour that grunt cannot observe is left out: warnings (js-yaml only reports them
// through an onWarning callback grunt never passes), the listener hook, and the json,
// legacy and filename options.
//
// Known differences from js-yaml on Node:
//   - !!js/function needs a JavaScript engine; a function source is reported as an
//     unsupported-tag error (a null or non-string node still fails as in js-yaml).
//   - Node's stack overflows on nodes nested a few thousand levels deep (a
//     RangeError); Go's growable stack parses them.
//   - Distinct lone surrogates collapse to U+FFFD, so two keys differing only in
//     such escapes are reported as duplicates.
package yaml

import (
	"github.com/gruntjs/grunt-go/js"
)

// SafeLoad parses src as a single YAML document with js-yaml's DEFAULT_SAFE_SCHEMA,
// as yaml.safeLoad(src) does. An empty stream yields js.Undefined.
func SafeLoad(src string) (any, error) {
	return load(src, defaultSafeSchema)
}

// Load parses src as a single YAML document with js-yaml's DEFAULT_FULL_SCHEMA, as
// yaml.load(src) does: the safe schema plus !!js/undefined, !!js/regexp and
// !!js/function.
func Load(src string) (any, error) {
	return load(src, defaultFullSchema)
}

// load is js-yaml's load(): it parses every document and insists on at most one.
func load(src string, s *schema) (any, error) {
	docs, err := loadDocuments(src, s)
	if err != nil {
		return nil, err
	}
	switch len(docs) {
	case 0:
		return js.Undefined, nil
	case 1:
		return docs[0], nil
	}
	return nil, newException("expected a single document in the stream, but found more", nil)
}

// abort carries an error thrown deep inside the recursive-descent parser up to
// loadDocuments, the way loader.js unwinds with throw. It never escapes the package.
type abort struct {
	err error
}

// loadDocuments is js-yaml's loadDocuments(): it normalises the input, then reads
// documents until the stream is exhausted.
func loadDocuments(src string, s *schema) (docs []any, err error) {
	defer func() {
		if r := recover(); r != nil {
			a, ok := r.(abort)
			if !ok {
				panic(r)
			}
			docs, err = nil, a.err
		}
	}()
	l := newLoader(src, s)
	l.run()
	return l.documents, nil
}

// throwErr unwinds the parser with err.
func throwErr(err error) {
	panic(abort{err: err})
}
