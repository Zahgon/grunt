package yaml

import (
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
)

// Block scalar chomping modes.
type chomping int

const (
	chompingClip chomping = iota + 1
	chompingStrip
	chompingKeep
)

// fromHexCode returns the value of hexadecimal digit c, or -1.
func fromHexCode(c int) int {
	if '0' <= c && c <= '9' {
		return c - '0'
	}
	if lc := c | 0x20; 'a' <= lc && lc <= 'f' {
		return lc - 'a' + 10
	}
	return -1
}

// escapedHexLen returns the digit count of the \x, \u or \U escape named by c, or 0.
func escapedHexLen(c int) int {
	switch c {
	case 'x':
		return 2
	case 'u':
		return 4
	case 'U':
		return 8
	}
	return 0
}

// fromDecimalCode returns the value of decimal digit c, or -1.
func fromDecimalCode(c int) int {
	if '0' <= c && c <= '9' {
		return c - '0'
	}
	return -1
}

// simpleEscapes maps the character after a backslash to its replacement for the
// escapes of simpleEscapeSequence in loader.js.
var simpleEscapes = map[int]uint16{
	'0': 0x00, 'a': 0x07, 'b': 0x08, 't': 0x09, '\t': 0x09, 'n': 0x0A, 'v': 0x0B,
	'f': 0x0C, 'r': 0x0D, 'e': 0x1B, ' ': ' ', '"': '"', '/': '/', '\\': '\\',
	'N': 0x85, '_': 0xA0, 'L': 0x2028, 'P': 0x2029,
}

// charFromCodepoint is loader.js's charFromCodepoint. c is the int32 that
// "(hexResult << 4) + digit" leaves, so an 8-digit \U escape can be negative or past
// U+10FFFF; String.fromCharCode then truncates each unit to 16 bits, as uint16 does.
func charFromCodepoint(c int32) []uint16 {
	if c <= 0xFFFF {
		return []uint16{uint16(c)}
	}
	return []uint16{uint16(((c - 0x10000) >> 10) + 0xD800), uint16(((c - 0x10000) & 0x3FF) + 0xDC00)}
}

// isNonPrintable is PATTERN_NON_PRINTABLE, evaluated over code units: C0 and C1
// controls other than tab, line feed, carriage return and NEL, U+FFFE, U+FFFF and
// unpaired surrogates.
func isNonPrintable(s []uint16) bool {
	for i, c := range s {
		switch {
		case c <= 0x08, c == 0x0B, c == 0x0C, 0x0E <= c && c <= 0x1F,
			0x7F <= c && c <= 0x84, 0x86 <= c && c <= 0x9F, c == 0xFFFE, c == 0xFFFF:
			return true
		case 0xD800 <= c && c <= 0xDBFF:
			if i+1 >= len(s) || s[i+1] < 0xDC00 || s[i+1] > 0xDFFF {
				return true
			}
		case 0xDC00 <= c && c <= 0xDFFF:
			if i == 0 || s[i-1] < 0xD800 || s[i-1] > 0xDBFF {
				return true
			}
		}
	}
	return false
}

// captureSegment appends input[start:end] to the scalar being read. Quoted scalars
// accept any character from tab up; other scalars reject non-printable characters.
func (l *loader) captureSegment(start, end int, checkJSON bool) {
	if start >= end {
		return
	}
	seg := l.input[start:end]
	if checkJSON {
		for _, c := range seg {
			if c != 0x09 && c < 0x20 {
				l.fail("expected valid JSON character")
			}
		}
	} else if isNonPrintable(seg) {
		l.fail("the stream contains non-printable characters")
	}
	l.scalar = append(l.scalar, seg...)
}

// finishScalar exposes the scalar read so far as the node's result.
func (l *loader) finishScalar() {
	l.kind = kindScalar
	l.result = jsstr.FromUTF16WellFormed(l.scalar)
}

// writeFoldedLines folds count line breaks: one becomes a space, n become n-1
// newlines.
func (l *loader) writeFoldedLines(count int) {
	if count == 1 {
		l.scalar = append(l.scalar, ' ')
	} else {
		l.scalar = appendRepeat(l.scalar, '\n', count-1)
	}
}

// appendRepeat appends n copies of c (none when n <= 0, like common.repeat).
func appendRepeat(s []uint16, c uint16, n int) []uint16 {
	for ; n > 0; n-- {
		s = append(s, c)
	}
	return s
}

