// Package log ports grunt-legacy-log 3.0.1 and grunt-legacy-log-utils 2.1.3, the
// logger behind grunt.log and grunt.verbose, together with the pieces of Node's util
// module (format and inspect) and of the colors and chalk packages that its output
// depends on.
//
// A Log writes to its output stream; its Verbose and NotVerbose loggers share the
// base logger's options, muted flag and hasLogged flag and only write when the
// verbose option is on (respectively off). Every writing method returns its receiver
// so calls chain as in the original (log.Write(msg).Error().Verbose.Error(stack)).
//
// Write errors: the original writes to process.stdout and never sees a failure. Here
// the first error returned by the output stream is kept, later writes are skipped,
// and Err reports it.
package log

import (
	"io"
	"os"
	"strings"

	"github.com/gruntjs/grunt-go/js"
)

// Options configures a Log. The zero value logs to os.Stdout with colours off, not
// verbose, no debug output.
type Options struct {
	// Color is the logger's color option. When it is false, option("no-color") is
	// true and colour codes are stripped from messages before they are written.
	Color bool
	// Verbose and Debug are the verbose and debug options.
	Verbose, Debug bool
	// Out is the output stream; nil means os.Stdout.
	Out io.Writer
	// MaxCols is where Writelns, Errorlns and Oklns wrap; 0 means the legacy defaults
	// (80 for Writelns, 77 for the others).
	MaxCols int
	// Muted suppresses all output.
	Muted bool
	// ColorCodes reports whether the colour helpers emit ANSI codes. In the original
	// that is decided by the colors and chalk packages themselves, not by the logger:
	// both turn codes off when stdout is not a TTY or --no-color is in process.argv.
	ColorCodes bool
	// Option, when set, answers option queries instead of Color, Verbose and Debug.
	// grunt passes itself to the logger, and the logger then asks grunt.option.
	Option func(name string) any
	// ErrorCount, when set, is incremented by every Error call
	// (grunt.fail.errorcount).
	ErrorCount *int
}

type mode int

const (
	modeAlways mode = iota
	modeVerbose
	modeNotVerbose
)

// shared is the state the base logger and its verbose/notverbose loggers share.
type shared struct {
	opts      Options
	hasLogged bool
	err       error
}

// Log is a grunt-legacy-log logger.
type Log struct {
	s    *shared
	mode mode

	// Always is the base logger, for every logger.
	Always *Log
	// Verbose and NotVerbose are set on the base logger only: they log only when
	// the verbose option is on, respectively off.
	Verbose, NotVerbose *Log
	// Or is set on Verbose and NotVerbose only and points at the other one.
	Or *Log
}

// New returns a base logger with its Verbose and NotVerbose loggers.
func New(opts Options) *Log {
	l := &Log{s: &shared{opts: opts}, mode: modeAlways}
	l.Always = l
	l.Verbose = &Log{s: l.s, mode: modeVerbose, Always: l}
	l.NotVerbose = &Log{s: l.s, mode: modeNotVerbose, Always: l}
	l.Verbose.Or = l.NotVerbose
	l.NotVerbose.Or = l.Verbose
	return l
}

// Options returns the options shared by the base logger and its verbose loggers;
// changes through the pointer take effect immediately, as assignments to
// log.options do in the original.
func (l *Log) Options() *Options { return &l.s.opts }

// Muted reports whether output is suppressed.
func (l *Log) Muted() bool { return l.s.opts.Muted }

// SetMuted suppresses or restores output for the base logger and its verbose loggers.
func (l *Log) SetMuted(muted bool) { l.s.opts.Muted = muted }

// HasLogged reports whether anything has been written (an unmuted write, even an
// empty one, by any of the related loggers).
func (l *Log) HasLogged() bool { return l.s.hasLogged }

// Err returns the first error the output stream returned, or nil.
func (l *Log) Err() error { return l.s.err }

