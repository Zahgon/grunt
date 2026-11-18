package grunt

import (
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsregexp"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/internal/lodash"
	"github.com/gruntjs/grunt-go/js"
)

// Config is grunt.config: the project configuration, read with dotted property
// paths ("pkg.name", or ["a.b", "c"] for keys that contain dots) and processed as
// templates on the way out.
type Config struct {
	g    *Grunt
	data any
	// depth counts nested Get calls, which recurse through "<%= prop %>" values.
	depth int
}

// maxGetDepth is how deep configuration values may refer to each other before the
// lookup fails like the original's call stack overflow (a value that refers to
// itself, such as cyc: '<%= cyc %>').
const maxGetDepth = 2000

// propStringTmplRe matches '<%= FOO %>' where FOO is a property path such as foo or
// foo.bar, but not a method call like foo() (lib/grunt/config.js).
var propStringTmplRe = mustCompile(`^<%=\s*([a-z0-9_$]+(?:\.[a-z0-9_$]+)*)\s*%>$`, "i")

func mustCompile(source, flags string) *jsregexp.Regexp {
	re, err := jsregexp.Compile(source, flags)
	if err != nil {
		panic(err)
	}
	return re
}

// Data is grunt.config.data, the raw configuration.
func (c *Config) Data() any { return c.data }

// Init is grunt.config.init (grunt.initConfig): it replaces the configuration
// (with an empty object for a falsy value) and returns it.
func (c *Config) Init(obj any) any {
	c.g.Verbose.Write("Initializing config...").Ok()
	obj = js.Normalize(obj)
	if !js.ToBoolean(obj) {
		obj = js.NewObject()
	}
	c.data = obj
	return obj
}

// Escape escapes the dots of a key ("a.b" becomes "a\.b") so that it is not split.
func (c *Config) Escape(s string) string { return strings.ReplaceAll(s, ".", `\.`) }

// GetPropString is grunt.config.getPropString: a list of keys joined with dots,
// each escaped; a string is returned as it is.
func (c *Config) GetPropString(prop any) string { return js.ToString(c.propString(js.Normalize(prop))) }

func (c *Config) propString(prop any) any {
	a, ok := prop.(*js.Array)
	if !ok {
		return prop
	}
	parts := make([]string, len(a.Items))
	for i, p := range a.Items {
		parts[i] = c.Escape(js.ToString(p))
	}
	return strings.Join(parts, ".")
}

// GetRaw is grunt.config.getRaw: the unprocessed value at prop (a dotted string or
// a list of keys), or the whole configuration without prop.
func (c *Config) GetRaw(prop ...any) any {
	if len(prop) == 0 || !js.ToBoolean(js.Normalize(prop[0])) {
		return c.data
	}
	return namespaceGet(c.data, c.propString(js.Normalize(prop[0])), false)
}

// Get is grunt.config.get (grunt.config(prop)): the value at prop with every
// template string in it processed. A configuration value that contains itself
// panics with the "Circular reference detected" error the original throws.
func (c *Config) Get(prop ...any) any {
	c.depth++
	defer func() { c.depth-- }()
	if c.depth > maxGetDepth {
		panic(js.NewRangeError("Maximum call stack size exceeded"))
	}
	return c.Process(c.GetRaw(prop...))
}

// Process is grunt.config.process: it processes every string in raw (recursing
// into arrays and objects) as a template with the configuration as data. A string
// that is just "<%= prop %>" is replaced by the value of prop itself, which may be
// an object or array, unless that is null or undefined.
func (c *Config) Process(raw any) any {
	v, err := recurse(js.Normalize(raw), func(value any) (any, error) {
		s, ok := value.(string)
		if !ok {
			return value, nil
		}
		u := jsstr.ToUTF16(s)
		if m := propStringTmplRe.Exec(u, 0); m != nil {
			if result := c.Get(jsstr.FromUTF16(u[m[2]:m[3]])); !js.IsNullish(result) {
				return result, nil
			}
		}
		return c.g.Template.Process(s, js.ObjectOf("data", c.data)), nil
	}, nil)
	if err != nil {
		panic(err)
	}
	return v
}

// Set is grunt.config.set (grunt.config(prop, value)): it stores value at prop,
// creating intermediate objects, and returns it.
func (c *Config) Set(prop any, value any) any {
	return namespaceSet(c.data, js.ToString(c.propString(js.Normalize(prop))), js.Normalize(value))
}

// Merge is grunt.config.merge: it deep-merges obj into the configuration and
// returns the configuration.
func (c *Config) Merge(obj any) any {
	if data, ok := c.data.(*js.Object); ok {
		lodash.Merge(data, js.Normalize(obj))
	}
	return c.data
}

// Requires is grunt.config.requires: it fails, after logging "Unable to process
// task.", unless every property (dotted string or list of keys) is set to
// something other than null or undefined.
func (c *Config) Requires(props ...any) error {
	g := c.g
	names := make([]string, len(props))
	for i, p := range props {
		names[i] = js.ToString(c.propString(js.Normalize(p)))
	}
	n := float64(len(names))
	msg := "Verifying propert" + pluralize(n, "y/ies") + " " + g.Log.Wordlist(names) + " exist" +
		pluralize(n, "s") + " in config..."
	g.Verbose.Write(msg)
	var failed []string
	dataSet := js.ToBoolean(c.data)
	if dataSet {
		for _, name := range names {
			var v any
			if err := throws(func() { v = c.Get(name) }); err != nil {
				return err
			}
			if js.IsNullish(v) {
				failed = append(failed, `"`+name+`"`)
			}
		}
	}
	if dataSet && len(failed) == 0 {
		g.Verbose.Ok()
		return nil
	}
	g.Verbose.Or.Write(msg)
	g.Log.Error().Error("Unable to process task.")
	if !dataSet {
		return js.NewError("Unable to load config.")
	}
	return js.NewError("Required config propert" + pluralize(float64(len(failed)), "y/ies") + " " +
		strings.Join(failed, ", ") + " missing.")
}
