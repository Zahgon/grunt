package log

import (
	"strconv"
	"strings"
)

// style is an ANSI style as colors 1.1.2 (lib/styles.js) and ansi-styles define it.
type style struct {
	open, close string
}

func ansi(open, close int) style {
	return style{"\x1b[" + strconv.Itoa(open) + "m", "\x1b[" + strconv.Itoa(close) + "m"}
}

// styles holds the colors 1.1.2 style table, which is also what chalk 4 uses for the
// names grunt passes to it.
var styles = map[string]style{
	"reset":         ansi(0, 0),
	"bold":          ansi(1, 22),
	"dim":           ansi(2, 22),
	"italic":        ansi(3, 23),
	"underline":     ansi(4, 24),
	"inverse":       ansi(7, 27),
	"hidden":        ansi(8, 28),
	"strikethrough": ansi(9, 29),
	"black":         ansi(30, 39),
	"red":           ansi(31, 39),
	"green":         ansi(32, 39),
	"yellow":        ansi(33, 39),
	"blue":          ansi(34, 39),
	"magenta":       ansi(35, 39),
	"cyan":          ansi(36, 39),
	"white":         ansi(37, 39),
	"gray":          ansi(90, 39),
	"grey":          ansi(90, 39),
	"bgBlack":       ansi(40, 49),
	"bgRed":         ansi(41, 49),
	"bgGreen":       ansi(42, 49),
	"bgYellow":      ansi(43, 49),
	"bgBlue":        ansi(44, 49),
	"bgMagenta":     ansi(45, 49),
	"bgCyan":        ansi(46, 49),
	"bgWhite":       ansi(47, 49),
	"blackBG":       ansi(40, 49),
	"redBG":         ansi(41, 49),
	"greenBG":       ansi(42, 49),
	"yellowBG":      ansi(43, 49),
	"blueBG":        ansi(44, 49),
	"magentaBG":     ansi(45, 49),
	"cyanBG":        ansi(46, 49),
	"whiteBG":       ansi(47, 49),
}

// Stylize is the String.prototype colour getter colors 1.1.2 installs ('str'.red):
// with codes enabled it wraps s in the style's open and close codes, without nesting
// or replacing codes already in s; otherwise it returns s unchanged. An unknown style
// name returns s unchanged (the original getter would be undefined).
//
// Note that colors.mode = 'none', which grunt sets for --no-color, has no effect in
// colors 1.1.2: only colors.enabled (false when stdout is not a TTY or --no-color is
// in process.argv) decides, and that is what enabled reports here.
func Stylize(enabled bool, s, name string) string {
	st, ok := styles[name]
	if !enabled || !ok {
		return s
	}
	return st.open + s + st.close
}

// chalkStyle is chalk 4's builder applied to one already-stringified argument at a
// colour level above zero: the style is re-opened after each of its own closing
// codes inside s, and the
// style is closed before and re-opened after every line break. Level zero (stdout
// not a TTY, or --no-color) returns s unchanged, as does the empty string.
func chalkStyle(enabled bool, s, name string) string {
	st, ok := styles[name]
	if !enabled || !ok || s == "" {
		return s
	}
	if strings.Contains(s, "\x1b") {
		s = strings.ReplaceAll(s, st.close, st.close+st.open)
	}
	if lf := strings.IndexByte(s, '\n'); lf != -1 {
		s = encaseCRLF(s, st.close, st.open, lf)
	}
	return st.open + s + st.close
}

// encaseCRLF is chalk's stringEncaseCRLFWithFirstIndex: it puts prefix before and
// postfix after every "\n" (or "\r\n") from index on.
func encaseCRLF(s, prefix, postfix string, index int) string {
	var b strings.Builder
	end := 0
	for index != -1 {
		gotCR := index > 0 && s[index-1] == '\r'
		cut, nl := index, "\n"
		if gotCR {
			cut, nl = index-1, "\r\n"
		}
		b.WriteString(s[end:cut])
		b.WriteString(prefix)
		b.WriteString(nl)
		b.WriteString(postfix)
		end = index + 1
		index = strings.IndexByte(s[end:], '\n')
		if index != -1 {
			index += end
		}
	}
	b.WriteString(s[end:])
	return b.String()
}

// StripColors is colors.stripColors: it removes every ESC [ digits m sequence.
// grunt-legacy-log-utils' uncolor uses the same pattern.
func StripColors(s string) string {
	if !strings.Contains(s, "\x1b[") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if n := colorCodeLen(s, i); n > 0 {
			i += n
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// colorCodeLen returns the length of the /\x1B\[\d+m/ match starting at s[i], or 0.
func colorCodeLen(s string, i int) int {
	if i+1 >= len(s) || s[i] != 0x1b || s[i+1] != '[' {
		return 0
	}
	j := i + 2
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	if j == i+2 || j >= len(s) || s[j] != 'm' {
		return 0
	}
	return j + 1 - i
}