// Option is Log.prototype.option: the Option hook's answer when there is one,
// otherwise the matching field, with "no-" names negating the field. Unknown names
// give js.Undefined (or true for an unknown "no-" name).
func (l *Log) Option(name string) any {
	o := &l.s.opts
	if o.Option != nil {
		return o.Option(name)
	}
	if rest, ok := strings.CutPrefix(name, "no-"); ok && rest != "" {
		return !js.ToBoolean(o.field(rest))
	}
	return o.field(name)
}

func (o *Options) field(name string) any {
	switch name {
	case "color":
		return o.Color
	case "verbose":
		return o.Verbose
	case "debug":
		return o.Debug
	case "muted":
		return o.Muted
	case "maxCols":
		if o.MaxCols == 0 {
			return nil
		}
		return float64(o.MaxCols)
	}
	return js.Undefined
}

func (l *Log) style(s, name string) string { return Stylize(l.s.opts.ColorCodes, s, name) }

// write is Log.prototype._write (with VerboseLog's verbose-mode check first).
func (l *Log) write(msg string) {
	if l.mode != modeAlways && js.ToBoolean(l.Option("verbose")) != (l.mode == modeVerbose) {
		return
	}
	if l.s.opts.Muted {
		return
	}
	l.s.hasLogged = true
	if js.ToBoolean(l.Option("no-color")) {
		msg = StripColors(msg)
	}
	msg = markup(msg, l.s.opts.ColorCodes)
	if msg == "" || l.s.err != nil {
		return
	}
	out := l.s.opts.Out
	if out == nil {
		out = os.Stdout
	}
	if _, err := io.WriteString(out, msg); err != nil {
		l.s.err = err
	}
}

func (l *Log) writeln(msg string) { l.write(msg + "\n") }

// format is Log.prototype._format: util.format with the first argument converted
// to a string, so it is always the format string.
func format(args []any) string {
	if len(args) > 0 {
		args = append([]any{js.ToString(js.Normalize(args[0]))}, args[1:]...)
	}
	return Format(args...)
}

// Write writes the formatted arguments.
func (l *Log) Write(args ...any) *Log {
	l.write(format(args))
	return l
}

// Writeln writes the formatted arguments and a line break.
func (l *Log) Writeln(args ...any) *Log {
	l.writeln(format(args))
	return l
}

// prefixed writes ">> " before every line of the trimmed message in the given
// colour, or fallback when there are no arguments.
func (l *Log) prefixed(args []any, color, fallback string) *Log {
	if len(args) == 0 {
		l.writeln(l.style(fallback, color))
		return l
	}
	msg := strings.TrimFunc(format(args), js.IsSpace)
	l.writeln(l.style(">> ", color) + strings.ReplaceAll(msg, "\n", l.style("\n>> ", color)))
	return l
}

// Warn writes ">> message" in red, or "ERROR" when called without arguments.
func (l *Log) Warn(args ...any) *Log { return l.prefixed(args, "red", "ERROR") }

// Error increments the error counter, when there is one, and warns. The counter is
// incremented even when this logger does not write (muted, or the wrong verbose mode).
func (l *Log) Error(args ...any) *Log {
	if c := l.s.opts.ErrorCount; c != nil {
		*c++
	}
	return l.Warn(args...)
}

// Ok writes ">> message" in green, or "OK" when called without arguments.
func (l *Log) Ok(args ...any) *Log { return l.prefixed(args, "green", "OK") }

func (l *Log) maxCols(legacy int) int {
	if l.s.opts.MaxCols != 0 {
		return l.s.opts.MaxCols
	}
	return legacy
}

// Errorlns wraps the message at MaxCols (77 by default) and logs it with Error.
func (l *Log) Errorlns(args ...any) *Log {
	return l.Error(Wraptext(l.maxCols(77), format(args)))
}

// Oklns wraps the message at MaxCols (77 by default) and logs it with Ok.
func (l *Log) Oklns(args ...any) *Log {
	return l.Ok(Wraptext(l.maxCols(77), format(args)))
}

