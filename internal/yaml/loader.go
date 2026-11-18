package yaml

import (
	"strings"
	"unicode/utf8"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// Node contexts, as in loader.js.
type nodeContext int

const (
	contextFlowIn nodeContext = iota + 1
	contextFlowOut
	contextBlockIn
	contextBlockOut
)

// Node kinds. js-yaml leaves state.kind null for an alias; kindNone stands for that.
const (
	kindNone     = ""
	kindScalar   = "scalar"
	kindSequence = "sequence"
	kindMapping  = "mapping"
)

// loader is js-yaml's State. The input is held as UTF-16 code units followed by the
// 0 terminator loader.js appends, so every position, column and character test
// matches the original.
type loader struct {
	input  []uint16
	length int // the input length before the terminator was appended
	schema *schema

	position   int
	line       int
	lineStart  int
	lineIndent int

	totalMergeKeys int
	documents      []any

	hasVersion bool
	tagMap     map[string]string
	anchorMap  map[string]any

	// tag is state.tag; hasTag is false where js-yaml's is null. A verbatim "!<>"
	// yields the empty tag, so the empty string cannot stand for null.
	tag    string
	hasTag bool
	// anchor is state.anchor; anchors are never empty, so "" stands for null.
	anchor string
	kind   string
	result any

	// scalar accumulates the code units of the scalar being read (state.result while
	// it is a string under construction).
	scalar []uint16
}

// newLoader prepares the input as loadDocuments does: a trailing line break is added
// when missing, a leading BOM is dropped, and NUL characters are rejected. Invalid
// UTF-8 in src is replaced as Node's UTF-8 decoder would when reading the file.
func newLoader(src string, s *schema) *loader {
	if !utf8.ValidString(src) {
		src = js.DecodeUTF8([]byte(src))
	}
	input := jsstr.ToUTF16(src)
	if n := len(input); n != 0 {
		if input[n-1] != '\n' && input[n-1] != '\r' {
			input = append(input, '\n')
		}
		if input[0] == 0xFEFF {
			input = input[1:]
		}
	}
	l := &loader{input: input, length: len(input), schema: s}
	for i, c := range input {
		if c == 0 {
			l.position = i
			l.fail("null byte is not allowed in input")
		}
	}
	l.input = append(l.input, 0)
	return l
}

// run is the document loop of loadDocuments: leading spaces count as indentation of
// the first line, then documents are read to the end of the stream.
func (l *loader) run() {
	for l.at(l.position) == ' ' {
		l.lineIndent++
		l.position++
	}
	for l.position < l.length-1 {
		l.readDocument()
	}
}

// at is String.prototype.charCodeAt; out of range it returns -1 where JavaScript
// returns NaN, which likewise compares unequal to every character.
func (l *loader) at(i int) int {
	if i < 0 || i >= len(l.input) {
		return -1
	}
	return int(l.input[i])
}

// slice is state.input.slice(start, end) as a Go string.
func (l *loader) slice(start, end int) string {
	return jsstr.FromUTF16WellFormed(l.input[start:end])
}

// fail is throwError: it raises a YAMLException marked at the current position.
func (l *loader) fail(reason string) {
	throwErr(newException(reason, &mark{
		buffer:   l.input,
		position: l.position,
		line:     l.line,
		column:   l.position - l.lineStart,
	}))
}

// Character classes of loader.js (is_EOL, is_WHITE_SPACE, is_WS_OR_EOL,
// is_FLOW_INDICATOR).

func isEOL(c int) bool { return c == '\n' || c == '\r' }

func isWhiteSpace(c int) bool { return c == '\t' || c == ' ' }

func isWSOrEOL(c int) bool { return c == '\t' || c == ' ' || c == '\n' || c == '\r' }

func isFlowIndicator(c int) bool {
	return c == ',' || c == '[' || c == ']' || c == '{' || c == '}'
}

// handleYAMLDirective is directiveHandlers.YAML. js-yaml only warns about minor
// versions and line breaks; the errors are kept.
func (l *loader) handleYAMLDirective(args []string) {
	if l.hasVersion {
		l.fail("duplication of %YAML directive")
	}
	if len(args) != 1 {
		l.fail("YAML directive accepts exactly one argument")
	}
	major, minor, ok := strings.Cut(args[0], ".")
	if !ok || !isDigits(major) || !isDigits(minor) {
		l.fail("ill-formed argument of the YAML directive")
	}
	// parseInt(major, 10) !== 1, without overflowing on long digit strings.
	if strings.TrimLeft(major, "0") != "1" {
		l.fail("unacceptable YAML version of the document")
	}
	l.hasVersion = true
}

// isDigits reports whether s is a non-empty run of ASCII digits ([0-9]+).
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isDecDigit(s[i]) {
			return false
		}
	}
	return true
}

