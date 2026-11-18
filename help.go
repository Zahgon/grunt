package grunt

import (
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/internal/lodash"
	"github.com/gruntjs/grunt-go/js"
)

// commandName is the name the usage line gives the command (path.basename of the
// script grunt runs as, "grunt").
const commandName = "grunt"

// help is lib/grunt/help.js's display: the --help screen.
func (g *Grunt) help(gruntfile func(g *Grunt)) {
	col1len := 0
	initCol1 := func(s string) { col1len = max(col1len, jsstr.Length(s)) }

	// Options, as a two-column table.
	var options [][2]string
	for _, o := range knownOptions {
		col1 := "--" + negatePrefix(o) + o.long
		if o.short != "" {
			col1 += ", -" + o.short
		}
		initCol1(col1)
		options = append(options, [2]string{col1, o.info})
	}

	// Initialize the task system so that the tasks can be listed.
	g.Task.init(nil, gruntfile)
	var tasks [][2]string
	for _, name := range g.Task.tasks.Keys() {
		initCol1(name)
		t := g.Task.lookup(name)
		info := t.info
		if t.multi {
			info += " *"
		}
		tasks = append(tasks, [2]string{t.name, info})
	}

	commandWidth := max(col1len+20, 76)
	widths := []int{1, col1len, 2, commandWidth - col1len}
	table := func(rows [][2]string) {
		for _, row := range rows {
			// grunt.util._.pad is underscore.string's pad: spaces on the left.
			padded, err := lodash.Pad(row[0], float64(col1len), js.Undefined, js.Undefined)
			if err != nil {
				panic(err)
			}
			g.Log.Writetableln(widths, []string{"", padded, "", row[1]})
		}
	}

	log := g.Log
	log.Writeln("Grunt: The JavaScript Task Runner (v" + g.Version + ")")

	log.Header("Usage")
	log.Writeln(" " + commandName + " [options] [task [task ...]]")

	log.Header("Options")
	table(options)
	log.Writeln().Writelns("Options marked with * have methods exposed via the grunt API and should " +
		"instead be specified inside the Gruntfile wherever possible.")

	log.Header("Available tasks")
	if len(tasks) == 0 {
		log.Writeln("(no tasks found)")
	} else {
		table(tasks)
		log.Writeln().Writelns("Tasks run in the order specified. Arguments may be passed to tasks that " +
			`accept them by using colons, like "lint:files". Tasks marked with * are ` +
			`"multi tasks" and will iterate over all sub-targets if no argument is ` +
			"specified.")
	}
	log.Writeln().Writelns("The list of available tasks may change based on tasks directories or " +
		"grunt plugins specified in the Gruntfile or via command-line options.")

	log.Writeln().Writeln("For more information, see http://gruntjs.com/")
}
