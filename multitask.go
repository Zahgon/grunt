package grunt

import (
	"strings"

	"github.com/gruntjs/grunt-go/internal/lodash"
	"github.com/gruntjs/grunt-go/js"
)

// This file is the multi-task half of lib/grunt/task.js: registerMultiTask, targets
// and the normalization of their files.

// RegisterMultiTask is grunt.registerMultiTask(name, [info,] fn): a task run once
// per target of its configuration, or for the target named as its first argument.
func (r *TaskRunner) RegisterMultiTask(name string, rest ...any) *TaskRunner {
	info, fn, shuffled := splitInfo(rest)
	if shuffled {
		info = "Custom multi task."
	}
	body, ok := taskBody(fn)
	if !ok {
		body = func(*Task) any { return js.NewTypeError("fn.apply is not a function") }
	}
	var entry *taskEntry
	entry = r.registerTask(name, info, TaskFunc(func(t *Task) any {
		return r.runMultiTask(entry.name, t, body)
	}))
	entry.multi = true
	return r
}

// isValidMultiTaskTarget rejects targets that start with "_" or are "options".
func isValidMultiTaskTarget(target string) bool {
	return !strings.HasPrefix(target, "_") && target != "options"
}

// runMultiTask is the task function registerMultiTask registers.
func (r *TaskRunner) runMultiTask(name string, t *Task, body func(t *Task) any) any {
	g := r.g
	target := ""
	if len(t.Args) > 0 {
		target = t.Args[0]
		t.Args = t.Args[1:]
	} else {
		t.Args = []string{}
	}
	// If a target wasn't specified, run this task once for each target.
	if target == "" || target == "*" {
		return r.runAllTargets(name, t.Args)
	}
	if !isValidMultiTaskTarget(target) {
		return js.NewError(`Invalid target "` + target + `" specified.`)
	}
	// Fail if any required config properties have been omitted.
	if err := t.RequiresConfig(js.NewArray(name, target)); err != nil {
		return err
	}
	t.options = func(defaults []any) *js.Object {
		targetObj := g.Config.Get(js.NewArray(name, target))
		var targetOptions any = js.NewObject()
		if o, ok := targetObj.(*js.Object); ok {
			targetOptions = o.Get("options")
		}
		return r.options(append(defaults, g.Config.Get(js.NewArray(name, "options")), targetOptions))
	}
	t.Target = target
	// Recreate flags so that the target isn't set as a flag.
	t.Flags = flagsOf(t.Args)
	t.Data = g.Config.Get(js.NewArray(name, target))
	t.Files = r.NormalizeMultiTaskFiles(t.Data, target)
	return body(t)
}

// runAllTargets runs the task once for every valid target of its configuration.
func (r *TaskRunner) runAllTargets(name string, args []string) any {
	var targets []string
	raw := r.g.Config.GetRaw(name)
	if js.ToBoolean(raw) {
		targets = js.OwnKeys(raw)
	}
	var valid []string
	for _, t := range targets {
		if isValidMultiTaskTarget(t) {
			valid = append(valid, t)
		}
	}
	if len(valid) == 0 {
		r.g.Log.Error(`No "` + name + `" targets found.`)
		return false
	}
	for _, target := range valid {
		r.Run(strings.Join(append([]string{name, target}, args...), ":"))
	}
	return nil
}

// FileMapping is one normalized src-dest mapping of a multi task (an element of
// this.files).
type FileMapping struct {
	// Dest is the destination path: a string, or js.Undefined when the mapping has
	// none (see HasDest).
	Dest any
	// Orig is a copy of the mapping as it was configured (.orig).
	Orig *js.Object
	// Props holds every other property of the mapping, such as expand options or
	// custom properties, in their configured order.
	Props *js.Object

	hasSrc, hasDest bool
	// src is the expanded list once known (always for expand mappings).
	src *js.Array
	// expand computes src on first use: the configured patterns are expanded only
	// when the task reads them, as the original's memoizing getter does.
	expand func() *js.Array
}

// HasSrc reports whether the mapping has a src property.
func (f *FileMapping) HasSrc() bool { return f.hasSrc }

// HasDest reports whether the mapping has a dest property.
func (f *FileMapping) HasDest() bool { return f.hasDest }

// Src is the mapping's source files, expanded from its patterns on first use.
// A mapping without src has none.
func (f *FileMapping) Src() []string {
	a, ok := f.srcValue().(*js.Array)
	if !ok {
		return nil
	}
	out := make([]string, len(a.Items))
	for i, s := range a.Items {
		out[i] = js.ToString(s)
	}
	return out
}

// srcValue is f.src as JavaScript sees it: an array, or undefined.
func (f *FileMapping) srcValue() any {
	if !f.hasSrc {
		return js.Undefined
	}
	if f.src == nil {
		f.src = f.expand()
	}
	return f.src
}

