// Package grunt is grunt 1.6.3, the JavaScript task runner, for gruntfiles written
// in Go. A gruntfile is a Go program whose main function hands its body to Main:
//
//	func main() {
//		grunt.Main(func(g *grunt.Grunt) {
//			g.InitConfig(grunt.O("pkg", grunt.O("name", "demo")))
//			g.RegisterTask("default", grunt.TaskFunc(func(t *grunt.Task) any {
//				g.Log.Writeln("Hello from " + js.ToString(g.Config.Get("pkg.name")))
//				return nil
//			}))
//		})
//	}
//
// The program behaves like `grunt` run by grunt-cli 1.5.0 in the directory of a
// Gruntfile.js with the same body: it parses the same command line, prints the same
// output and exits with the same code.
//
// Every JavaScript method maps to a Go method of the same name in Go case. Values
// follow package js: undefined is js.Undefined, null is nil, numbers are float64,
// arrays *js.Array, objects *js.Object, dates js.Date, functions *js.Function and
// buffers []byte. Parameters of type any also accept Go int, []any, []string and
// map[string]any, converted by js.Normalize. Calls that throw in JavaScript return
// a non-nil error (a *js.Error) in Go; the few that throw only on programming
// errors or exotic input (circular configuration data, an invalid argument type)
// panic with the *js.Error instead, which a task body turns into a task failure
// exactly as a throw does in JavaScript.
package grunt

import (
	"bufio"
	"io"
	"os"
	"sync"

	"github.com/gruntjs/grunt-go/internal/jsrt"
	"github.com/gruntjs/grunt-go/internal/lodash"
	"github.com/gruntjs/grunt-go/js"
)

// Version is grunt.version.
const Version = "1.6.3"

// Grunt is the grunt object a gruntfile receives.
type Grunt struct {
	// Log and Verbose are grunt.log and grunt.verbose.
	Log, Verbose *Log
	// Config is grunt.config.
	Config *Config
	// Task is grunt.task.
	Task *TaskRunner
	// File is grunt.file.
	File *File
	// Template is grunt.template.
	Template *Template
	// Fail is grunt.fail.
	Fail *Fail
	// Event is grunt.event.
	Event *Event
	// Util is grunt.util.
	Util *Util
	// Version is grunt.version ("1.6.3").
	Version string
	// Package is grunt.package, the package metadata.
	Package *js.Object

	option *option
	proc   *process
	// underscore is grunt.util._, shared by the Go API and the templates.
	underscore *js.Object
}

// process is the part of Node's process object grunt uses: the output streams,
// the environment, the clock and process.exit.
type process struct {
	stdout *syncWriter
	stderr io.Writer
	// exit is process.exit: it flushes stdout and ends the program.
	exit func(code int)
	// getenv is process.env lookup.
	getenv func(key string) (string, bool)
	// now is Date.now().
	now func() float64
	// stdoutIsTTY tells whether colour codes can be printed.
	stdoutIsTTY bool
	// installPath is what `--version --verbose` reports as grunt's install path.
	installPath string
}

// consoleWrite writes s to stdout or stderr for console.log and console.error, and
// for Node's report of an uncaught exception. A failed write is not reported: the
// Console Node creates for process.stdout and process.stderr ignores errors from its
// streams (ignoreErrors defaults to true), and the uncaught-exception report is
// written just before the process exits.
func consoleWrite(w io.Writer, s string) {
	_, _ = io.WriteString(w, s)
}

// syncWriter is a buffered stream shared by the main goroutine and goroutines a
// gruntfile starts for asynchronous tasks.
type syncWriter struct {
	mu sync.Mutex
	w  *bufio.Writer
}

func newSyncWriter(w io.Writer) *syncWriter {
	return &syncWriter{w: bufio.NewWriter(w)}
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// Flush writes buffered output to the underlying stream.
func (s *syncWriter) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Flush()
}

// newGrunt builds the grunt object, wiring the modules together the way
// lib/grunt.js requires them.
func newGrunt(proc *process) *Grunt {
	g := &Grunt{Version: Version, proc: proc, option: newOption()}
	g.Package = packageJSON()
	g.underscore = lodash.Underscore()
	g.Util = &Util{g: g}
	g.Fail = &Fail{g: g}
	g.Log = newLog(g)
	g.Verbose = g.Log.Verbose
	g.Event = newEvent(proc.stderr)
	g.File = &File{g: g, DefaultEncoding: "utf8"}
	g.Config = &Config{g: g, data: js.NewObject()}
	g.Template = newTemplate(g)
	g.Task = newTaskRunner(g)
	return g
}

