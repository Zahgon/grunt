package grunt

import (
	"os"
	"slices"
	"strings"

	"github.com/gruntjs/grunt-go/internal/glob"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/internal/nodefs"
	"github.com/gruntjs/grunt-go/internal/nodepath"
	"github.com/gruntjs/grunt-go/js"
)

// This file is the task loading of lib/grunt/task.js: task.init, which loads the
// gruntfile, and loadTasks / loadNpmTasks.

// init is task.init: it loads the gruntfile (unless only init tasks were named),
// after making its directory (or --base) the working directory, then the task
// plugins named by --npm and the task directories named by --tasks.
func (r *TaskRunner) init(tasks []string, gruntfile func(g *Grunt)) {
	g := r.g
	allInit := len(tasks) > 0
	for _, name := range tasks {
		if t := r.taskPlusArgs(name).task; t == nil || !t.init {
			allInit = false
		}
	}
	if !allInit {
		path := js.ToString(g.Option("gruntfile"))
		if !js.ToBoolean(g.Option("gruntfile")) {
			path = nodepath.Join(cwd(), gruntfileName)
		}
		g.Verbose.Writeln().Write(`Reading "` + nodepath.Basename(path) + `" Gruntfile...`).Ok()
		// Make all paths relative to the Gruntfile's location (or --base).
		dir := nodepath.Dirname(path)
		if base := g.Option("base"); js.ToBoolean(base) {
			dir = js.ToString(base)
		}
		if err := os.Chdir(dir); err != nil {
			e := nodefs.NewError(err, "chdir", cwd())
			e.Dest = dir
			g.uncaught(e)
		}
		r.loadTasksMessage("Gruntfile")
		r.loadTask(path, func() { gruntfile(g) })
	}
	for _, name := range optionList(g.Option("npm")) {
		r.LoadNpmTasks(name)
	}
	for _, dir := range optionList(g.Option("tasks")) {
		r.LoadTasks(dir)
	}
}

// optionList is (grunt.option(name) || []).map(String) for an array option.
func optionList(v any) []string {
	a, ok := v.(*js.Array)
	if !ok {
		return nil
	}
	out := make([]string, len(a.Items))
	for i, it := range a.Items {
		out[i] = js.ToString(it)
	}
	return out
}

// loadTasksMessage logs which tasks are being registered.
func (r *TaskRunner) loadTasksMessage(info string) {
	r.g.Verbose.Subhead("Registering " + info + " tasks.")
}

// loadTask runs a task file (load) and logs the tasks it registered and renamed,
// or the error it threw.
func (r *TaskRunner) loadTask(path string, load func()) {
	g := r.g
	r.loadStack = append(r.loadStack, r.registry)
	r.registry = &registry{filepath: path}
	msg := `Loading "` + nodepath.Basename(path) + `" tasks...`
	if thrown, ok := catch(load); ok {
		var stack any = js.Undefined
		if err, isErr := thrown.(error); isErr {
			thrown = asJSError(err)
			stack = errorStack(err)
		}
		g.Log.Write(msg).Error().Verbose.Error(stack).Or.Error(thrown)
	} else {
		g.Verbose.Write(msg).Ok()
		regCount := 0
		for _, l := range []struct {
			prefix string
			names  []string
		}{{"- ", r.registry.untasks}, {"+ ", r.registry.tasks}} {
			names := uniqueSorted(l.names)
			if len(names) > 0 {
				regCount++
				g.Verbose.Writeln(l.prefix + g.Log.Wordlist(names))
			}
		}
		if regCount == 0 {
			g.Verbose.Warn("No tasks were registered or unregistered.")
		}
	}
	r.registry = r.loadStack[len(r.loadStack)-1]
	r.loadStack = r.loadStack[:len(r.loadStack)-1]
}

// uniqueSorted is _.chain(list).uniq().sort().value().
func uniqueSorted(list []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range list {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	slices.SortStableFunc(out, jsstr.Compare)
	return out
}

// LoadTasks is grunt.loadTasks: it loads the task files (*.js, *.cjs, *.coffee) of a
// directory. A Go gruntfile cannot run JavaScript task files, so each one found is
// reported as failing to load; a missing directory logs
// `Tasks directory "dir" not found.`
func (r *TaskRunner) LoadTasks(dir string) {
	r.loadTasksMessage(`"` + dir + `"`)
	if !r.g.File.Exists(dir) {
		r.g.Log.Error(`Tasks directory "` + dir + `" not found.`)
		return
	}
	r.loadTasksDir(dir)
}

// loadTasksDir loads the task files of an existing directory.
func (r *TaskRunner) loadTasksDir(dir string) {
	files, err := globTaskFiles(dir)
	if err != nil {
		r.g.Log.Verbose.Error(errorStack(err)).Or.Error(asJSError(err))
		return
	}
	for _, name := range files {
		path := nodepath.Join(dir, name)
		r.loadTask(path, func() {
			panic(js.NewError(`Cannot load JavaScript task file "` + path + `" from a Go gruntfile.`))
		})
	}
}

// globTaskFiles lists the task files of a directory (glob '*.{js,cjs,coffee}').
func globTaskFiles(dir string) ([]string, error) {
	return glob.Sync("*.{js,cjs,coffee}", glob.Options{Cwd: dir})
}

// LoadNpmTasks is grunt.loadNpmTasks: it loads the tasks directory of a plugin
// installed in node_modules. Go gruntfiles have no Node module resolution, so a
// plugin is looked up only in ./node_modules (and "gruntcollection" plugins are not
// expanded into their dependencies); a missing one logs
// `Local Npm module "name" not found. Is it installed?`
func (r *TaskRunner) LoadNpmTasks(name string) {
	g := r.g
	r.loadTasksMessage(`"` + name + `" local Npm module`)
	root := nodepath.Resolve(cwd(), "node_modules")
	if !g.File.Exists(nodepath.Join(root, name)) {
		// The original then asks require.resolve, which searches the parent
		// directories' node_modules as well.
		parts := strings.Split(name, "/")
		normalized := parts[0]
		if strings.HasPrefix(name, "@") && len(parts) > 1 {
			normalized = parts[0] + "/" + parts[1]
		}
		g.Log.Error(`Local Npm module "` + normalized + `" not found. Is it installed?`)
		return
	}
	tasksDir := nodepath.Join(root, name, "tasks")
	if g.File.Exists(tasksDir) {
		r.loadTasksDir(tasksDir)
	} else {
		g.Log.Error(`Local Npm module "` + name + `" not found. Is it installed?`)
	}
}
