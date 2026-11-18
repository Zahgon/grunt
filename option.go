package grunt

import (
	"strings"

	"github.com/gruntjs/grunt-go/js"
)

// option is lib/grunt/option.js: the command-line option data.
type option struct {
	data *js.Object
}

func newOption() *option { return &option{data: js.NewObject()} }

// get is option(key): a "no-" key reports whether the option is exactly false.
func (o *option) get(key string) any {
	// key.match(/^no-(.+)$/): "." does not match line terminators.
	if rest, ok := strings.CutPrefix(key, "no-"); ok && rest != "" && !strings.ContainsAny(rest, "\n\r  ") {
		return o.data.Get(rest) == false
	}
	return o.data.Get(key)
}

// truthy is !!option(key).
func (o *option) truthy(key string) bool { return js.ToBoolean(o.get(key)) }

// Option is grunt.option(key) and grunt.option(key, value): with a value it sets the
// option and returns the value, otherwise it returns the option's value. For a key
// "no-x" it reports whether option x is exactly false.
func (g *Grunt) Option(key string, value ...any) any {
	if len(value) > 0 {
		v := js.Normalize(value[0])
		g.option.data.Set(key, v)
		return v
	}
	return g.option.get(key)
}

// OptionInit is grunt.option.init: it replaces the option data with obj (an empty
// object when obj is not an object) and returns it.
func (g *Grunt) OptionInit(obj any) *js.Object {
	o, ok := js.Normalize(obj).(*js.Object)
	if !ok {
		o = js.NewObject()
	}
	g.option.data = o
	return o
}

// OptionFlags is grunt.option.flags: the options as command-line flags, skipping
// empty arrays.
func (g *Grunt) OptionFlags() []string {
	var flags []string
	for _, key := range g.option.data.Keys() {
		val := g.option.data.Get(key)
		if a, ok := val.(*js.Array); ok && len(a.Items) == 0 {
			continue
		}
		flag := "--"
		if val == false {
			flag += "no-"
		}
		flag += key
		if _, ok := val.(bool); !ok {
			flag += "=" + js.ToString(val)
		}
		flags = append(flags, flag)
	}
	return flags
}

// OptionKeys is grunt.option.keys.
func (g *Grunt) OptionKeys() []string { return g.option.data.Keys() }