// handleTagDirective is directiveHandlers.TAG.
func (l *loader) handleTagDirective(args []string) {
	if len(args) != 2 {
		l.fail("TAG directive accepts exactly two arguments")
	}
	handle, prefix := args[0], args[1]
	if !isTagHandle(jsstr.ToUTF16(handle)) {
		l.fail("ill-formed tag handle (first argument) of the TAG directive")
	}
	if _, ok := l.tagMap[handle]; ok {
		l.fail("there is a previously declared suffix for \"" + handle + "\" tag handle")
	}
	if !isTagURI(jsstr.ToUTF16(prefix)) {
		l.fail("ill-formed tag prefix (second argument) of the TAG directive")
	}
	l.tagMap[handle] = prefix
}

// storeAnchor is "state.anchorMap[name] = value". anchorMap is a plain object, so the
// name "__proto__" sets its prototype and is never found by hasOwnProperty later.
func (l *loader) storeAnchor(name string, value any) {
	if name != "__proto__" {
		l.anchorMap[name] = value
	}
}

// readLineBreak consumes LF, CR or CRLF and starts a new line.
func (l *loader) readLineBreak() {
	switch l.at(l.position) {
	case '\n':
		l.position++
	case '\r':
		l.position++
		if l.at(l.position) == '\n' {
			l.position++
		}
	default:
		l.fail("a line break is expected")
	}
	l.line++
	l.lineStart = l.position
}

// skipSeparationSpace skips white space, line breaks and (when allowed) comments,
// tracking the indentation of the last line, and returns the number of line breaks.
// js-yaml's checkIndent argument only feeds a warning and is omitted.
func (l *loader) skipSeparationSpace(allowComments bool) int {
	lineBreaks := 0
	ch := l.at(l.position)
	for ch != 0 {
		for isWhiteSpace(ch) {
			l.position++
			ch = l.at(l.position)
		}
		if allowComments && ch == '#' {
			for {
				l.position++
				ch = l.at(l.position)
				if ch == '\n' || ch == '\r' || ch == 0 {
					break
				}
			}
		}
		if !isEOL(ch) {
			break
		}
		l.readLineBreak()
		ch = l.at(l.position)
		lineBreaks++
		l.lineIndent = 0
		for ch == ' ' {
			l.lineIndent++
			l.position++
			ch = l.at(l.position)
		}
	}
	return lineBreaks
}

// testDocumentSeparator reports whether "---" or "..." followed by white space or the
// end starts at the current position, which the caller has checked is a line start.
func (l *loader) testDocumentSeparator() bool {
	p := l.position
	ch := l.at(p)
	if (ch == '-' || ch == '.') && ch == l.at(p+1) && ch == l.at(p+2) {
		ch = l.at(p + 3)
		return ch == 0 || isWSOrEOL(ch)
	}
	return false
}