// NormalizeMultiTaskFiles is grunt.task.normalizeMultiTaskFiles: it turns a
// target's configuration, in any of the src-dest, files-object, files-array or
// compact formats, into file mappings, expanding "expand: true" entries into one
// mapping per destination. Template strings in destination keys are processed.
func (r *TaskRunner) NormalizeMultiTaskFiles(data any, target string) []*FileMapping {
	g := r.g
	data = js.Normalize(data)
	var files []*js.Object
	if obj, ok := data.(*js.Object); ok {
		filesProp := obj.Get("files")
		if obj.Has("src") || obj.Has("dest") {
			o := js.NewObject()
			for _, k := range obj.Keys() {
				if k != "options" {
					o.Set(k, obj.Get(k))
				}
			}
			files = append(files, o)
		} else if fo, ok := filesProp.(*js.Object); ok {
			files = append(files, r.destKeyedFiles(fo)...)
		} else if fa, ok := filesProp.(*js.Array); ok {
			for _, item := range lodash.FlattenDeep(fa).Items {
				o, ok := item.(*js.Object)
				if !ok {
					// 'src' in obj, for a primitive.
					panic(js.NewTypeError("Cannot use 'in' operator to search for 'src' in " + js.ToString(item)))
				}
				if o.Has("src") || o.Has("dest") {
					files = append(files, o)
				} else {
					files = append(files, r.destKeyedFiles(o)...)
				}
			}
		}
	} else if kindOf(data) != "object" {
		files = append(files, js.ObjectOf("src", data, "dest", g.Config.Process(target)))
	}

	// If no src/dest or files were specified, there are no files.
	if len(files) == 0 {
		g.Verbose.Writeln("File: " + g.Log.style("[no files]", "yellow"))
		return []*FileMapping{}
	}

	// Normalize .src properties to flattened arrays.
	for _, obj := range files {
		src := obj.Get("src")
		if !obj.Has("src") || !js.ToBoolean(src) {
			continue
		}
		if a, ok := src.(*js.Array); ok {
			obj.Set("src", lodash.Flatten(a))
		} else {
			obj.Set("src", js.NewArray(src))
		}
	}

	var mappings []*FileMapping
	for _, obj := range files {
		mappings = append(mappings, r.fileMappings(obj)...)
	}

	// Log the src and dest of each mapping when --verbose is specified.
	if g.option.truthy("verbose") {
		for _, f := range mappings {
			var output []string
			if f.hasSrc {
				if src := f.Src(); len(src) > 0 {
					output = append(output, g.Log.Wordlist(src))
				} else {
					output = append(output, g.Log.style("[no src]", "yellow"))
				}
			}
			if f.hasDest {
				if js.ToBoolean(f.Dest) {
					output = append(output, "-> "+g.Log.style(js.ToString(f.Dest), "cyan"))
				} else {
					output = append(output, "-> "+g.Log.style("[no dest]", "yellow"))
				}
			}
			if len(output) > 0 {
				g.Verbose.Writeln("Files: " + strings.Join(output, " "))
			}
		}
	}
	return mappings
}

// destKeyedFiles turns {dest: src, ...} into {src, dest} objects, processing each
// destination as a template.
func (r *TaskRunner) destKeyedFiles(o *js.Object) []*js.Object {
	var files []*js.Object
	for _, dest := range o.Keys() {
		files = append(files, js.ObjectOf("src", o.Get(dest), "dest", r.g.Config.Process(dest)))
	}
	return files
}

// fileMappings builds the mappings of one normalized files object.
func (r *TaskRunner) fileMappings(obj *js.Object) []*FileMapping {
	g := r.g
	// Expansion options: the object without src and dest.
	expandOptions := lodash.Extend(js.NewObject(), obj)
	expandOptions.Delete("src")
	expandOptions.Delete("dest")

	if js.ToBoolean(obj.Get("expand")) {
		var out []*FileMapping
		for _, m := range g.File.ExpandMapping(obj.Get("src"), obj.Get("dest"), expandOptions) {
			result := lodash.Extend(js.NewObject(), obj)
			result.Set("src", g.Config.Process(m.Get("src")))
			result.Set("dest", g.Config.Process(m.Get("dest")))
			for _, prop := range []string{"expand", "cwd", "flatten", "rename", "ext"} {
				result.Delete(prop)
			}
			f := newFileMapping(result, obj)
			src, _ := result.Get("src").(*js.Array)
			f.src = src
			out = append(out, f)
		}
		return out
	}

	f := newFileMapping(obj, obj)
	f.expand = func() *js.Array {
		src := obj.Get("src")
		if a, ok := src.(*js.Array); ok {
			src = lodash.Flatten(a)
		} else {
			src = js.NewArray(src)
		}
		return stringsArray(g.File.Expand(expandOptions, src))
	}
	return []*FileMapping{f}
}

// newFileMapping splits a mapping object into its src, dest and other properties.
func newFileMapping(result, orig *js.Object) *FileMapping {
	f := &FileMapping{
		Dest:    js.Undefined,
		Orig:    lodash.Extend(js.NewObject(), orig),
		Props:   js.NewObject(),
		hasSrc:  result.Has("src"),
		hasDest: result.Has("dest"),
	}
	if f.hasDest {
		f.Dest = result.Get("dest")
	}
	for _, k := range result.Keys() {
		if k != "src" && k != "dest" {
			f.Props.Set(k, result.Get(k))
		}
	}
	return f
}

// stringsArray converts a Go string list to an array.
func stringsArray(list []string) *js.Array {
	items := make([]any, len(list))
	for i, s := range list {
		items[i] = s
	}
	return js.NewArray(items...)
}
