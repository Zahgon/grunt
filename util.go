package grunt

import (
	"strconv"
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsrt"
	"github.com/gruntjs/grunt-go/js"
)

// Util is grunt.util (grunt-legacy-util 2.0.2).
type Util struct {
	g *Grunt
}

// Underscore is grunt.util._: lodash 4.18.1 with underscore.string 3.3.6 mixed in.
// Its methods are *js.Function values; the same object is `_` in templates.
func (u *Util) Underscore() *js.Object { return u.g.underscore }

// kindOf is util.kindOf: the [[Class]] of a value in lower case, "null",
// "undefined", or "object" for everything without a class of its own (plain
// objects, Buffers).
func kindOf(v any) string {
	switch kind := strings.ToLower(js.ClassOf(v)); kind {
	case "null", "undefined", "number", "string", "boolean", "function", "regexp", "array", "date", "error":
		return kind
	}
	return "object"
}

// KindOf is grunt.util.kindOf.
func (u *Util) KindOf(v any) string { return kindOf(js.Normalize(v)) }

// Pluralize is grunt.util.pluralize: of "a/b" (split on sep, "/" by default) it
// returns "a" when n is exactly 1 and "b" otherwise.
func (u *Util) Pluralize(n any, s string, sep ...string) string {
	return pluralize(js.Normalize(n), s, sep...)
}

func pluralize(n any, s string, sep ...string) string {
	separator := "/"
	if len(sep) > 0 && sep[0] != "" {
		separator = sep[0]
	}
	parts := strings.Split(s, separator)
	i := 1
	if n == 1.0 {
		i = 0
	}
	if i < len(parts) {
		return parts[i]
	}
	return ""
}

// NormalizeLF is grunt.util.normalizelf: every "\r\n" or "\n" becomes the system
// line feed, "\n".
func (u *Util) NormalizeLF(s string) string { return normalizeLF(s) }

func normalizeLF(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

// Repeat is grunt.util.repeat: s (a space when empty or omitted) repeated n times.
// A negative n below -1 is an invalid array length, a RangeError.
func (u *Util) Repeat(n int, s ...string) string {
	str := " "
	if len(s) > 0 && s[0] != "" {
		str = s[0]
	}
	if n < -1 {
		panic(js.InvalidArrayLength())
	}
	return strings.Repeat(str, max(n, 0))
}

// Error is grunt.util.error: err as an Error (a message becomes a new Error), with
// orig as its origError.
func (u *Util) Error(err any, orig ...error) *js.Error {
	return utilError(js.Normalize(err), orig...)
}

func utilError(err any, orig ...error) *js.Error {
	e, ok := err.(*js.Error)
	if !ok {
		e = js.NewError(js.ToString(err))
	}
	if len(orig) > 0 && orig[0] != nil {
		e.Orig = orig[0]
	}
	return e
}

// Recurse is grunt.util.recurse: it rebuilds arrays and plain objects, passing
// every other value through fn. When cont returns false for a value, the value is
// kept as it is. A value that contains itself fails with "Circular reference
// detected (path)".
func (u *Util) Recurse(value any, fn func(v any) any, cont ...func(v any) bool) (any, error) {
	var c func(any) bool
	if len(cont) > 0 {
		c = cont[0]
	}
	return recurse(js.Normalize(value), func(v any) (any, error) { return js.Normalize(fn(v)), nil }, c)
}

// recurse is util.recurse with a fallible fn (config processing can throw).
func recurse(value any, fn func(any) (any, error), cont func(any) bool) (any, error) {
	return recurseIn(value, fn, cont, nil, "")
}

func recurseIn(value any, fn func(any) (any, error), cont func(any) bool, objs []any, path string) (any, error) {
	for _, o := range objs {
		if o == value {
			e := js.NewError("Circular reference detected (" + path + ")")
			e.Props = js.ObjectOf("path", path)
			return nil, e
		}
	}
	if cont != nil && !cont(value) {
		return value, nil
	}
	switch x := value.(type) {
	case *js.Array:
		out := make([]any, len(x.Items))
		for i, item := range x.Items {
			v, err := recurseIn(item, fn, cont, append(objs[:len(objs):len(objs)], x), path+"["+strconv.Itoa(i)+"]")
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return js.NewArray(out...), nil
	case *js.Object:
		out := js.NewObject()
		for _, key := range x.Keys() {
			step := "." + key
			if hasNonWordChar(key) {
				step = `["` + key + `"]`
			}
			v, err := recurseIn(x.Get(key), fn, cont, append(objs[:len(objs):len(objs)], x), path+step)
			if err != nil {
				return nil, err
			}
			out.Set(key, v)
		}
		return out, nil
	}
	return fn(value)
}

// hasNonWordChar is /\W/.test(s).
func hasNonWordChar(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return !isWordChar(r) })
}