// readTagProperty reads a "!", "!!name", "!handle!name" or "!<verbatim>" tag and
// resolves it against the %TAG handles into state.tag.
func (l *loader) readTagProperty() bool {
	ch := l.at(l.position)
	if ch != '!' {
		return false
	}
	if l.hasTag {
		l.fail("duplication of a tag property")
	}
	l.position++
	ch = l.at(l.position)

	isVerbatim, isNamed := false, false
	var tagHandle string
	switch ch {
	case '<':
		isVerbatim = true
		l.position++
		ch = l.at(l.position)
	case '!':
		isNamed = true
		tagHandle = "!!"
		l.position++
		ch = l.at(l.position)
	default:
		tagHandle = "!"
	}

	start := l.position
	var tagName []uint16
	if isVerbatim {
		// A do-while in loader.js: the first character after "<" is never tested,
		// so "!<>x>" is the tag ">x".
		for {
			l.position++
			ch = l.at(l.position)
			if ch == 0 || ch == '>' {
				break
			}
		}
		if l.position >= l.length {
			l.fail("unexpected end of the stream within a verbatim tag")
		}
		tagName = l.input[start:l.position]
		l.position++
	} else {
		for ch != 0 && !isWSOrEOL(ch) {
			if ch == '!' {
				if isNamed {
					l.fail("tag suffix cannot contain exclamation marks")
				}
				handle := l.input[start-1 : l.position+1]
				if !isTagHandle(handle) {
					l.fail("named tag handle cannot contain such characters")
				}
				tagHandle = jsstr.FromUTF16WellFormed(handle)
				isNamed = true
				start = l.position + 1
			}
			l.position++
			ch = l.at(l.position)
		}
		tagName = l.input[start:l.position]
		for _, c := range tagName {
			if isFlowIndicator(int(c)) {
				l.fail("tag suffix cannot contain flow indicator characters")
			}
		}
	}

	name := jsstr.FromUTF16WellFormed(tagName)
	if len(tagName) > 0 && !isTagURI(tagName) {
		l.fail("tag name cannot contain such characters: " + name)
	}

	switch prefix, declared := l.tagMap[tagHandle]; {
	case isVerbatim:
		l.tag = name
	case declared:
		l.tag = prefix + name
	case tagHandle == "!":
		l.tag = "!" + name
	case tagHandle == "!!":
		l.tag = "tag:yaml.org,2002:" + name
	default:
		l.fail("undeclared tag handle \"" + tagHandle + "\"")
	}
	l.hasTag = true
	return true
}

// isTagHandle is PATTERN_TAG_HANDLE, /^(?:!|!!|![a-z\-]+!)$/i.
func isTagHandle(s []uint16) bool {
	n := len(s)
	if n == 0 || s[0] != '!' || s[n-1] != '!' {
		return false
	}
	for _, c := range s[1:max(n-1, 1)] {
		if !(c == '-' || 'a' <= c|0x20 && c|0x20 <= 'z') {
			return false
		}
	}
	return true
}

// isTagURI is PATTERN_TAG_URI,
// /^(?:!|[^,\[\]\{\}])(?:%[0-9a-f]{2}|[0-9a-z\-#;\/\?:@&=\+\$,_\.!~\*'\(\)\[\]])*$/i,
// over code units.
func isTagURI(s []uint16) bool {
	if len(s) == 0 {
		return false
	}
	switch s[0] {
	case ',', '[', ']', '{', '}':
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c == '%' {
			if i+2 >= len(s) || fromHexCode(int(s[i+1])) < 0 || fromHexCode(int(s[i+2])) < 0 {
				return false
			}
			i += 2
			continue
		}
		if c < 0x80 && (isASCIIAlnum(c) || strings.IndexByte("-#;/?:@&=+$,_.!~*'()[]", byte(c)) >= 0) {
			continue
		}
		return false
	}
	return true
}

// isASCIIAlnum reports whether c is an ASCII letter or digit.
func isASCIIAlnum(c uint16) bool {
	return '0' <= c && c <= '9' || 'a' <= c|0x20 && c|0x20 <= 'z'
}

// readAnchorProperty reads an "&name" anchor into state.anchor.
func (l *loader) readAnchorProperty() bool {
	if l.at(l.position) != '&' {
		return false
	}
	if l.anchor != "" {
		l.fail("duplication of an anchor property")
	}
	l.position++
	start := l.position
	for ch := l.at(l.position); ch != 0 && !isWSOrEOL(ch) && !isFlowIndicator(ch); ch = l.at(l.position) {
		l.position++
	}
	if l.position == start {
		l.fail("name of an anchor node must contain at least one character")
	}
	l.anchor = l.slice(start, l.position)
	return true
}

