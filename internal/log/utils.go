package log

import (
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// This file ports grunt-legacy-log-utils 2.1.3.

// Uncolor is logUtils.uncolor: it removes every /\x1B\[\d+m/ colour code.
func Uncolor(s string) string {
	return StripColors(s)
}

// wordlist is logUtils.wordlist with its default options: each item is stringified
// by string concatenation (js.ToString), passed through chalk.cyan, and the results
// are joined with ", ".
func wordlist(items []any, colorCodes bool) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = chalkStyle(colorCodes, js.ToString(it), "cyan")
	}
	return strings.Join(parts, ", ")
}

// Wraptext is logUtils.wraptext: it word-wraps text to width columns, counting UTF-16
// code units and not counting ANSI colour codes, and re-emits the last colour code at
// the start of each continued line.
//
// It reproduces the original's quirks: the scan is text.match(/(?:(\x1B\[\d+m)|\n|(.))([\s\S]*)/),
// so "\r", U+2028 and U+2029 (which neither "\n" nor "." match) are dropped; only
// U+0020 counts as a break opportunity; a word longer than width is cut, and a cut
// can fall between the halves of a surrogate pair, each half then printing as U+FFFD.
//
// The original never terminates for width < 1; here such widths behave as 1.
func Wraptext(width int, text string) string {
	if width < 1 {
		width = 1
	}
	var result [][]uint16
	var captured [][]uint16
	var color []uint16
	charlen := 0
	rest := jsstr.ToUTF16(text)
	for {
		// Find the first unit the pattern can start at.
		p := 0
		for p < len(rest) && (rest[p] == '\r' || rest[p] == 0x2028 || rest[p] == 0x2029) {
			p++
		}
		if p == len(rest) {
			break
		}
		if n := colorCodeLen16(rest, p); n > 0 {
			color = rest[p : p+n]
			captured = append(captured, color)
			rest = rest[p+n:]
			continue
		}
		c := rest[p]
		rest = rest[p+1:]
		if c != '\n' {
			if charlen == 0 && color != nil {
				captured = append(captured, color)
			}
			captured = append(captured, []uint16{c})
			charlen++
			if charlen <= width || c == ' ' {
				continue
			}
			// Roll everything back to the last space.
			tmp := -1
			for i := len(captured) - 1; i >= 0; i-- {
				if len(captured[i]) == 1 && captured[i][0] == ' ' {
					tmp = i
					break
				}
			}
			// captured.slice(tmp === -1 ? tmp : tmp + 1) and captured.slice(0, tmp):
			// with no space, the last unit is carried over and slice(0, -1) drops it.
			from, keep := tmp+1, tmp
			if tmp == -1 {
				from, keep = len(captured)-1, len(captured)-1
			}
			var back []uint16
			for _, tok := range captured[from:] {
				back = append(back, tok...)
			}
			rest = append(back, rest...)
			captured = captured[:keep]
		}
		result = append(result, joinTokens(captured))
		captured = nil
		charlen = 0
	}
	result = append(result, joinTokens(captured))
	lines := make([]string, len(result))
	for i, r := range result {
		lines[i] = jsstr.FromUTF16WellFormed(r)
	}
	return strings.Join(lines, "\n")
}

func joinTokens(tokens [][]uint16) []uint16 {
	var out []uint16
	for _, t := range tokens {
		out = append(out, t...)
	}
	return out
}

// colorCodeLen16 is colorCodeLen over code units.
func colorCodeLen16(u []uint16, i int) int {
	if i+1 >= len(u) || u[i] != 0x1b || u[i+1] != '[' {
		return 0
	}
	j := i + 2
	for j < len(u) && u[j] >= '0' && u[j] <= '9' {
		j++
	}
	if j == i+2 || j >= len(u) || u[j] != 'm' {
		return 0
	}
	return j + 1 - i
}

// Table is logUtils.table: it wraps texts[i] to widths[i] and lays the columns out
// side by side, padding each cell (measured without colour codes) to its width. A
// row's trailing columns that have no line are not padded, as in the original.
// A missing texts entry is treated as "" (the original throws a TypeError).
func Table(widths []int, texts []string) string {
	var rows [][]string // rows[j][i] is line j of column i; "" where absent
	var rowLen []int    // JavaScript row.length: highest column set plus one
	for i, w := range widths {
		text := ""
		if i < len(texts) {
			text = texts[i]
		}
		for j, line := range strings.Split(Wraptext(w, text), "\n") {
			if j == len(rows) {
				rows = append(rows, make([]string, len(widths)))
				rowLen = append(rowLen, 0)
			}
			rows[j][i] = line
			rowLen[j] = i + 1
		}
	}
	lines := make([]string, len(rows))
	for j, row := range rows {
		var b strings.Builder
		for i := 0; i < rowLen[j]; i++ {
			b.WriteString(row[i])
			if diff := widths[i] - jsstr.Length(Uncolor(row[i])); diff > 0 {
				b.WriteString(strings.Repeat(" ", diff))
			}
		}
		lines[j] = b.String()
	}
	return strings.Join(lines, "\n")
}
