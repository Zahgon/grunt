package grunt

import (
	"embed"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/internal/nodepath"
	"github.com/gruntjs/grunt-go/internal/nopt"
	"github.com/gruntjs/grunt-go/js"
)

// cliVersion is the version of grunt-cli whose behaviour the command line follows.
const cliVersion = "1.5.0"

// gruntfileName is the name the gruntfile is reported under: the Go program stands
// for the Gruntfile in the base directory.
const gruntfileName = "Gruntfile.go"

// completions are grunt-cli's shell completion scripts (grunt --completion=bash).
//
//go:embed completion/bash completion/zsh
var completions embed.FS

// knownOption is an entry of grunt-known-options 2.0.0.
type knownOption struct {
	long, short, info string
	typ               nopt.Type
	negate            bool
}

// knownOptions are the command-line options grunt and grunt-cli know, in order.
var knownOptions = []knownOption{
	{long: "help", short: "h", info: "Display this help text.", typ: nopt.Boolean},
	{long: "base", short: "b", info: "Specify an alternate base path. By default, all file paths are relative to the Gruntfile. " +
		"(grunt.file.setBase) *", typ: nopt.Path},
	{long: "color", info: "Disable colored output.", typ: nopt.Boolean, negate: true},
	{long: "gruntfile", info: "Specify an alternate Gruntfile. By default, grunt looks in the current or parent directories " +
		"for the nearest Gruntfile.js or Gruntfile.coffee file.", typ: nopt.Path},
	{long: "debug", short: "d", info: "Enable debugging mode for tasks that support it.", typ: nopt.OneOf(nopt.Number, nopt.Boolean)},
	{long: "stack", info: "Print a stack trace when exiting with a warning or fatal error.", typ: nopt.Boolean},
	{long: "force", short: "f", info: "A way to force your way past warnings. Want a suggestion? Don't use this option, fix your code.",
		typ: nopt.Boolean},
	{long: "tasks", info: `Additional directory paths to scan for task and "extra" files. (grunt.loadTasks) *`, typ: nopt.Array},
	{long: "npm", info: `Npm-installed grunt plugins to scan for task and "extra" files. (grunt.loadNpmTasks) *`, typ: nopt.Array},
	{long: "write", info: "Disable writing files (dry run).", typ: nopt.Boolean, negate: true},
	{long: "verbose", short: "v", info: "Verbose mode. A lot more information output.", typ: nopt.Boolean},
	{long: "version", short: "V", info: "Print the grunt version. Combine with --verbose for more info.", typ: nopt.Boolean},
	{long: "completion", info: "Output shell auto-completion rules. See the grunt-cli documentation for more information.",
		typ: nopt.String},
	{long: "preload", short: "p", info: "Specify a language interpreter to require first if you are writing your Gruntfile in " +
		"a language Grunt doesn't support by default.", typ: nopt.String},
}

// parseArgv parses the command line as grunt-cli does: nopt over the known options,
// with each short option an alias of its long form.
func parseArgv(argv []string) (*nopt.Result, error) {
	types := map[string]nopt.Type{}
	aliases := map[string][]string{}
	for _, o := range knownOptions {
		types[o.long] = o.typ
		if o.short != "" {
			aliases[o.short] = []string{"--" + o.long}
		}
	}
	return nopt.Parse(types, aliases, argv, 1)
}