// readAlias reads a "*name" alias; its result is the anchored value itself, so
// aliases share mappings and sequences.
func (l *loader) readAlias() bool {
	if l.at(l.position) != '*' {
		return false
	}
	l.position++
	start := l.position
	for ch := l.at(l.position); ch != 0 && !isWSOrEOL(ch) && !isFlowIndicator(ch); ch = l.at(l.position) {
		l.position++
	}
	if l.position == start {
		l.fail("name of an alias node must contain at least one character")
	}
	alias := l.slice(start, l.position)
	value, ok := l.anchorMap[alias]
	if !ok {
		l.fail("unidentified alias \"" + alias + "\"")
	}
	l.result = value
	l.skipSeparationSpace(true)
	return true
}

// indentStatus compares a line's indentation with its parent's: 1 deeper, 0 equal,
// -1 shallower.
func indentStatus(lineIndent, parentIndent int) int {
	switch {
	case lineIndent > parentIndent:
		return 1
	case lineIndent == parentIndent:
		return 0
	}
	return -1
}

// composeNode reads one node, with its tag and anchor properties, in context ctx at
// an indentation deeper than parentIndent, and resolves its tag. It reports whether
// anything (a property or content) was read.
func (l *loader) composeNode(parentIndent int, ctx nodeContext, allowToSeek, allowCompact bool) bool {
	status := 1
	atNewLine, hasContent := false, false

	l.tag, l.hasTag = "", false
	l.anchor = ""
	l.kind = kindNone
	l.result = nil

	allowBlockStyles := ctx == contextBlockOut || ctx == contextBlockIn
	allowBlockScalars, allowBlockCollections := allowBlockStyles, allowBlockStyles

	if allowToSeek && l.skipSeparationSpace(true) > 0 {
		atNewLine = true
		status = indentStatus(l.lineIndent, parentIndent)
	}

	if status == 1 {
		for l.readTagProperty() || l.readAnchorProperty() {
			if l.skipSeparationSpace(true) > 0 {
				atNewLine = true
				allowBlockCollections = allowBlockStyles
				status = indentStatus(l.lineIndent, parentIndent)
			} else {
				allowBlockCollections = false
			}
		}
	}

	if allowBlockCollections {
		allowBlockCollections = atNewLine || allowCompact
	}

	if status == 1 || ctx == contextBlockOut {
		flowIndent := parentIndent + 1
		if ctx == contextFlowIn || ctx == contextFlowOut {
			flowIndent = parentIndent
		}
		blockIndent := l.position - l.lineStart

		if status == 1 {
			if allowBlockCollections && (l.readBlockSequence(blockIndent) || l.readBlockMapping(blockIndent, flowIndent)) ||
				l.readFlowCollection(flowIndent) {
				hasContent = true
			} else {
				if allowBlockScalars && l.readBlockScalar(flowIndent) ||
					l.readSingleQuotedScalar() ||
					l.readDoubleQuotedScalar() {
					hasContent = true
				} else if l.readAlias() {
					hasContent = true
					if l.hasTag || l.anchor != "" {
						l.fail("alias node should not have any properties")
					}
				} else if l.readPlainScalar(flowIndent, ctx == contextFlowIn) {
					hasContent = true
					if !l.hasTag {
						l.tag, l.hasTag = "?", true
					}
				}
				if l.anchor != "" {
					l.storeAnchor(l.anchor, l.result)
				}
			}
		} else if status == 0 {
			// Block sequences may share their parent's indentation.
			// http://www.yaml.org/spec/1.2/spec.html#id2799784
			hasContent = allowBlockCollections && l.readBlockSequence(blockIndent)
		}
	}

	if l.hasTag && l.tag != "!" {
		l.resolveTag()
	}
	return l.hasTag || l.anchor != "" || hasContent
}

