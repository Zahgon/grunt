package grunt

import (
	"strings"

	"github.com/gruntjs/grunt-go/internal/lodash"
	"github.com/gruntjs/grunt-go/js"
)

// TaskFunc is a task body; t is `this` inside a JavaScript task function, and the
// positional task arguments ("build:a:b" gives "a" and "b") are t.Args.
//
// Returning nil or true is success; false fails the task generically
// (`Task "x" failed.`); an error fails it with that error, as a throw does in
// JavaScript. Any other value counts as success, as a thrown non-Error value does
// in grunt. A panic with a value is treated like a JavaScript throw of that value.
// After t.Async() the return value is ignored and the task ends when the function
// Async returned is called.
type TaskFunc func(t *Task) any

// Task is the context of a running task (`this` inside a task function).
type Task struct {
	// Name is the task's name and NameArgs the name with its arguments as given
	// ("build:a:b").
	Name, NameArgs string
	// Args are the positional arguments, without the target for multi tasks.
	Args []string
	// Flags has each argument as a key with the value true.
	Flags *js.Object
	// Target, Data and Files are set for multi tasks: the target name, its
	// configuration data (grunt.config([name, target])) and its normalized files.
	Target string
	Data   any
	Files  []*FileMapping

	runner  *TaskRunner
	async   func() func(result any)
	options func(defaults []any) *js.Object
}

// Async is this.async(): the task becomes asynchronous and ends when the returned
// function is called, with nil, true or undefined for success, false or an error for
// failure. Only the first call of the returned function counts. It may be called
// from any goroutine.
func (t *Task) Async() func(result any) { return t.async() }

// Options is this.options(defaults...): the defaults, overridden by the task's
// "options" configuration and, for a multi task, by the target's "options".
func (t *Task) Options(defaults ...any) *js.Object {
	for i, d := range defaults {
		defaults[i] = js.Normalize(d)
	}
	return t.options(defaults)
}

// FilesSrc is this.filesSrc: the src lists of t.Files flattened and without
// duplicates; a mapping without src contributes undefined.
func (t *Task) FilesSrc() *js.Array {
	srcs := make([]any, len(t.Files))
	for i, f := range t.Files {
		srcs[i] = f.srcValue()
	}
	return lodash.Uniq(lodash.Flatten(js.NewArray(srcs...)))
}

// ErrorCount is this.errorCount: the number of grunt.log.error calls since the most
// recently started task began.
func (t *Task) ErrorCount() int { return t.runner.g.Fail.errorcount - t.runner.errorcount }

// Requires is this.requires (grunt.task.requires).
func (t *Task) Requires(names ...any) error { return t.runner.Requires(names...) }

// RequiresConfig is this.requiresConfig (grunt.config.requires).
func (t *Task) RequiresConfig(props ...any) error { return t.runner.g.Config.Requires(props...) }

// taskEntry is a registered task (an entry of grunt.task._tasks).
type taskEntry struct {
	name string
	info string
	// fn runs the task with its context.
	fn func(t *Task) any
	// alias is set for alias tasks, which log their header only in verbose mode.
	alias bool
	// multi and init mark multi tasks and init tasks.
	multi, init bool
	// filepath is the file the task was registered from (meta.filepath), or
	// undefined for tasks registered while no task file was loading.
	filepath any
}

// queueItem is a task+args object of the queue, or the placeholder or the marker.
type queueItem struct {
	task     *taskEntry
	nameArgs string
	args     []string
	flags    *js.Object
}

// registry tracks the tasks registered and renamed while a task file loads.
type registry struct {
	tasks, untasks []string
	filepath       any
}

// TaskRunner is grunt.task: task registration and the task queue (lib/util/task.js
// extended by lib/grunt/task.js).
type TaskRunner struct {
	g *Grunt
	// tasks maps names to *taskEntry in JavaScript key order.
	tasks       *js.Object
	queue       []*queueItem
	placeholder *queueItem
	marker      *queueItem
	running     bool
	success     map[string]bool
	current     *Task
	// onError and onDone are the error and done options of task.options.
	onError func(err error)
	onDone  func()
	// errorcount is grunt.fail.errorcount when the current task started.
	errorcount int
	registry   *registry
	// loadStack saves the registries of the task files being loaded.
	loadStack []*registry
}

