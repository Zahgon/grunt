package grunt

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gruntjs/grunt-go/internal/dateformat"
	"github.com/gruntjs/grunt-go/internal/jsdate"
	"github.com/gruntjs/grunt-go/internal/jsrt"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// Template is grunt.template: lodash templates ("<%= expr %>", "<%- escaped %>",
// "<% code %>") rendered with the configuration as data, and date formatting.
type Template struct {
	g          *Grunt
	delimiters map[string]*delimiters
	realm      *jsrt.Realm
	// gruntObj is `grunt` as template code sees it.
	gruntObj *js.Object
}

// delimiters is one set of template delimiters (template.addDelimiters).
type delimiters struct {
	opener, closer string
	settings       jsrt.Settings
}

func newTemplate(g *Grunt) *Template {
	t := &Template{
		g:          g,
		delimiters: map[string]*delimiters{},
		realm:      jsrt.NewRealm(jsrt.Options{Now: g.proc.now}),
	}
	// The underscore default template syntax is a sane default for the config system.
	t.AddDelimiters("config", "<%", "%>")
	return t
}

// wallClock is Date.now().
func wallClock() float64 { return float64(time.Now().UnixMilli()) }

// AddDelimiters registers a named set of delimiters, such as ("curly", "{%", "%}"),
// for the delimiters option of Process.
func (t *Template) AddDelimiters(name, opener, closer string) {
	t.delimiters[name] = &delimiters{opener: opener, closer: closer, settings: jsrt.Delimiters(opener, closer)}
}

// lookup returns the named delimiters, or the "config" ones for an unknown name.
func (t *Template) lookup(name any) *delimiters {
	if n, ok := name.(string); ok {
		if d, ok := t.delimiters[n]; ok {
			return d
		}
	}
	return t.delimiters["config"]
}

// SetDelimiters is grunt.template.setDelimiters: it returns the named delimiters
// (the "config" ones for an unknown name) as {opener, closer, lodash: {evaluate,
// interpolate, escape}}.
func (t *Template) SetDelimiters(name string) *js.Object {
	d := t.lookup(name)
	re := func(source string) *js.RegExp { return &js.RegExp{Source: source, Flags: "g"} }
	return js.ObjectOf("opener", d.opener, "closer", d.closer, "lodash", js.ObjectOf(
		"evaluate", re(d.settings.Evaluate),
		"interpolate", re(d.settings.Interpolate),
		"escape", re(d.settings.Escape),
	))
}

// Date is grunt.template.date (dateformat 4.6.3): date (a js.Date, milliseconds or a
// date string) formatted with a mask or mask name. An invalid date is a TypeError.
func (t *Template) Date(date any, format string) (string, error) {
	return dateformat.Format(js.Normalize(date), format, false, false, t.g.proc.now())
}

// Today is grunt.template.today: the current date formatted with Date. When the
// SOURCE_DATE_EPOCH environment variable is set, "now" is that time instead.
func (t *Template) Today(format string) (string, error) {
	return t.today(format)
}

func (t *Template) today(format any) (string, error) {
	now := t.g.proc.now()
	date := js.Date{Ms: now}
	if epoch, ok := t.g.proc.getenv("SOURCE_DATE_EPOCH"); ok && epoch != "" {
		// new Date(SOURCE_DATE_EPOCH * 1000 + now.getTimezoneOffset() * 60000): the
		// epoch's UTC wall-clock time read as local time.
		ms := js.StringToNumber(epoch) * 1000
		if !math.IsNaN(now) {
			ms += jsdate.TimezoneOffset(now) * 60000
		}
		date = js.Date{Ms: jsdate.TimeClip(ms)}
	}
	return dateformat.Format(date, format, false, false, now)
}