// Success writes the message in green.
func (l *Log) Success(args ...any) *Log {
	l.writeln(l.style(format(args), "green"))
	return l
}

// Fail writes the message in red.
func (l *Log) Fail(args ...any) *Log {
	l.writeln(l.style(format(args), "red"))
	return l
}

// Header writes the message underlined, after a blank line unless nothing has been
// logged yet.
func (l *Log) Header(args ...any) *Log {
	msg := format(args)
	if l.s.hasLogged {
		l.writeln("")
	}
	l.writeln(l.style(msg, "underline"))
	return l
}

// Subhead writes the message in bold, after a blank line unless nothing has been
// logged yet.
func (l *Log) Subhead(args ...any) *Log {
	msg := format(args)
	if l.s.hasLogged {
		l.writeln("")
	}
	l.writeln(l.style(msg, "bold"))
	return l
}

// Debug writes "[D] message" when the debug option is on.
func (l *Log) Debug(args ...any) *Log {
	msg := format(args)
	if js.ToBoolean(l.Option("debug")) {
		l.writeln("[D] " + l.style(msg, "magenta"))
	}
	return l
}

// Writetableln writes one table row (see Table) and a line break.
func (l *Log) Writetableln(widths []int, texts []string) *Log {
	l.writeln(Table(widths, texts))
	return l
}

// Writelns wraps the message at MaxCols (80 by default) and writes it.
func (l *Log) Writelns(args ...any) *Log {
	l.writeln(Wraptext(l.maxCols(80), format(args)))
	return l
}

// Writeflags writes "prefix: flags" (prefix "Flags" when empty). An array lists its
// items; an object lists its keys, each followed by "=" and the JSON of its value
// unless the value is true; anything else, or an empty list, shows "(none)".
//
// The only error is JSON.stringify's TypeError for a value with a cycle, in which
// case nothing is written.
func (l *Log) Writeflags(obj any, prefix string) error {
	var list string
	obj = js.Normalize(obj)
	if a, ok := obj.(*js.Array); ok {
		list = l.Wordlist(a.Items)
	} else if keys, values, ok := objectEntries(obj); ok {
		words := make([]any, len(keys))
		for i, k := range keys {
			if values[i] == true {
				words[i] = k
				continue
			}
			s, defined, err := stringifyJSON(values[i])
			if err != nil {
				return err
			}
			if !defined {
				s = "undefined"
			}
			words[i] = k + "=" + s
		}
		list = l.Wordlist(words)
	}
	if prefix == "" {
		prefix = "Flags"
	}
	if list == "" {
		list = l.style("(none)", "cyan")
	}
	l.writeln(prefix + ": " + list)
	return nil
}

// objectEntries is Object.keys(obj).map(k => obj[k]) for the values typeof reports
// as "object"; ok is false for primitives, null and functions.
func objectEntries(obj any) (keys []string, values []any, ok bool) {
	switch x := obj.(type) {
	case *js.Object:
		for _, k := range x.Keys() {
			keys = append(keys, k)
			values = append(values, x.Get(k))
		}
	case *js.Error:
		if x.Props != nil {
			return objectEntries(x.Props)
		}
	case []byte:
		for i, c := range x {
			keys = append(keys, js.NumberToString(float64(i)))
			values = append(values, float64(c))
		}
	case js.Date, *js.RegExp:
	default:
		return nil, nil, false
	}
	return keys, values, true
}

// Wordlist is logUtils.wordlist with its default options: the items as strings,
// coloured cyan by chalk, joined with ", ".
func (l *Log) Wordlist(items []any) string { return wordlist(items, l.s.opts.ColorCodes) }

// Uncolor is logUtils.uncolor (see the package function).
func (l *Log) Uncolor(s string) string { return Uncolor(s) }

// Wraptext is logUtils.wraptext (see the package function).
func (l *Log) Wraptext(width int, text string) string { return Wraptext(width, text) }

// Table is logUtils.table (see the package function).
func (l *Log) Table(widths []int, texts []string) string { return Table(widths, texts) }