func newTaskRunner(g *Grunt) *TaskRunner {
	return &TaskRunner{
		g:           g,
		tasks:       js.NewObject(),
		placeholder: &queueItem{},
		marker:      &queueItem{},
		success:     map[string]bool{},
		registry:    &registry{filepath: js.Undefined},
	}
}

func (r *TaskRunner) lookup(name string) *taskEntry {
	t, _ := r.tasks.Get(name).(*taskEntry)
	return t
}

// throwIfRunning is _throwIfRunning: inside the runner (or without an error
// handler) the error is thrown; otherwise the error handler reports it.
func (r *TaskRunner) throwIfRunning(err error) {
	if r.running || r.onError == nil {
		panic(err)
	}
	r.onError(err)
}

// parseArgs is Task.prototype.parseArgs: the first argument when it is an array,
// otherwise all arguments.
func parseArgs(args []any) []any {
	if len(args) > 0 {
		if a, ok := js.Normalize(args[0]).(*js.Array); ok {
			return a.Items
		}
	}
	out := make([]any, len(args))
	for i, a := range args {
		out[i] = js.Normalize(a)
	}
	return out
}

// RegisterTask is grunt.registerTask(name, [info,] body). body is a TaskFunc (or a
// func(*Task) any, or a *js.Function called with the arguments), or an alias: a task
// name, or a list of task names, to run in its place. info defaults to "Custom
// task." or to "Alias for ...".
func (r *TaskRunner) RegisterTask(name string, rest ...any) *TaskRunner {
	info, fn, _ := splitInfo(rest)
	r.registerTask(name, info, fn)
	return r
}

// splitInfo is the "if (fn == null) { fn = info; info = null; }" shuffle of the
// optional info argument; shuffled reports that info was omitted.
func splitInfo(rest []any) (info, fn any, shuffled bool) {
	info, fn = js.Undefined, js.Undefined
	if len(rest) > 0 {
		info = rest[0]
	}
	if len(rest) > 1 {
		fn = rest[1]
	}
	if js.IsNullish(fn) {
		return js.Undefined, info, true
	}
	return info, fn, false
}

// taskBody converts a task function value; ok is false for an alias.
func taskBody(fn any) (func(t *Task) any, bool) {
	switch f := fn.(type) {
	case TaskFunc:
		return f, true
	case func(t *Task) any:
		return f, true
	case *js.Function:
		return func(t *Task) any {
			args := make([]any, len(t.Args))
			for i, a := range t.Args {
				args[i] = a
			}
			v, err := f.Call(t, args)
			if err != nil {
				panic(err)
			}
			return v
		}, true
	}
	return nil, false
}

// registerTask is lib/grunt/task.js's registerTask over lib/util/task.js's.
func (r *TaskRunner) registerTask(name string, info, fn any) *taskEntry {
	// Add task to registry.
	r.registry.tasks = append(r.registry.tasks, name)
	entry := &taskEntry{name: name, filepath: r.registry.filepath}
	body, ok := taskBody(fn)
	if !ok {
		// A string or array of strings was passed instead of fn: an alias.
		targets := parseArgs([]any{js.Normalize(fn)})
		body = func(t *Task) any {
			// this.run.bind(this, fn): a string alias passes the task's arguments on
			// as further task names, an array alias ignores them.
			args := []any{js.Normalize(fn)}
			for _, a := range t.Args {
				args = append(args, a)
			}
			r.Run(args...)
			return nil
		}
		entry.alias = true
		if !js.ToBoolean(info) {
			names := make([]string, len(targets))
			for i, t := range targets {
				names[i] = js.ToString(t)
			}
			plural := "s"
			if len(names) == 1 {
				plural = ""
			}
			info = `Alias for "` + strings.Join(names, `", "`) + `" task` + plural + "."
		}
	} else if !js.ToBoolean(info) {
		info = "Custom task."
	}
	entry.info = js.ToString(info)
	entry.fn = r.wrapTask(entry, body)
	r.tasks.Set(name, entry)
	return entry
}