// Process is grunt.template.process(tmpl, options): it renders tmpl with
// options.data (the configuration by default; `grunt` is available too) and
// options.delimiters (a name given to AddDelimiters), over and over while the
// result still contains template tags and keeps changing. Line endings are
// normalized to "\n". A failing template is reported with grunt.warn ("An error
// occurred while processing a template (...)", exit code 4), and with --force the
// text rendered so far is returned.
func (t *Template) Process(tmpl string, options ...any) string {
	g := t.g
	opts := optionsObject(options, 0)
	d := t.lookup(opts.Get("delimiters"))
	// Clone data, initializing to config data or an empty object if omitted.
	data := opts.Get("data")
	if !js.ToBoolean(data) {
		data = g.Config.data
	}
	if !js.ToBoolean(data) {
		data = js.NewObject()
	}
	if !isObject(data) {
		panic(js.NewTypeError("Object prototype may only be an Object or null: " + js.ToString(data)))
	}
	own := js.NewObject()
	// Expose grunt, unless that conflicts with an existing .grunt property.
	if !jsrt.HasProperty(data, "grunt") {
		own.Set("grunt", t.gruntObject())
	}
	obj := jsrt.WithProto(own, data)
	settings := t.settings(d, opts)
	last := tmpl
	for strings.Contains(tmpl, d.opener) {
		out, err := t.render(tmpl, settings, obj)
		if err != nil {
			t.fail(err, tmpl)
			break
		}
		tmpl = out
		// Nothing left to process if the template didn't change.
		if tmpl == last {
			break
		}
		last = tmpl
	}
	return normalizeLF(jsstr.ToWellFormed(tmpl))
}

// render is _.template(tmpl, options)(data).
func (t *Template) render(tmpl string, settings jsrt.Settings, data any) (string, error) {
	compiled, err := jsrt.CompileTemplate(tmpl, settings)
	if err != nil {
		return "", err
	}
	return compiled.Execute(data)
}

// settings are the _.template options: grunt's delimiters, under the escape,
// evaluate, interpolate, variable and imports options the caller passed.
func (t *Template) settings(d *delimiters, opts *js.Object) jsrt.Settings {
	s := d.settings
	s.Realm = t.realm
	s.Imports = js.ObjectOf("_", t.g.underscore)
	for name, field := range map[string]*string{"escape": &s.Escape, "evaluate": &s.Evaluate, "interpolate": &s.Interpolate} {
		if re, ok := opts.Get(name).(*js.RegExp); ok {
			*field = re.Source
		}
	}
	if v := opts.Get("variable"); js.ToBoolean(v) {
		s.Variable = js.ToString(v)
	}
	if imports, ok := opts.Get("imports").(*js.Object); ok {
		merged := imports.Clone()
		if !merged.Has("_") {
			merged.Set("_", t.g.underscore)
		}
		s.Imports = merged
	}
	return s
}

// fail is template.process's catch block: it rewrites the error's message and
// warns with it.
func (t *Template) fail(err error, tmpl string) {
	g := t.g
	// Since lodash (and Underscore.js 1.3.3), \n and \r inside template tags throw.
	if js.ToString(asJSError(err)) == "SyntaxError: Unexpected token ILLEGAL" && strings.ContainsAny(tmpl, "\n\r") {
		g.Log.Errorlns("A special character was detected in this template. " +
			`Inside template tags, the \n and \r special characters must be ` +
			`escaped as \\n and \\r. (grunt 0.4.0+)`)
	}
	var thrown *jsrt.Thrown
	if errors.As(err, &thrown) {
		// e.message = ...: a thrown object gets the property; a primitive cannot
		// hold one, which is a TypeError in strict mode.
		o, ok := thrown.Value.(*js.Object)
		if !ok {
			panic(js.NewTypeError(setMessageError(thrown.Value)))
		}
		msg := jsrt.GetProperty(o, "message")
		o.Set("message", "An error occurred while processing a template ("+js.ToString(msg)+").")
		g.Warn(o, TemplateError)
		return
	}
	e := asJSError(err)
	e.Message = "An error occurred while processing a template (" + e.Message + ")."
	g.Warn(e, TemplateError)
}