// resolveTag applies the node's tag: implicit resolution for "?" (plain scalars), or
// the explicitly named type.
func (l *loader) resolveTag() {
	if l.tag == "?" {
		// Implicit resolution only applies to scalars; the check matters for an
		// explicit "!<?> [0]".
		if l.result != nil && l.kind != kindScalar {
			l.fail("unacceptable node kind for !<?> tag; it should be \"scalar\", not \"" + kindName(l.kind) + "\"")
		}
		for _, t := range l.schema.implicit {
			if t.resolve(l.result) {
				l.result = l.construct(t)
				l.tag = t.tag
				if l.anchor != "" {
					l.storeAnchor(l.anchor, l.result)
				}
				break
			}
		}
		return
	}
	kind := l.kind
	if kind == kindNone {
		kind = "fallback"
	}
	t, ok := l.schema.typeMap[kind][l.tag]
	if !ok {
		l.fail("unknown tag !<" + l.tag + ">")
	}
	if l.result != nil && t.kind != l.kind {
		l.fail("unacceptable node kind for !<" + l.tag + "> tag; it should be \"" + t.kind + "\", not \"" + kindName(l.kind) + "\"")
	}
	if !t.resolve(l.result) {
		l.fail("cannot resolve a node with !<" + l.tag + "> explicit tag")
	}
	l.result = l.construct(t)
	if l.anchor != "" {
		l.storeAnchor(l.anchor, l.result)
	}
}

// construct runs t's constructor on the node's result. A constructor error is thrown
// as is, without a mark, as the JavaScript exception would be.
func (l *loader) construct(t *yamlType) any {
	v, err := t.construct(l.result)
	if err != nil {
		throwErr(err)
	}
	return v
}

// kindName renders state.kind the way string concatenation does: null for an alias.
func kindName(kind string) string {
	if kind == kindNone {
		return "null"
	}
	return kind
}

// readDocument reads the directives, the "---" marker, the root node and the "..."
// marker of one document, appending the root to the documents read.
func (l *loader) readDocument() {
	hasDirectives := false

	l.hasVersion = false
	l.tagMap = map[string]string{}
	l.anchorMap = map[string]any{}

	for l.at(l.position) != 0 {
		l.skipSeparationSpace(true)
		ch := l.at(l.position)
		if l.lineIndent > 0 || ch != '%' {
			break
		}

		hasDirectives = true
		l.position++
		ch = l.at(l.position)
		start := l.position
		for ch != 0 && !isWSOrEOL(ch) {
			l.position++
			ch = l.at(l.position)
		}
		name := l.slice(start, l.position)
		var args []string
		if name == "" {
			l.fail("directive name must not be less than one character in length")
		}

		for ch != 0 {
			for isWhiteSpace(ch) {
				l.position++
				ch = l.at(l.position)
			}
			if ch == '#' {
				for {
					l.position++
					ch = l.at(l.position)
					if ch == 0 || isEOL(ch) {
						break
					}
				}
				break
			}
			if isEOL(ch) {
				break
			}
			start = l.position
			for ch != 0 && !isWSOrEOL(ch) {
				l.position++
				ch = l.at(l.position)
			}
			args = append(args, l.slice(start, l.position))
		}

		if ch != 0 {
			l.readLineBreak()
		}
		// Unknown directives only produce a warning.
		switch name {
		case "YAML":
			l.handleYAMLDirective(args)
		case "TAG":
			l.handleTagDirective(args)
		}
	}

	l.skipSeparationSpace(true)

	if l.lineIndent == 0 && l.at(l.position) == '-' && l.at(l.position+1) == '-' && l.at(l.position+2) == '-' {
		l.position += 3
		l.skipSeparationSpace(true)
	} else if hasDirectives {
		l.fail("directives end mark is expected")
	}

	l.composeNode(l.lineIndent-1, contextBlockOut, false, true)
	l.skipSeparationSpace(true)

	l.documents = append(l.documents, l.result)

	if l.position == l.lineStart && l.testDocumentSeparator() {
		if l.at(l.position) == '.' {
			l.position += 3
			l.skipSeparationSpace(true)
		}
		return
	}
	if l.position < l.length-1 {
		l.fail("end of the stream or a document separator is expected")
	}
}