// installPath is where `--version --verbose` says grunt is installed: the directory
// of the gruntfile program, the Go counterpart of the grunt package directory.
func installPath() string {
	exe, err := os.Executable()
	if err != nil {
		// The platform cannot name the running executable: use the program path the
		// command line was started with, made absolute against the working directory
		// (left as given if that is gone too).
		exe = os.Args[0]
		if abs, err := filepath.Abs(exe); err == nil {
			exe = abs
		}
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return filepath.Dir(exe)
}

// isTerminal reports whether f is a character device, as Node's isTTY does.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// cli is grunt-cli's bin/grunt followed by grunt.tasks: it parses argv, prints the
// completion script or the grunt-cli version when asked, and runs grunt with the
// parsed options, the gruntfile being <base or working directory>/Gruntfile.go.
func (g *Grunt) cli(argv []string, gruntfile func(g *Grunt)) {
	parsed, err := parseArgv(argv)
	if err != nil {
		g.uncaught(err)
	}
	options := parsed.Options
	if options.Has("completion") {
		g.printCompletion(options.Get("completion"))
	} else if js.ToBoolean(options.Get("version")) {
		consoleWrite(g.proc.stdout, "grunt-cli v"+cliVersion+"\n")
	}
	dir := cwd()
	if base := options.Get("base"); js.ToBoolean(base) {
		dir = nodepath.Resolve(dir, js.ToString(base))
	}
	configPath := nodepath.Join(dir, gruntfileName)
	if gf := options.Get("gruntfile"); js.ToBoolean(gf) {
		configPath = nodepath.Resolve(cwd(), js.ToString(gf))
	}
	options.Set("gruntfile", configPath)
	g.tasks(parsed.Remain, options, gruntfile)
}

// printCompletion is grunt-cli's completion.print: the named shell's completion
// script, or an echo command saying it does not exist (exit code 5).
func (g *Grunt) printCompletion(name any) {
	n := js.ToString(name)
	out, err := completions.ReadFile("completion/" + n)
	code := 0
	if err != nil || n == "" || strings.Contains(n, "/") {
		code = InvalidAutocomplete
		msg := `echo "Specified grunt shell auto-completion rules `
		if js.ToBoolean(name) && n != "true" {
			msg += "for '" + n + "' "
		}
		out = []byte(msg + `not found."`)
	}
	consoleWrite(g.proc.stdout, string(out)+"\n")
	g.proc.exit(code)
}

// uncaught ends the program for an exception thrown outside the task runner, as
// Node does for an uncaught exception: the error on stderr and exit code 1. (Node
// also prints the throwing source line and the stack frames.)
func (g *Grunt) uncaught(v any) {
	if _, ok := v.(exitSignal); ok {
		panic(v)
	}
	if err, ok := v.(error); ok {
		v = asJSError(err)
	}
	consoleWrite(g.proc.stderr, js.ToString(v)+"\n")
	g.proc.exit(1)
}

// tasks is grunt.tasks(tasks, options): it runs the named tasks ("default" when
// none are given) after loading the gruntfile, then reports and exits.
func (g *Grunt) tasks(names []string, options *js.Object, gruntfile func(g *Grunt)) {
	g.OptionInit(options)

	// Display the grunt version and quit if the user did --version.
	if g.option.truthy("version") {
		g.Log.Writeln("grunt v" + g.Version)
		if g.option.truthy("verbose") {
			g.Verbose.Writeln("Install path: " + g.proc.installPath)
			// Initialize the task system silently so that the tasks can be listed.
			g.Log.SetMuted(true)
			g.Task.init(nil, gruntfile)
			g.Log.SetMuted(false)
			taskNames := g.Task.tasks.Keys()
			slices.SortStableFunc(taskNames, jsstr.Compare)
			g.Verbose.Writeln("Available tasks: " + strings.Join(taskNames, " "))
			var opts []string
			for _, o := range knownOptions {
				opts = append(opts, "--"+negatePrefix(o)+o.long)
				if o.short != "" {
					opts = append(opts, "-"+o.short)
				}
			}
			g.Verbose.Writeln("Available options: " + strings.Join(opts, " "))
		}
		return
	}

	g.Log.initColors(g.proc.stdoutIsTTY)

	// Display help and quit if the user did --help.
	if g.option.truthy("help") {
		g.help(gruntfile)
		return
	}

	g.Verbose.Header("Initializing").Writeflags(g.OptionFlags(), "Command-line options")

	tasksSpecified := len(names) > 0
	if !tasksSpecified {
		names = []string{"default"}
	}
	g.Task.init(names, gruntfile)

	g.Verbose.Writeln()
	if !tasksSpecified {
		g.Verbose.Writeln("No tasks specified, running default tasks.")
	}
	g.Verbose.Writeflags(names, "Running tasks")

	g.Task.onError = func(err error) { g.Fail.Warn(err, TaskFailure) }
	g.Task.onDone = func() {
		g.Fail.report()
		g.proc.exit(0)
	}
	// From here on, an exception that escapes is reported as a fatal error
	// (process.on('uncaughtException')).
	if thrown, ok := catch(func() {
		// Queue each task on its own, so that each missing one is reported.
		for _, name := range names {
			g.Task.Run(name)
		}
		g.Task.start()
	}); ok {
		g.Fail.Fatal(thrown, TaskFailure)
	}
}

func negatePrefix(o knownOption) string {
	if o.negate {
		return "no-"
	}
	return ""
}
