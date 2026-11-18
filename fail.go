package grunt

import (
	"github.com/gruntjs/grunt-go/js"
)

// Exit codes (grunt.fail.code).
const (
	FatalError          = 1
	MissingGruntfile    = 2
	TaskFailure         = 3
	TemplateError       = 4
	InvalidAutocomplete = 5
	Warning             = 6
)

// Fail is grunt.fail: warnings, fatal errors and the final report.
type Fail struct {
	g                     *Grunt
	errorcount, warncount int
}

// ErrorCount is grunt.fail.errorcount, the number of grunt.log.error calls.
func (f *Fail) ErrorCount() int { return f.errorcount }

// WarnCount is grunt.fail.warncount, the number of warnings.
func (f *Fail) WarnCount() int { return f.warncount }

// message is e.message for the values a gruntfile throws or passes to warn and
// fatal; ok is false when e has no message property. Reading a property of null or
// undefined throws, as it does in JavaScript.
func message(e any) (msg any, ok bool) {
	switch x := e.(type) {
	case *js.Error:
		return x.Message, true
	case error:
		return x.Error(), true
	case *js.Object:
		if x.Has("message") {
			return x.Get("message"), true
		}
	case nil, js.UndefinedType:
		panic(js.NewTypeError("Cannot read properties of " + js.ToString(e) + " (reading 'message')"))
	}
	return js.Undefined, false
}

// writeln is fail.js's writeln: it unmutes the log and prints the message as a
// warning or a fatal error.
func (f *Fail) writeln(e any, warn bool) {
	g := f.g
	g.Log.SetMuted(false)
	// String(e.message || e)
	m, _ := message(e)
	if !js.ToBoolean(m) {
		m = e
	}
	msg := js.ToString(m)
	if !g.option.truthy("no-color") {
		msg += "\x07" // Beep!
	}
	if warn {
		msg = "Warning: " + msg + " "
		if g.option.truthy("force") {
			msg += g.Log.style("Used --force, continuing.", "underline")
		} else {
			msg += "Use --force to continue."
		}
		msg = g.Log.style(msg, "yellow")
	} else {
		msg = g.Log.style("Fatal error: "+msg, "red")
	}
	g.Log.Writeln(msg)
}

// dumpStack prints the error's stack (or its origError's) with --stack. Errors made
// in Go carry no stack frames, so the stack is the error's first line,
// "Name: message".
func (f *Fail) dumpStack(e any) {
	if !f.g.option.truthy("stack") {
		return
	}
	err, ok := e.(error)
	if !ok {
		return
	}
	stack := errorStack(err)
	if je, ok := err.(*js.Error); ok && je.Orig != nil {
		stack = errorStack(je.Orig)
	}
	// console.log writes to stdout.
	consoleWrite(f.g.proc.stdout, stack+"\n")
}

// exitCode is `typeof errcode === 'number' ? errcode : fallback`.
func exitCode(code []int, fallback int) int {
	if len(code) > 0 {
		return code[0]
	}
	return fallback
}

// Fatal is grunt.fail.fatal: it prints "Fatal error: message" and exits with code
// (FatalError by default).
func (f *Fail) Fatal(e any, code ...int) {
	e = js.Normalize(e)
	f.writeln(e, false)
	f.dumpStack(e)
	f.g.proc.exit(exitCode(code, FatalError))
}

// Warn is grunt.fail.warn: it prints "Warning: message" and, unless --force is on,
// "Aborted due to warnings." and exits with code (Warning by default).
func (f *Fail) Warn(e any, code ...int) {
	e = js.Normalize(e)
	// typeof e === 'string' ? e : e.message
	msg := e
	if _, ok := e.(string); !ok {
		msg, _ = message(e)
	}
	f.warncount++
	f.writeln(msg, true)
	if !f.g.option.truthy("force") {
		f.dumpStack(e)
		f.g.Log.Writeln().Fail("Aborted due to warnings.")
		f.g.proc.exit(exitCode(code, Warning))
	}
}

// report is fail.report, printed when every task has run.
func (f *Fail) report() {
	if f.warncount > 0 {
		f.g.Log.Writeln().Fail("Done, but with warnings.")
	} else {
		f.g.Log.Writeln().Success("Done.")
	}
}
