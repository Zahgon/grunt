package grunt

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gruntjs/grunt-go/js"
)

// The scenario tests run Go gruntfiles through the whole command line and compare
// stdout and the exit code with what grunt 1.6.3 printed for the JavaScript twins in
// testdata/scenarios/<name>/Gruntfile.js (recorded by testdata/scenarios/record.py).

type scenarioRun struct {
	Argv []string `json:"argv"`
	Code int      `json:"code"`
	Out  string   `json:"out"`
}

func loadScenario(t *testing.T, name string) []scenarioRun {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "scenarios", "expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	var all map[string][]scenarioRun
	if err := json.Unmarshal(data, &all); err != nil {
		t.Fatal(err)
	}
	return all[name]
}

// copyFixtures copies a scenario's input files (everything but its Gruntfile.js and
// runs.json) into dir.
func copyFixtures(t *testing.T, name, dir string) {
	t.Helper()
	src := filepath.Join("testdata", "scenarios", name)
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if rel == "." || rel == "Gruntfile.js" || rel == "runs.json" {
			return nil
		}
		target := filepath.Join(dir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// runGrunt runs gruntfile with the command-line arguments argv in dir, as Main
// does, and returns its standard output and exit code.
func runGrunt(t *testing.T, dir string, argv []string, gruntfile func(g *Grunt)) (string, int) {
	t.Helper()
	t.Chdir(dir)
	var stdout, stderr bytes.Buffer
	proc := &process{
		stdout:      newSyncWriter(&stdout),
		stderr:      &stderr,
		getenv:      func(string) (string, bool) { return "", false },
		now:         wallClock,
		installPath: "/install",
	}
	proc.exit = func(code int) { panic(exitSignal{code}) }
	code := func() (code int) {
		defer func() {
			if v := recover(); v != nil {
				sig, ok := v.(exitSignal)
				if !ok {
					panic(v)
				}
				code = sig.code
			}
		}()
		newGrunt(proc).cli(append([]string{"grunt"}, argv...), gruntfile)
		return 0
	}()
	if err := proc.stdout.Flush(); err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(stdout.String(), dir, "<dir>"), code
}

var gruntfileExt = regexp.MustCompile(`Gruntfile\.(js|go)\b`)

// checkScenario runs every recorded command line of a scenario in a fresh copy of
// its fixtures and compares the results.
func checkScenario(t *testing.T, name string, gruntfile func(g *Grunt)) {
	runs := loadScenario(t, name)
	if len(runs) == 0 {
		t.Fatalf("no recorded runs for %s", name)
	}
	for _, run := range runs {
		t.Run(strings.Join(run.Argv, " "), func(t *testing.T) {
			dir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			copyFixtures(t, name, dir)
			out, code := runGrunt(t, dir, run.Argv, gruntfile)
			want := gruntfileExt.ReplaceAllString(run.Out, "Gruntfile.*")
			got := gruntfileExt.ReplaceAllString(out, "Gruntfile.*")
			if got != want {
				t.Errorf("stdout differs\n--- got\n%s\n--- want\n%s", got, want)
			}
			if code != run.Code {
				t.Errorf("exit code %d, want %d", code, run.Code)
			}
		})
	}
}

func TestTaskQueue(t *testing.T) {
	checkScenario(t, "queue", func(g *Grunt) {
		g.RegisterTask("a", "Task a.", TaskFunc(func(t *Task) any {
			g.Log.Writeln("a " + js.ToString(JSON(t.Args)) + " " + js.ToString(JSON(t.Flags)))
			g.Task.Run("b:1", "c")
			return nil
		}))
		g.RegisterTask("b", TaskFunc(func(t *Task) any {
			g.Log.Writeln("b " + t.NameArgs)
			g.Task.Mark()
			g.Task.Run("c:x")
			g.Task.ClearQueue(true)
			return nil
		}))
		g.RegisterTask("c", TaskFunc(func(t *Task) any {
			g.Log.Writeln("c " + t.NameArgs)
			return nil
		}))
		g.RegisterTask("x:y", TaskFunc(func(t *Task) any {
			g.Log.Writeln("x:y " + js.ToString(JSON(t.Args)))
			return nil
		}))
		g.RegisterTask("fails", TaskFunc(func(*Task) any { return false }))
		g.RegisterTask("throws", TaskFunc(func(*Task) any { panic(js.NewError("boom")) }))
		g.RegisterTask("async", TaskFunc(func(t *Task) any {
			done := t.Async()
			go func() {
				time.Sleep(5 * time.Millisecond)
				g.Log.Writeln("later")
				done(false)
			}()
			return nil
		}))
		g.RegisterTask("needs", TaskFunc(func(t *Task) any {
			if err := t.Requires("c"); err != nil {
				g.Log.Writeln(err.Error())
			} else {
				g.Log.Writeln("c ok")
			}
			return t.Requires("fails")
		}))
		g.RegisterTask("stop", TaskFunc(func(*Task) any {
			g.Task.ClearQueue()
			return nil
		}))
		g.RegisterTask("default", []string{"a", "x:y:z"})
		g.RegisterTask("str", "c")
		g.RenameTask("missing", "other")
	})
}

func TestConfig(t *testing.T) {
	checkScenario(t, "config", func(g *Grunt) {
		g.InitConfig(O(
			"pkg", O("name", "demo", "list", A(1, 2)),
			"paths", O("src", "src/<%= pkg.name %>", "dist", "<%= paths.src %>/dist", "all", "<%= pkg.list %>",
				"upper", "<%= pkg.name.toUpperCase() %>"),
			"a.b", O("c", "dotted"),
			"cyc", "<%= cyc %>",
			"bad", "<%= nope %>",
		))
		show := func(v any) { g.Log.Writeln(JSON(v)) }
		g.RegisterTask("cfg", TaskFunc(func(*Task) any {
			c := g.Config
			show(c.Get("paths.dist"))
			show(c.Get("paths.all"))
			show(c.Get("paths.upper"))
			show(c.Get(A("a.b", "c")))
			show(c.Get(`a\.b.c`))
			show(c.GetRaw("paths.src"))
			show(c.Process(O("k", A("<%= pkg.name %>-x", 3))))
			g.Log.Writeln(c.Escape("a.b.c") + " " + c.GetPropString(A("a.b", "c")))
			c.Set("new.deep", "<%= pkg.name %>!")
			show(c.Get("new"))
			if err := throws(func() { c.Get("cyc") }); err != nil {
				g.Log.Writeln("caught: " + err.Error())
			}
			if err := c.Requires("pkg.name", A("missing", "x")); err != nil {
				g.Log.Writeln("caught: " + err.Error())
			}
			show(c.Merge(O("pkg", O("list", A("m")))).(*js.Object).Get("pkg"))
			return nil
		}))
		g.RegisterTask("bad", TaskFunc(func(*Task) any {
			g.Log.Writeln("got " + js.ToString(JSON(g.Config.Get("bad"))))
			return nil
		}))
	})
}

func TestMultiTaskFiles(t *testing.T) {
	checkScenario(t, "files", func(g *Grunt) {
		g.InitConfig(O(
			"name", "bundle",
			"files", O(
				"options", O("sep", ";"),
				"compact", O("src", A("src/*.js", "!src/b.js"), "dest", "out/<%= name %>.js", "extra", 1),
				"object", O("files", O("out/<%= name %>.min.js", A("src/**/*.js"), "out/css", "src/*.css")),
				"array", O("files", A(
					O("src", "src/sub/*", "dest", "out/sub/"),
					O("out/x", A(A("src/a.js"), A(A("src/missing.js")))),
					O("dest", "out/nosrc"),
				)),
				"expand", O("expand", true, "cwd", "src/", "src", A("**/*.js"), "dest", "out/exp/", "ext", ".min.js",
					"options", O("flag", true)),
				"flat", O("expand", true, "cwd", "src", "src", "**/*", "dest", "out/flat", "flatten", true,
					"filter", "isFile", "extDot", "last", "ext", ".x"),
				"list", A("src/c.css", "src/nope"),
				"_hidden", O("src", "src/a.js"),
			),
		))
		g.RegisterMultiTask("files", "Lists files.", TaskFunc(func(t *Task) any {
			g.Log.Writeln("target " + t.Target + " " + js.ToString(JSON(t.Options(O("sep", ",", "z", 0)))))
			for _, f := range t.Files {
				m := O()
				if f.HasSrc() {
					m.Set("src", js.Normalize(f.Src()))
				}
				m.Set("dest", f.Dest)
				m.Set("orig", f.Orig)
				g.Log.Writeln(JSON(m))
			}
			g.Log.Writeln("filesSrc " + js.ToString(JSON(t.FilesSrc())))
			return nil
		}))
	})
}

func TestHelp(t *testing.T) {
	checkScenario(t, "help", func(g *Grunt) {
		noop := TaskFunc(func(*Task) any { return nil })
		g.RegisterTask("build", "Builds the project with a long description that has to wrap onto another line of the table.", noop)
		g.RegisterMultiTask("lint", noop)
		g.RegisterMultiTask("empty-info", "", noop)
		g.RegisterTask("default", []string{"build", "lint"})
		g.RegisterTask("10", "Numeric.", noop)
	})
}

func TestFailures(t *testing.T) {
	checkScenario(t, "fail", func(g *Grunt) {
		g.RegisterTask("warn", TaskFunc(func(*Task) any {
			g.Log.Writeln("before")
			g.Warn("careful", 9)
			g.Log.Writeln("after")
			return nil
		}))
		g.RegisterTask("fatal", TaskFunc(func(*Task) any {
			g.Fail.Fatal(js.NewError("dead"), 7)
			return nil
		}))
		g.RegisterTask("errors", TaskFunc(func(t *Task) any {
			g.Log.Error("one")
			g.Log.Error()
			g.Log.Writeln("count " + js.ToString(float64(t.ErrorCount())))
			return nil
		}))
	})
}