// packageJSON is the part of grunt's package.json that grunt.package exposes.
func packageJSON() *js.Object {
	return js.ObjectOf(
		"name", "grunt",
		"description", "The JavaScript Task Runner",
		"version", Version,
		"author", "Grunt Development Team (https://gruntjs.com/development-team)",
		"homepage", "https://gruntjs.com/",
		"repository", "https://github.com/gruntjs/grunt.git",
		"license", "MIT",
	)
}

// InitConfig is grunt.initConfig (grunt.config.init).
func (g *Grunt) InitConfig(obj any) any { return g.Config.Init(obj) }

// RegisterTask is grunt.registerTask; see TaskRunner.RegisterTask.
func (g *Grunt) RegisterTask(name string, rest ...any) *TaskRunner {
	return g.Task.RegisterTask(name, rest...)
}

// RegisterMultiTask is grunt.registerMultiTask; see TaskRunner.RegisterMultiTask.
func (g *Grunt) RegisterMultiTask(name string, rest ...any) *TaskRunner {
	return g.Task.RegisterMultiTask(name, rest...)
}

// RegisterInitTask is grunt.registerInitTask; see TaskRunner.RegisterInitTask.
func (g *Grunt) RegisterInitTask(name string, rest ...any) *TaskRunner {
	return g.Task.RegisterInitTask(name, rest...)
}

// RenameTask is grunt.renameTask; see TaskRunner.RenameTask.
func (g *Grunt) RenameTask(oldname, newname string) *TaskRunner {
	return g.Task.RenameTask(oldname, newname)
}

// LoadTasks is grunt.loadTasks; see TaskRunner.LoadTasks.
func (g *Grunt) LoadTasks(dir string) { g.Task.LoadTasks(dir) }

// LoadNpmTasks is grunt.loadNpmTasks; see TaskRunner.LoadNpmTasks.
func (g *Grunt) LoadNpmTasks(name string) { g.Task.LoadNpmTasks(name) }

// Warn is grunt.warn (grunt.fail.warn).
func (g *Grunt) Warn(e any, code ...int) { g.Fail.Warn(e, code...) }

// Fatal is grunt.fatal (grunt.fail.fatal).
func (g *Grunt) Fatal(e any, code ...int) { g.Fail.Fatal(e, code...) }

// O builds an object from alternating keys and values, keeping JavaScript's key
// order (array-index keys first, ascending, then the others in insertion order).
func O(kv ...any) *js.Object { return js.ObjectOf(kv...) }

// A builds an array.
func A(items ...any) *js.Array {
	for i, it := range items {
		items[i] = js.Normalize(it)
	}
	return js.NewArray(items...)
}

// Fn wraps a Go function as a JavaScript function, for the options that take one
// (such as the process option of grunt.file.copy). Returning an error throws it.
func Fn(f func(args ...any) any) *js.Function {
	return &js.Function{Call: func(_ any, args []any) (any, error) {
		v := f(args...)
		if err, ok := v.(error); ok {
			return nil, err
		}
		return js.Normalize(v), nil
	}}
}

// JSON is JSON.stringify(v, null, space): a string, or js.Undefined when v has no
// JSON representation (undefined, a function). A value with a cycle panics with
// V8's TypeError, as JSON.stringify throws it.
func JSON(v any, space ...any) any {
	var sp any = js.Undefined
	if len(space) > 0 {
		sp = js.Normalize(space[0])
	}
	s, ok, err := jsrt.JSONStringify(js.Normalize(v), nil, sp)
	if err != nil {
		panic(err)
	}
	if !ok {
		return js.Undefined
	}
	return s
}

// Main runs the gruntfile as the grunt command line does: it parses os.Args as
// grunt-cli does, loads the gruntfile (calls the function), runs the requested tasks
// and exits with grunt's exit code.
func Main(gruntfile func(g *Grunt)) {
	stdout := newSyncWriter(os.Stdout)
	proc := &process{
		stdout:      stdout,
		stderr:      os.Stderr,
		getenv:      os.LookupEnv,
		now:         wallClock,
		stdoutIsTTY: isTerminal(os.Stdout),
		installPath: installPath(),
	}
	proc.exit = func(code int) {
		// process.exit: Node writes stdout synchronously to pipes and files, so all
		// output written so far reaches it before the process ends. The process
		// ends whether or not that write succeeds.
		_ = stdout.Flush()
		os.Exit(code)
	}
	g := newGrunt(proc)
	g.cli(os.Args, gruntfile)
	// grunt exits explicitly when its tasks are done; returning here means it
	// stopped early (--version, --help), and Node exits with 0 once nothing is left.
	proc.exit(0)
}