// wrapTask is the task function lib/grunt/task.js installs around every task body:
// it sets up the context, logs the "Running" header and runs the body.
func (r *TaskRunner) wrapTask(entry *taskEntry, body func(t *Task) any) func(t *Task) any {
	return func(t *Task) any {
		g := r.g
		// Guaranteed to always be the actual task name.
		name := entry.name
		r.errorcount = g.Fail.errorcount
		t.options = func(defaults []any) *js.Object {
			return r.options(append(defaults, g.Config.Get(js.NewArray(name, "options"))))
		}
		// An alias, or a multi task called without a target, logs only in verbose mode.
		logger := g.Log
		if entry.alias || entry.multi && (len(t.Args) == 0 || t.Args[0] == "" || t.Args[0] == "*") {
			logger = g.Verbose
		}
		header := `Running "` + t.NameArgs + `"`
		if t.Name != t.NameArgs {
			header += " (" + t.Name + ")"
		}
		logger.Header(header + " task")
		logger.Debug("Task source: " + js.ToString(entry.filepath))
		return body(t)
	}
}

// options is the body of this.options: _.extend({}, sources...), logged in verbose
// mode.
func (r *TaskRunner) options(sources []any) *js.Object {
	opts := lodash.Extend(js.NewObject(), sources...)
	r.g.Verbose.Writeflags(opts, "Options")
	return opts
}

// RegisterInitTask is grunt.registerInitTask: a task that does not need the
// Gruntfile, so running only init tasks skips loading it.
func (r *TaskRunner) RegisterInitTask(name string, rest ...any) *TaskRunner {
	info, fn, _ := splitInfo(rest)
	r.registerTask(name, info, fn).init = true
	return r
}

// RenameTask is grunt.renameTask. Renaming a missing task logs an error.
func (r *TaskRunner) RenameTask(oldname, newname string) *TaskRunner {
	entry := r.lookup(oldname)
	if entry == nil {
		r.g.Log.Error(`Cannot rename missing "` + oldname + `" task.`)
		return r
	}
	r.tasks.Set(newname, entry)
	entry.name = newname
	r.tasks.Delete(oldname)
	r.registry.untasks = append(r.registry.untasks, oldname)
	r.registry.tasks = append(r.registry.tasks, newname)
	return r
}

// Exists reports whether a task is registered.
func (r *TaskRunner) Exists(name string) bool { return r.tasks.Has(name) }

// IsTaskAlias is grunt.task.isTaskAlias. lib/grunt/task.js replaces every task's
// function with a wrapper that does not carry the alias flag of lib/util/task.js,
// so this is false for every registered task; a missing task is a TypeError.
func (r *TaskRunner) IsTaskAlias(name string) bool {
	if r.lookup(name) == nil {
		panic(js.NewTypeError("Cannot read properties of undefined (reading 'fn')"))
	}
	return false
}

// Current is grunt.task.current: the running task, or nil.
func (r *TaskRunner) Current() *Task { return r.current }

// SplitArgs splits "a:b\:c" on colons into ["a", "b:c"], unescaping "\:" and "\\".
func (r *TaskRunner) SplitArgs(s string) []string {
	if s == "" {
		return []string{}
	}
	s = strings.ReplaceAll(s, `\\`, "￿")
	s = strings.ReplaceAll(s, `\:`, "￾")
	parts := strings.Split(s, ":")
	for i, p := range parts {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(p, "￾", ":"), "￿", `\`)
	}
	return parts
}

// taskPlusArgs is _taskPlusArgs: "foo:bar:baz" is task "foo:bar:baz" if it exists,
// else task "foo:bar" with argument "baz", else task "foo" with "bar" and "baz".
func (r *TaskRunner) taskPlusArgs(name string) *queueItem {
	parts := r.SplitArgs(name)
	i := len(parts)
	var task *taskEntry
	for {
		task = r.lookup(strings.Join(parts[:i], ":"))
		if task != nil {
			break
		}
		if i--; i <= 0 {
			break
		}
	}
	// parts.slice(i): i is -1 only when parts is empty.
	args := parts[max(i, 0):]
	return &queueItem{task: task, nameArgs: name, args: args, flags: flagsOf(args)}
}

func flagsOf(args []string) *js.Object {
	flags := js.NewObject()
	for _, a := range args {
		flags.Set(a, true)
	}
	return flags
}
