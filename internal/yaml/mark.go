package yaml

import (
	"strconv"
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// mark is js-yaml's Mark (lib/js-yaml/mark.js): a position in the input buffer, in
// UTF-16 code units, with a zero-based line and column. js-yaml also records a file
// name; grunt never passes one, so it is always null here.
type mark struct {
	buffer   []uint16
	position int
	line     int
	column   int
}

// snippetBreak reports whether c ends a line for Mark.getSnippet, which stops at
// NUL, CR, LF, NEL, LINE SEPARATOR and PARAGRAPH SEPARATOR.
func snippetBreak(c uint16) bool {
	switch c {
	case 0x00, '\r', '\n', 0x85, 0x2028, 0x2029:
		return true
	}
	return false
}

// snippet is Mark.prototype.getSnippet with its defaults (indent 4, maxLength 75): the
// line around the position, cut to " ... " when long, and a caret under the position.
// ok is false when the buffer is empty, where js-yaml returns null.
func (m *mark) snippet() (s string, ok bool) {
	const indent = 4
	// maxLength / 2 - 1 with maxLength 75: the limit is the float 36.5.
	const half = 75.0/2 - 1
	if len(m.buffer) == 0 {
		return "", false
	}
	head, start := "", m.position
	for start > 0 && !snippetBreak(m.buffer[start-1]) {
		start--
		if float64(m.position-start) > half {
			head = " ... "
			start += 5
			break
		}
	}
	tail, end := "", m.position
	for end < len(m.buffer) && !snippetBreak(m.buffer[end]) {
		end++
		if float64(end-m.position) > half {
			tail = " ... "
			end -= 5
			break
		}
	}
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", indent))
	b.WriteString(head)
	b.WriteString(jsstr.FromUTF16WellFormed(m.buffer[start:end]))
	b.WriteString(tail)
	b.WriteByte('\n')
	b.WriteString(strings.Repeat(" ", indent+m.position-start+len(head)))
	b.WriteByte('^')
	return b.String(), true
}

// String is Mark.prototype.toString(false): "at line L, column C:" and the snippet.
func (m *mark) String() string {
	where := "at line " + strconv.Itoa(m.line+1) + ", column " + strconv.Itoa(m.column+1)
	if s, ok := m.snippet(); ok {
		where += ":\n" + s
	}
	return where
}

// newException builds js-yaml's YAMLException (lib/js-yaml/exception.js). Its message is
// the reason followed by the mark, when there is one. The reason and mark are kept as
// own properties, as js-yaml sets them.
func newException(reason string, m *mark) *js.Error {
	props := js.NewObject()
	props.Set("reason", reason)
	msg := reason
	if m == nil {
		props.Set("mark", js.Undefined)
	} else {
		msg += " " + m.String()
		props.Set("mark", js.ObjectOf(
			"name", nil,
			"buffer", jsstr.FromUTF16WellFormed(m.buffer),
			"position", m.position,
			"line", m.line,
			"column", m.column,
		))
	}
	return &js.Error{Name: "YAMLException", Message: msg, Props: props}
}