// readPlainScalar reads an unquoted scalar, folding continuation lines indented at
// least nodeIndent. It reports false, consuming nothing, when no scalar starts here.
func (l *loader) readPlainScalar(nodeIndent int, withinFlowCollection bool) bool {
	ch := l.at(l.position)
	if isWSOrEOL(ch) || isFlowIndicator(ch) {
		return false
	}
	switch ch {
	case '#', '&', '*', '!', '|', '>', '\'', '"', '%', '@', '`':
		return false
	case '?', '-':
		following := l.at(l.position + 1)
		if isWSOrEOL(following) || withinFlowCollection && isFlowIndicator(following) {
			return false
		}
	}

	l.scalar = l.scalar[:0]
	captureStart, captureEnd := l.position, l.position
	hasPendingContent := false
	var line, lineStart, lineIndent int

	for ch != 0 {
		if ch == ':' {
			following := l.at(l.position + 1)
			if isWSOrEOL(following) || withinFlowCollection && isFlowIndicator(following) {
				break
			}
		} else if ch == '#' {
			if isWSOrEOL(l.at(l.position - 1)) {
				break
			}
		} else if (l.position == l.lineStart && l.testDocumentSeparator()) ||
			withinFlowCollection && isFlowIndicator(ch) {
			break
		} else if isEOL(ch) {
			line, lineStart, lineIndent = l.line, l.lineStart, l.lineIndent
			l.skipSeparationSpace(false)
			if l.lineIndent >= nodeIndent {
				hasPendingContent = true
				ch = l.at(l.position)
				continue
			}
			l.position = captureEnd
			l.line, l.lineStart, l.lineIndent = line, lineStart, lineIndent
			break
		}

		if hasPendingContent {
			l.captureSegment(captureStart, captureEnd, false)
			l.writeFoldedLines(l.line - line)
			captureStart, captureEnd = l.position, l.position
			hasPendingContent = false
		}
		if !isWhiteSpace(ch) {
			captureEnd = l.position + 1
		}
		l.position++
		ch = l.at(l.position)
	}

	l.captureSegment(captureStart, captureEnd, false)
	if len(l.scalar) == 0 {
		return false
	}
	l.finishScalar()
	return true
}

// readSingleQuotedScalar reads a single-quoted scalar, where a doubled quote stands
// for one and line breaks fold. (js-yaml's nodeIndent argument only feeds a warning.)
func (l *loader) readSingleQuotedScalar() bool {
	if l.at(l.position) != '\'' {
		return false
	}
	l.scalar = l.scalar[:0]
	l.position++
	captureStart, captureEnd := l.position, l.position

	for ch := l.at(l.position); ch != 0; ch = l.at(l.position) {
		switch {
		case ch == '\'':
			l.captureSegment(captureStart, l.position, true)
			l.position++
			if l.at(l.position) != '\'' {
				l.finishScalar()
				return true
			}
			// A doubled quote stands for one quote: keep the second.
			captureStart = l.position
			l.position++
			captureEnd = l.position
		case isEOL(ch):
			l.captureSegment(captureStart, captureEnd, true)
			l.writeFoldedLines(l.skipSeparationSpace(false))
			captureStart, captureEnd = l.position, l.position
		case l.position == l.lineStart && l.testDocumentSeparator():
			l.fail("unexpected end of the document within a single quoted scalar")
		default:
			l.position++
			captureEnd = l.position
		}
	}
	l.fail("unexpected end of the stream within a single quoted scalar")
	return false
}

// readDoubleQuotedScalar reads a "..." scalar with its escapes; line breaks fold and
// an escaped line break is dropped with the following indentation.
func (l *loader) readDoubleQuotedScalar() bool {
	if l.at(l.position) != '"' {
		return false
	}
	l.scalar = l.scalar[:0]
	l.position++
	captureStart, captureEnd := l.position, l.position

	for ch := l.at(l.position); ch != 0; ch = l.at(l.position) {
		switch {
		case ch == '"':
			l.captureSegment(captureStart, l.position, true)
			l.position++
			l.finishScalar()
			return true
		case ch == '\\':
			l.captureSegment(captureStart, l.position, true)
			l.position++
			ch = l.at(l.position)
			if isEOL(ch) {
				l.skipSeparationSpace(false)
			} else if r, ok := simpleEscapes[ch]; ok {
				l.scalar = append(l.scalar, r)
				l.position++
			} else if n := escapedHexLen(ch); n > 0 {
				var code int32
				for ; n > 0; n-- {
					l.position++
					d := fromHexCode(l.at(l.position))
					if d < 0 {
						l.fail("expected hexadecimal character")
					}
					code = code<<4 + int32(d)
				}
				l.scalar = append(l.scalar, charFromCodepoint(code)...)
				l.position++
			} else {
				l.fail("unknown escape sequence")
			}
			captureStart, captureEnd = l.position, l.position
		case isEOL(ch):
			l.captureSegment(captureStart, captureEnd, true)
			l.writeFoldedLines(l.skipSeparationSpace(false))
			captureStart, captureEnd = l.position, l.position
		case l.position == l.lineStart && l.testDocumentSeparator():
			l.fail("unexpected end of the document within a double quoted scalar")
		default:
			l.position++
			captureEnd = l.position
		}
	}
	l.fail("unexpected end of the stream within a double quoted scalar")
	return false
}