// setMessageError is V8's message for assigning e.message on a primitive.
func setMessageError(v any) string {
	if js.IsNullish(v) {
		return "Cannot set properties of " + js.ToString(v) + " (setting 'message')"
	}
	s := js.ToString(v)
	if _, ok := v.(string); ok {
		s = strconv.Quote(s)
		s = "'" + s[1:len(s)-1] + "'"
	} else {
		s = "'" + s + "'"
	}
	return "Cannot create property 'message' on " + js.TypeOf(v) + " " + s
}

// gruntObject is the `grunt` that template code can use: grunt.version,
// grunt.package, grunt.template.{date,today,process}, grunt.config.{get,getRaw,
// process,data}, grunt.option, grunt.util.{kindOf,pluralize,repeat,normalizelf,_}
// and grunt.log.{write,writeln}.
func (t *Template) gruntObject() *js.Object {
	g := t.g
	if t.gruntObj == nil {
		fn := func(name string, length int, call func(args []any) (any, error)) *js.Function {
			return &js.Function{Name: name, Length: length, Call: func(_ any, args []any) (any, error) { return call(args) }}
		}
		arg := func(args []any, i int) any {
			if i < len(args) {
				return args[i]
			}
			return js.Undefined
		}
		str := func(args []any, i int) string { return js.ToString(arg(args, i)) }
		ok := func(v any) (any, error) { return v, nil }
		t.gruntObj = js.ObjectOf(
			"version", g.Version,
			"package", g.Package,
			"template", js.ObjectOf(
				"date", fn("dateFormat", 4, func(a []any) (any, error) {
					return dateformat.Format(arg(a, 0), arg(a, 1), js.ToBoolean(arg(a, 2)), js.ToBoolean(arg(a, 3)), g.proc.now())
				}),
				"today", fn("today", 1, func(a []any) (any, error) { return t.today(arg(a, 0)) }),
				"process", fn("process", 2, func(a []any) (any, error) { return ok(t.Process(str(a, 0), arg(a, 1))) }),
			),
			"config", js.ObjectOf(
				"get", fn("get", 1, func(a []any) (any, error) { return ok(g.Config.Get(a...)) }),
				"getRaw", fn("getRaw", 1, func(a []any) (any, error) { return ok(g.Config.GetRaw(a...)) }),
				"process", fn("process", 1, func(a []any) (any, error) { return ok(g.Config.Process(arg(a, 0))) }),
			),
			"option", fn("option", 2, func(a []any) (any, error) {
				return ok(g.Option(str(a, 0), a[min(len(a), 1):]...))
			}),
			"util", js.ObjectOf(
				"kindOf", fn("kindOf", 1, func(a []any) (any, error) { return ok(kindOf(arg(a, 0))) }),
				"pluralize", fn("pluralize", 3, func(a []any) (any, error) {
					sep := ""
					if js.ToBoolean(arg(a, 2)) {
						sep = str(a, 2)
					}
					return ok(pluralize(arg(a, 0), str(a, 1), sep))
				}),
				"repeat", fn("repeat", 2, func(a []any) (any, error) {
					s := ""
					if js.ToBoolean(arg(a, 1)) {
						s = str(a, 1)
					}
					return ok(g.Util.Repeat(int(js.ToNumber(arg(a, 0))), s))
				}),
				"normalizelf", fn("normalizelf", 1, func(a []any) (any, error) { return ok(normalizeLF(str(a, 0))) }),
				"_", g.underscore,
			),
			"log", js.ObjectOf(
				"write", fn("write", 0, func(a []any) (any, error) { g.Log.Write(a...); return js.Undefined, nil }),
				"writeln", fn("writeln", 0, func(a []any) (any, error) { g.Log.Writeln(a...); return js.Undefined, nil }),
			),
		)
	}
	// grunt.config.data is the current configuration.
	t.gruntObj.Get("config").(*js.Object).Set("data", g.Config.data)
	return t.gruntObj
}