// NamespaceGet is grunt.util.namespace.get (getobject): the value at a dotted path
// ("a.b", with "\." for a literal dot) or a list of keys, creating missing objects
// on the way when create is true.
func (u *Util) NamespaceGet(obj any, parts any, create ...bool) any {
	return namespaceGet(js.Normalize(obj), js.Normalize(parts), len(create) > 0 && create[0])
}

// NamespaceSet is grunt.util.namespace.set: it stores value at the dotted path,
// creating intermediate objects, and returns value (undefined when nothing was set,
// such as for a path through "__proto__").
func (u *Util) NamespaceSet(obj any, parts string, value any) any {
	return namespaceSet(js.Normalize(obj), parts, js.Normalize(value))
}

// NamespaceExists is grunt.util.namespace.exists.
func (u *Util) NamespaceExists(obj any, parts string) bool {
	keys := namespaceParts(parts)
	prop := keys[len(keys)-1]
	o := namespaceGet(js.Normalize(obj), keysArray(keys[:len(keys)-1]), false)
	return isObject(o) && jsrt.HasProperty(o, prop)
}

// namespaceParts is getobject's getParts: split on dots not preceded by a backslash.
func namespaceParts(s string) []string {
	s = strings.ReplaceAll(s, `\.`, "￿")
	parts := strings.Split(s, ".")
	for i, p := range parts {
		parts[i] = strings.ReplaceAll(p, "￿", ".")
	}
	return parts
}

func keysArray(keys []string) *js.Array {
	items := make([]any, len(keys))
	for i, k := range keys {
		items[i] = k
	}
	return js.NewArray(items...)
}

// isObject is `typeof v === 'object' && v`.
func isObject(v any) bool { return js.IsObject(v) && js.TypeOf(v) == "object" }

// namespaceGet is getobject.get. parts is a dotted string or an array-like list of
// keys; anything else has no keys, so obj itself is returned.
func namespaceGet(obj, parts any, create bool) any {
	var keys []any
	switch p := parts.(type) {
	case string:
		for _, k := range namespaceParts(p) {
			keys = append(keys, k)
		}
	case *js.Array:
		keys = p.Items
	}
	for len(keys) > 0 && isObject(obj) {
		part := js.ToString(keys[0])
		keys = keys[1:]
		v := jsrt.GetProperty(obj, part)
		if create && !jsrt.HasProperty(obj, part) {
			v = js.NewObject()
			js.SetProperty(obj, part, v)
		}
		obj = v
	}
	return obj
}

// namespaceSet is getobject.set.
func namespaceSet(obj any, parts string, value any) any {
	keys := namespaceParts(parts)
	for _, k := range keys {
		// Do not allow setting __proto__ (CVE-2020-28282).
		if k == "__proto__" {
			return js.Undefined
		}
	}
	prop := keys[len(keys)-1]
	obj = namespaceGet(obj, keysArray(keys[:len(keys)-1]), true)
	if !isObject(obj) {
		return js.Undefined
	}
	js.SetProperty(obj, prop, value)
	return value
}