// readBlockScalar reads a literal (|) or folded (>) scalar with its optional chomping
// and indentation indicators; content lines are indented more than nodeIndent unless
// an indicator says otherwise.
func (l *loader) readBlockScalar(nodeIndent int) bool {
	var folding bool
	ch := l.at(l.position)
	switch ch {
	case '|':
		folding = false
	case '>':
		folding = true
	default:
		return false
	}
	chomp := chompingClip
	didReadContent, detectedIndent := false, false
	textIndent, emptyLines := nodeIndent, 0
	atMoreIndented := false

	l.scalar = l.scalar[:0]

	for ch != 0 {
		l.position++
		ch = l.at(l.position)
		if ch == '+' || ch == '-' {
			if chomp != chompingClip {
				l.fail("repeat of a chomping mode identifier")
			}
			chomp = chompingStrip
			if ch == '+' {
				chomp = chompingKeep
			}
		} else if d := fromDecimalCode(ch); d >= 0 {
			if d == 0 {
				l.fail("bad explicit indentation width of a block scalar; it cannot be less than one")
			} else if detectedIndent {
				l.fail("repeat of an indentation width identifier")
			}
			textIndent = nodeIndent + d - 1
			detectedIndent = true
		} else {
			break
		}
	}

	if isWhiteSpace(ch) {
		for {
			l.position++
			ch = l.at(l.position)
			if !isWhiteSpace(ch) {
				break
			}
		}
		if ch == '#' {
			for {
				l.position++
				ch = l.at(l.position)
				if isEOL(ch) || ch == 0 {
					break
				}
			}
		}
	}

	for ch != 0 {
		l.readLineBreak()
		l.lineIndent = 0
		ch = l.at(l.position)

		for (!detectedIndent || l.lineIndent < textIndent) && ch == ' ' {
			l.lineIndent++
			l.position++
			ch = l.at(l.position)
		}
		if !detectedIndent && l.lineIndent > textIndent {
			textIndent = l.lineIndent
		}
		if isEOL(ch) {
			emptyLines++
			continue
		}

		// End of the scalar: perform the chomping.
		if l.lineIndent < textIndent {
			switch chomp {
			case chompingKeep:
				l.scalar = appendRepeat(l.scalar, '\n', emptyLines+boolInt(didReadContent))
			case chompingClip:
				if didReadContent {
					l.scalar = append(l.scalar, '\n')
				}
			}
			break
		}

		if folding {
			switch {
			case isWhiteSpace(ch):
				// More-indented lines are not folded, except for the first content
				// line (cf. Example 8.1).
				atMoreIndented = true
				l.scalar = appendRepeat(l.scalar, '\n', emptyLines+boolInt(didReadContent))
			case atMoreIndented:
				atMoreIndented = false
				l.scalar = appendRepeat(l.scalar, '\n', emptyLines+1)
			case emptyLines == 0:
				// One line break folds into a space.
				if didReadContent {
					l.scalar = append(l.scalar, ' ')
				}
			default:
				l.scalar = appendRepeat(l.scalar, '\n', emptyLines)
			}
		} else {
			// Literal style keeps every line break except the header's.
			l.scalar = appendRepeat(l.scalar, '\n', emptyLines+boolInt(didReadContent))
		}

		didReadContent, detectedIndent = true, true
		emptyLines = 0
		captureStart := l.position
		for !isEOL(ch) && ch != 0 {
			l.position++
			ch = l.at(l.position)
		}
		l.captureSegment(captureStart, l.position, false)
	}
	l.finishScalar()
	return true
}

// boolInt is 1 for true and 0 for false.
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
