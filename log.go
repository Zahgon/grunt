package grunt

import (
	"strings"

	"github.com/gruntjs/grunt-go/internal/log"
	"github.com/gruntjs/grunt-go/js"
)

// Log is a grunt-legacy-log logger: grunt.log, grunt.verbose and their relatives.
// Messages are formatted with Node's util.format; every writing method returns the
// logger so calls chain as in JavaScript (g.Log.Write("x").Ok()).
type Log struct {
	// Verbose and NotVerbose log only in (respectively out of) --verbose mode; they
	// are set on grunt.log only.
	Verbose, NotVerbose *Log
	// Or is set on Verbose and NotVerbose and points at the other one.
	Or *Log
	// Always is grunt.log, for every logger.
	Always *Log

	l *log.Log
}

// newLog builds grunt.log as lib/grunt.js does with new Log({grunt: grunt}): the
// logger asks grunt.option for its color, verbose and debug options and counts
// errors in grunt.fail.errorcount.
func newLog(g *Grunt) *Log {
	base := log.New(log.Options{
		Out:        g.proc.stdout,
		Option:     g.option.get,
		ErrorCount: &g.Fail.errorcount,
	})
	l := &Log{l: base}
	l.Always = l
	l.Verbose = &Log{l: base.Verbose, Always: l}
	l.NotVerbose = &Log{l: base.NotVerbose, Always: l}
	l.Verbose.Or = l.NotVerbose
	l.NotVerbose.Or = l.Verbose
	return l
}

// initColors is log.initColors: the colors and chalk packages emit codes only when
// stdout is a terminal and --no-color was not given.
func (l *Log) initColors(terminal bool) {
	l.l.Options().ColorCodes = terminal && !js.ToBoolean(l.l.Option("no-color"))
}

// style applies a colour getter of the colors package ('str'.yellow).
func (l *Log) style(s, name string) string {
	return log.Stylize(l.l.Options().ColorCodes, s, name)
}

// Write writes the formatted arguments.
func (l *Log) Write(args ...any) *Log { l.l.Write(args...); return l }

// Writeln writes the formatted arguments and a line break.
func (l *Log) Writeln(args ...any) *Log { l.l.Writeln(args...); return l }

// Writelns writes the message wrapped at 80 columns.
func (l *Log) Writelns(args ...any) *Log { l.l.Writelns(args...); return l }

// Warn writes ">> message", or "ERROR" without arguments.
func (l *Log) Warn(args ...any) *Log { l.l.Warn(args...); return l }

// Error counts an error in grunt.fail.errorcount and warns.
func (l *Log) Error(args ...any) *Log { l.l.Error(args...); return l }

// Ok writes ">> message", or "OK" without arguments.
func (l *Log) Ok(args ...any) *Log { l.l.Ok(args...); return l }

// Errorlns is Error with the message wrapped at 77 columns.
func (l *Log) Errorlns(args ...any) *Log { l.l.Errorlns(args...); return l }

// Oklns is Ok with the message wrapped at 77 columns.
func (l *Log) Oklns(args ...any) *Log { l.l.Oklns(args...); return l }

// Success writes the message in green.
func (l *Log) Success(args ...any) *Log { l.l.Success(args...); return l }

// Fail writes the message in red.
func (l *Log) Fail(args ...any) *Log { l.l.Fail(args...); return l }

// Header writes the message underlined, after a blank line once anything has been
// logged.
func (l *Log) Header(args ...any) *Log { l.l.Header(args...); return l }

// Subhead writes the message in bold, after a blank line once anything has been
// logged.
func (l *Log) Subhead(args ...any) *Log { l.l.Subhead(args...); return l }

// Debug writes "[D] message" when --debug is on.
func (l *Log) Debug(args ...any) *Log { l.l.Debug(args...); return l }

// Writeflags writes "prefix: a, b=value" for an object's keys or an array's items
// (prefix "Flags" by default). A value with a cycle panics with JSON.stringify's
// TypeError, which the original throws.
func (l *Log) Writeflags(obj any, prefix ...string) *Log {
	p := ""
	if len(prefix) > 0 {
		p = prefix[0]
	}
	if err := l.l.Writeflags(obj, p); err != nil {
		panic(err)
	}
	return l
}

// Writetableln writes one row of a table (see Table).
func (l *Log) Writetableln(widths []int, texts []string) *Log {
	l.l.Writetableln(widths, texts)
	return l
}

// Wordlist is grunt.log.wordlist(arr, options): the items coloured (cyan, or
// options.color; a falsy colour leaves them plain) and joined with ", " (or
// options.separator).
func (l *Log) Wordlist(arr []string, options ...any) string {
	separator, color := any(", "), any("cyan")
	if len(options) > 0 {
		if o, ok := js.Normalize(options[0]).(*js.Object); ok {
			if v := o.Get("separator"); v != js.Undefined {
				separator = v
			}
			if v := o.Get("color"); v != js.Undefined {
				color = v
			}
		}
	}
	items := make([]string, len(arr))
	for i, s := range arr {
		if js.ToBoolean(color) {
			s = l.style(s, js.ToString(color))
		}
		items[i] = s
	}
	return strings.Join(items, js.ToString(separator))
}

// Wraptext word-wraps text to width columns, keeping colour codes.
func (l *Log) Wraptext(width int, text string) string { return log.Wraptext(width, text) }

// Table lays texts out in columns of the given widths, wrapping each cell.
func (l *Log) Table(widths []int, texts []string) string { return log.Table(widths, texts) }

// Uncolor removes colour codes.
func (l *Log) Uncolor(s string) string { return log.Uncolor(s) }

// SetMuted sets grunt.log.muted, which silences every logger.
func (l *Log) SetMuted(muted bool) { l.l.SetMuted(muted) }

// Muted reports grunt.log.muted.
func (l *Log) Muted() bool { return l.l.Muted() }
