package jsrt

import (
	"math"
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsregexp"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsnum"
	"github.com/gruntjs/grunt-go/js"
)

// syntaxError carries a SyntaxError out of the recursive-descent parser, with the
// source position it was reported at.
type syntaxError struct {
	msg string
	pos int
}

// parser is a recursive-descent parser for sloppy-mode JavaScript that reports the
// first error with V8's message, at the token where V8 reports it.
type parser struct {
	lx      lexer
	tok     token
	prevEnd int
	fn      *funcCtx
	// parenIdents holds identifiers written in parentheses, which are not valid
	// binding targets.
	parenIdents map[*ident]bool
}

// funcCtx is the per-function parsing state.
type funcCtx struct {
	info   *funcInfo
	blocks []*blockCtx
	labels []labelCtx
	// breakable and loops count the enclosing switch/loop statements.
	breakable, loops int
	params           map[string]bool
}

type labelCtx struct {
	name   string
	isLoop bool
}

// blockCtx tracks the declarations of one scope for the redeclaration early errors.
type blockCtx struct {
	info    *scopeInfo
	lexical map[string]bool
	funcs   map[string]bool
	// vars holds the var names declared in this scope or any nested one.
	vars map[string]bool
	// catchParams holds simple catch parameter names, which var may redeclare.
	catchParams map[string]bool
	isFunction  bool
}

func (p *parser) fail(msg string) {
	panic(&syntaxError{msg, p.tok.pos})
}

// failAt reports msg at a given token.
func (p *parser) failAt(t token, msg string) {
	panic(&syntaxError{msg, t.pos})
}

// tryParse runs f and returns the SyntaxError it reports, if any.
func (p *parser) tryParse(f func()) (err *syntaxError) {
	defer func() {
		if r := recover(); r != nil {
			se, ok := r.(*syntaxError)
			if !ok {
				panic(r)
			}
			err = se
		}
	}()
	f()
	return nil
}

// unexpected reports t as V8's ReportUnexpectedToken does.
func (p *parser) unexpected(t token) {
	p.failAt(t, unexpectedMessage(t))
}

// unexpectedMessage is V8's message for an unexpected token.
func unexpectedMessage(t token) string {
	switch t.kind {
	case tEOF:
		return "Unexpected end of input"
	case tNumber:
		return "Unexpected number"
	case tString:
		return "Unexpected string"
	case tTemplate:
		return "Unexpected template string"
	case tRegExp:
		return "Unexpected regular expression"
	case tPrivate:
		return "Unexpected identifier '" + t.text + "'"
	case tIllegal:
		if t.msg != "" {
			return t.msg
		}
		return "Invalid or unexpected token"
	case tIdent:
		if t.escaped && isStrictReserved(t.text) {
			return "Keyword must not contain escaped characters"
		}
		switch t.text {
		case "await":
			// await is an identifier in scripts but V8 still names it a reserved word.
			return "Unexpected reserved word"
		case "of", "async", "get", "set", "accessor", "using":
			// Contextual keywords have tokens of their own in V8.
			return "Unexpected token '" + t.text + "'"
		case "implements", "interface", "package", "private", "protected", "public":
			// V8 reports these future reserved words without their text.
			return "Unexpected identifier 'undefined'"
		}
		return "Unexpected identifier '" + t.text + "'"
	case tKeyword:
		if t.escaped {
			return "Keyword must not contain escaped characters"
		}
		if t.text == "enum" {
			return "Unexpected reserved word"
		}
	}
	return "Unexpected token '" + t.text + "'"
}

func isStrictReserved(name string) bool {
	switch name {
	case "let", "static", "yield", "implements", "interface", "package", "private", "protected", "public", "await":
		return true
	}
	return false
}

func (p *parser) next() {
	p.prevEnd = p.tok.end
	p.tok = p.lx.next()
}

func (p *parser) is(text string) bool {
	return (p.tok.kind == tPunct || p.tok.kind == tKeyword) && p.tok.text == text
}

func (p *parser) isIdent(name string) bool {
	return p.tok.kind == tIdent && p.tok.text == name && !p.tok.escaped
}

func (p *parser) eat(text string) bool {
	if p.is(text) {
		p.next()
		return true
	}
	return false
}

func (p *parser) expect(text string) {
	if !p.is(text) {
		p.unexpected(p.tok)
	}
	p.next()
}

// semicolon applies automatic semicolon insertion.
func (p *parser) semicolon() {
	if p.eat(";") {
		return
	}
	if p.is("}") || p.tok.kind == tEOF || p.tok.nlBefore {
		return
	}
	p.unexpected(p.tok)
}

// save and restore let the parser look ahead past several tokens.
type parserState struct {
	pos     int
	tok     token
	prevEnd int
}

func (p *parser) save() parserState { return parserState{p.lx.pos, p.tok, p.prevEnd} }

func (p *parser) restore(s parserState) {
	p.lx.pos, p.tok, p.prevEnd = s.pos, s.tok, s.prevEnd
}

// peek returns the token after the current one.
func (p *parser) peek() token {
	s := p.save()
	p.next()
	t := p.tok
	p.restore(s)
	return t
}

// parseFunctionConstructor parses source as V8's Function constructor does: the
// whole text "(function anonymous(params\n) {\nbody\n})" as one script holding a
// single function expression.
func parseFunctionConstructor(params []string, body string) (fi *funcInfo, err error) {
	src := "(function anonymous(" + strings.Join(params, ",") + "\n) {\n" + body + "\n})"
	defer func() {
		if r := recover(); r != nil {
			se, ok := r.(*syntaxError)
			if !ok {
				panic(r)
			}
			fi, err = nil, js.NewSyntaxError(se.msg)
		}
	}()
	p := &parser{lx: lexer{src: src}}
	p.next()
	p.fn = &funcCtx{info: &funcInfo{}}
	p.pushBlock(true)
	e := p.parseExpression(false)
	p.semicolon()
	if p.tok.kind != tEOF {
		p.unexpected(p.tok)
	}
	f, ok := e.(*funcLit)
	if !ok {
		p.fail("Unexpected token")
	}
	return f.fn, nil
}

// Scope bookkeeping.

func (p *parser) pushBlock(isFunction bool) *blockCtx {
	b := &blockCtx{info: &scopeInfo{}, lexical: map[string]bool{}, funcs: map[string]bool{}, vars: map[string]bool{}, isFunction: isFunction}
	p.fn.blocks = append(p.fn.blocks, b)
	return b
}

func (p *parser) popBlock() *scopeInfo {
	b := p.fn.blocks[len(p.fn.blocks)-1]
	p.fn.blocks = p.fn.blocks[:len(p.fn.blocks)-1]
	return b.info
}

func (p *parser) curBlock() *blockCtx { return p.fn.blocks[len(p.fn.blocks)-1] }

func redeclared(name string) string {
	return "Identifier '" + name + "' has already been declared"
}

func (p *parser) declareLexical(name, kind string) {
	b := p.curBlock()
	if b.lexical[name] || b.funcs[name] || b.vars[name] || (b.isFunction && p.fn.params[name]) || b.catchParams[name] {
		p.fail(redeclared(name))
	}
	if name == "let" {
		p.fail("let is disallowed as a lexically bound name")
	}
	b.lexical[name] = true
	if kind == "const" {
		b.info.consts = append(b.info.consts, name)
	} else {
		b.info.lets = append(b.info.lets, name)
	}
}

func (p *parser) declareVar(name string) {
	for i := len(p.fn.blocks) - 1; i >= 0; i-- {
		b := p.fn.blocks[i]
		if b.lexical[name] || (!b.isFunction && b.funcs[name]) {
			p.fail(redeclared(name))
		}
		b.vars[name] = true
		if b.isFunction {
			break
		}
	}
	addName(&p.fn.info.varNames, name)
}

func addName(list *[]string, name string) {
	for _, n := range *list {
		if n == name {
			return
		}
	}
	*list = append(*list, name)
}

func (p *parser) declareFunction(fi *funcInfo) {
	b := p.curBlock()
	if b.isFunction {
		if b.lexical[fi.name] {
			p.fail(redeclared(fi.name))
		}
		b.funcs[fi.name] = true
		addName(&p.fn.info.varNames, fi.name)
	} else {
		// A block-level function is lexical, but sloppy mode lets functions
		// redeclare each other (Annex B.3.2.4).
		if b.lexical[fi.name] || b.vars[fi.name] {
			p.fail(redeclared(fi.name))
		}
		b.funcs[fi.name] = true
		// Annex B.3.2.1: the function is also var-declared in the enclosing function
		// unless that would conflict with a lexical declaration.
		conflict := false
		for i := len(p.fn.blocks) - 2; i >= 0; i-- {
			if p.fn.blocks[i].lexical[fi.name] {
				conflict = true
			}
			if p.fn.blocks[i].isFunction {
				break
			}
		}
		if !conflict && !p.fn.params[fi.name] {
			addName(&p.fn.info.varNames, fi.name)
		}
	}
	for i, f := range b.info.funcs {
		if f.name == fi.name {
			b.info.funcs[i] = fi
			return
		}
	}
	b.info.funcs = append(b.info.funcs, fi)
}

// boundNames returns the identifiers a binding pattern declares.
func boundNames(n node, out []string) []string {
	switch x := n.(type) {
	case *ident:
		return append(out, x.name)
	case *arrayPattern:
		for _, e := range x.elems {
			if e != nil {
				out = boundNames(e.target, out)
			}
		}
		if x.rest != nil {
			out = boundNames(x.rest, out)
		}
	case *objectPattern:
		for _, pr := range x.props {
			out = boundNames(pr.elem.target, out)
		}
		if x.rest != nil {
			out = boundNames(x.rest, out)
		}
	}
	return out
}

// Statements.

func (p *parser) parseStatementList(end string) []stmt {
	var list []stmt
	for !p.is(end) && p.tok.kind != tEOF {
		list = append(list, p.parseStatementListItem())
	}
	return list
}

func (p *parser) parseStatementListItem() stmt {
	switch {
	case p.is("function"):
		return p.parseFunctionDeclaration()
	case p.is("const"):
		d := p.parseVarDecl("const", false)
		p.semicolon()
		return d
	case p.is("class"):
		p.parseClassHead(true)
	case p.isLetDeclaration():
		d := p.parseVarDecl("let", false)
		p.semicolon()
		return d
	}
	return p.parseStatement()
}

// isLetDeclaration reports whether the identifier let starts a declaration here.
func (p *parser) isLetDeclaration() bool {
	if !p.isIdent("let") {
		return false
	}
	t := p.peek()
	return t.kind == tIdent || (t.kind == tPunct && (t.text == "[" || t.text == "{")) ||
		(t.kind == tKeyword && !t.nlBefore && t.text != "in" && t.text != "instanceof")
}

func (p *parser) parseStatement() stmt {
	pos := p.tok.pos
	if p.tok.kind == tKeyword && !p.tok.escaped {
		switch p.tok.text {
		case "var":
			d := p.parseVarDecl("var", false)
			p.semicolon()
			return d
		case "if":
			return p.parseIf()
		case "for":
			return p.parseFor()
		case "while":
			p.next()
			p.expect("(")
			test := p.parseExpression(false)
			p.expect(")")
			body := p.parseLoopBody()
			return &whileStmt{base{pos}, test, body}
		case "do":
			p.next()
			body := p.parseLoopBody()
			if !p.is("while") {
				p.unexpected(p.tok)
			}
			p.next()
			p.expect("(")
			test := p.parseExpression(false)
			p.expect(")")
			// A semicolon after do-while is always optional (ES2015 ASI rule).
			p.eat(";")
			return &doWhile{base{pos}, body, test}
		case "continue":
			return p.parseContinue()
		case "break":
			return p.parseBreak()
		case "return":
			p.next()
			var x expr
			if !p.is(";") && !p.is("}") && p.tok.kind != tEOF && !p.tok.nlBefore {
				x = p.parseExpression(false)
			}
			p.semicolon()
			return &returnStmt{base{pos}, x}
		case "throw":
			p.next()
			if p.tok.nlBefore {
				p.fail("Illegal newline after throw")
			}
			x := p.parseExpression(false)
			p.semicolon()
			return &throwStmt{base{pos}, x}
		case "try":
			return p.parseTry()
		case "switch":
			return p.parseSwitch()
		case "with":
			p.next()
			p.expect("(")
			obj := p.parseExpression(false)
			p.expect(")")
			body := p.parseSubStatement()
			return &withStmt{base{pos}, obj, body}
		case "debugger":
			p.next()
			p.semicolon()
			return &emptyStmt{base{pos}}
		case "function":
			// Annex B.3.4: a function declaration as the body of an if statement
			// behaves as if it were wrapped in a block.
			p.fail("In non-strict mode code, functions can only be declared at top level, inside a block, or as the body of an if statement.")
		case "class", "const":
			p.unexpected(p.tok)
		}
	}
	if p.is("{") {
		return p.parseBlock()
	}
	if p.is(";") {
		p.next()
		return &emptyStmt{base{pos}}
	}
	if p.isLetDeclaration() {
		p.fail("Lexical declaration cannot appear in a single-statement context")
	}
	if p.tok.kind == tIdent && p.peek().kind == tPunct && p.peek().text == ":" {
		return p.parseLabeled()
	}
	x := p.parseExpression(false)
	p.semicolon()
	return &exprStmt{base{pos}, x}
}

// parseSubStatement parses the body of if/with/labels, where sloppy mode accepts a
// function declaration as if it were in its own block.
func (p *parser) parseSubStatement() stmt {
	if p.is("function") {
		pos := p.tok.pos
		p.pushBlock(false)
		d := p.parseFunctionDeclaration()
		scope := p.popBlock()
		return &block{base{pos}, []stmt{d}, scope}
	}
	return p.parseStatement()
}

func (p *parser) parseLoopBody() stmt {
	p.fn.loops++
	p.fn.breakable++
	defer func() { p.fn.loops--; p.fn.breakable-- }()
	if p.is("function") {
		p.fail("In non-strict mode code, functions can only be declared at top level, inside a block, or as the body of an if statement.")
	}
	return p.parseStatement()
}

func (p *parser) parseLabeled() stmt {
	pos := p.tok.pos
	name := p.tok.text
	for _, l := range p.fn.labels {
		if l.name == name {
			p.fail("Label '" + name + "' has already been declared")
		}
	}
	p.next()
	p.expect(":")
	isLoop := p.is("for") || p.is("while") || p.is("do")
	p.fn.labels = append(p.fn.labels, labelCtx{name, isLoop})
	defer func() { p.fn.labels = p.fn.labels[:len(p.fn.labels)-1] }()
	var body stmt
	if p.is("function") {
		body = p.parseSubStatement()
	} else {
		body = p.parseStatement()
	}
	return &labeled{base{pos}, name, body}
}

func (p *parser) parseContinue() stmt {
	pos := p.tok.pos
	p.next()
	label := ""
	if p.tok.kind == tIdent && !p.tok.nlBefore {
		label = p.tok.text
		found := false
		for _, l := range p.fn.labels {
			if l.name == label {
				found = true
				if !l.isLoop {
					p.fail("Illegal continue statement: '" + label + "' does not denote an iteration statement")
				}
			}
		}
		if !found {
			p.fail("Undefined label '" + label + "'")
		}
		p.next()
	} else if p.fn.loops == 0 {
		p.fail("Illegal continue statement: no surrounding iteration statement")
	}
	p.semicolon()
	return &continueStmt{base{pos}, label}
}

func (p *parser) parseBreak() stmt {
	pos := p.tok.pos
	p.next()
	label := ""
	if p.tok.kind == tIdent && !p.tok.nlBefore {
		label = p.tok.text
		found := false
		for _, l := range p.fn.labels {
			found = found || l.name == label
		}
		if !found {
			p.fail("Undefined label '" + label + "'")
		}
		p.next()
	} else if p.fn.breakable == 0 {
		p.fail("Illegal break statement")
	}
	p.semicolon()
	return &breakStmt{base{pos}, label}
}

func (p *parser) parseBlock() *block {
	pos := p.tok.pos
	p.expect("{")
	p.pushBlock(false)
	body := p.parseStatementList("}")
	scope := p.popBlock()
	p.expect("}")
	return &block{base{pos}, body, scope}
}

func (p *parser) parseIf() stmt {
	pos := p.tok.pos
	p.next()
	p.expect("(")
	test := p.parseExpression(false)
	p.expect(")")
	cons := p.parseSubStatement()
	var alt stmt
	if p.eat("else") {
		alt = p.parseSubStatement()
	}
	return &ifStmt{base{pos}, test, cons, alt}
}

// parseVarDecl parses a var, let or const declaration list. In a for head (noIn)
// it leaves initializer checks to the caller.
func (p *parser) parseVarDecl(kind string, forHead bool) *varDecl {
	pos := p.tok.pos
	p.next()
	d := &varDecl{base: base{pos}, kind: kind}
	for {
		target := p.parseBindingTarget()
		names := boundNames(target, nil)
		for _, n := range names {
			if kind == "var" {
				p.declareVar(n)
			} else {
				p.declareLexical(n, kind)
			}
		}
		var init expr
		if p.eat("=") {
			init = p.parseAssignment(forHead)
		} else if !forHead || !(p.isIdent("of") || p.is("in")) {
			if kind == "const" {
				p.fail("Missing initializer in const declaration")
			}
			if _, simple := target.(*ident); !simple {
				p.fail("Missing initializer in destructuring declaration")
			}
		}
		d.decls = append(d.decls, &declarator{target, init})
		if !p.eat(",") {
			break
		}
	}
	return d
}

// parseBindingTarget parses a BindingIdentifier or BindingPattern. Patterns are
// parsed as array or object literals and converted, as V8 does, so invalid
// elements are reported as V8 reports them.
func (p *parser) parseBindingTarget() node {
	pos := p.tok.pos
	switch {
	case p.is("[") || p.is("{"):
		pat := p.toPattern(p.parseLiteral(true))
		p.checkBindingPattern(pat)
		return pat
	case p.tok.kind == tIdent:
		name := p.tok.text
		p.next()
		return &ident{base{pos}, name}
	}
	p.unexpected(p.tok)
	return nil
}

// checkBindingPattern rejects the targets a declaration may not have: members
// and parenthesized names.
func (p *parser) checkBindingPattern(n node) {
	switch x := n.(type) {
	case *member:
		p.fail("Illegal property in declaration context")
	case *ident:
		if p.parenIdents[x] {
			p.fail("Invalid destructuring assignment target")
		}
	case *arrayPattern:
		for _, e := range x.elems {
			if e != nil {
				p.checkBindingPattern(e.target)
			}
		}
		if x.rest != nil {
			p.checkBindingPattern(x.rest)
		}
	case *objectPattern:
		for _, pr := range x.props {
			p.checkBindingPattern(pr.elem.target)
		}
		if x.rest != nil {
			if _, ok := x.rest.(*ident); !ok {
				p.fail("`...` must be followed by an identifier in declaration contexts")
			}
			p.checkBindingPattern(x.rest)
		}
	}
}

func (p *parser) parseBindingElement() *patternElem {
	target := p.parseBindingTarget()
	el := &patternElem{target: target}
	if p.eat("=") {
		el.def = p.parseAssignment(false)
	}
	return el
}

func (p *parser) parseFor() stmt {
	pos := p.tok.pos
	p.next()
	if p.isIdent("await") {
		p.fail("for await is not supported")
	}
	p.expect("(")
	p.pushBlock(false)
	var init node
	switch {
	case p.is(";"):
	case p.is("var"):
		init = p.parseVarDecl("var", true)
	case p.is("const"):
		init = p.parseVarDecl("const", true)
	case p.isLetDeclaration():
		init = p.parseVarDecl("let", true)
	default:
		startTok := p.tok
		x := p.parseExpressionNoIn()
		if p.is("in") || p.isIdent("of") {
			if p.isIdent("of") && startTok.kind == tIdent && startTok.text == "let" && !startTok.escaped {
				p.fail("The left-hand side of a for-of loop may not be 'let'.")
			}
			init = p.toAssignTarget(x, true)
		} else {
			init = x
		}
	}
	if p.is("in") || p.isIdent("of") {
		of := p.isIdent("of")
		if d, ok := init.(*varDecl); ok {
			loopKind := "for-in"
			if of {
				loopKind = "for-of"
			}
			if len(d.decls) != 1 {
				p.fail("Invalid left-hand side in " + loopKind + " loop: Must have a single binding.")
			}
			if d.decls[0].init != nil {
				p.fail(loopKind + " loop variable declaration may not have an initializer.")
			}
		} else if init == nil {
			p.unexpected(p.tok)
		}
		p.next()
		var right expr
		if of {
			right = p.parseAssignment(false)
		} else {
			right = p.parseExpression(false)
		}
		p.expect(")")
		body := p.parseLoopBody()
		scope := p.popBlock()
		return &forInOf{base{pos}, of, init, right, body, scope}
	}
	if d, ok := init.(*varDecl); ok {
		for _, dc := range d.decls {
			if dc.init == nil {
				if d.kind == "const" {
					p.fail("Missing initializer in const declaration")
				}
				if _, simple := dc.target.(*ident); !simple {
					p.fail("Missing initializer in destructuring declaration")
				}
			}
		}
	}
	p.expect(";")
	var test, upd expr
	if !p.is(";") {
		test = p.parseExpression(false)
	}
	p.expect(";")
	if !p.is(")") {
		upd = p.parseExpression(false)
	}
	p.expect(")")
	body := p.parseLoopBody()
	scope := p.popBlock()
	return &forStmt{base{pos}, init, test, upd, body, scope}
}

func (p *parser) parseTry() stmt {
	pos := p.tok.pos
	p.next()
	t := &tryStmt{base: base{pos}}
	t.body = p.parseBlock()
	if p.eat("catch") {
		b := p.pushBlock(false)
		if p.eat("(") {
			t.param = p.parseBindingTarget()
			b.catchParams = map[string]bool{}
			for _, n := range boundNames(t.param, nil) {
				if b.catchParams[n] {
					p.fail(redeclared(n))
				}
				b.catchParams[n] = true
				b.info.lets = append(b.info.lets, n)
			}
			if _, simple := t.param.(*ident); !simple {
				// Only a simple catch parameter may be redeclared by var (B.3.4).
				for n := range b.catchParams {
					b.lexical[n] = true
				}
			}
			p.expect(")")
		}
		t.catchScope = b.info
		// The catch block is its own scope, but its declarations may not clash
		// with the parameter.
		hpos := p.tok.pos
		p.expect("{")
		inner := p.pushBlock(false)
		inner.catchParams = b.catchParams
		body := p.parseStatementList("}")
		scope := p.popBlock()
		p.expect("}")
		p.popBlock()
		t.handler = &block{base{hpos}, body, scope}
	}
	if p.eat("finally") {
		t.finalizer = p.parseBlock()
	}
	if t.handler == nil && t.finalizer == nil {
		p.fail("Missing catch or finally after try")
	}
	return t
}

func (p *parser) parseSwitch() stmt {
	pos := p.tok.pos
	p.next()
	p.expect("(")
	disc := p.parseExpression(false)
	p.expect(")")
	p.expect("{")
	s := &switchStmt{base: base{pos}, disc: disc}
	p.pushBlock(false)
	p.fn.breakable++
	hasDefault := false
	for !p.is("}") {
		c := &switchCase{}
		if p.eat("default") {
			if hasDefault {
				p.fail("More than one default clause in switch statement")
			}
			hasDefault = true
		} else {
			p.expect("case")
			c.test = p.parseExpression(false)
		}
		p.expect(":")
		for !p.is("case") && !p.is("default") && !p.is("}") && p.tok.kind != tEOF {
			c.body = append(c.body, p.parseStatementListItem())
		}
		s.cases = append(s.cases, c)
	}
	p.fn.breakable--
	s.scope = p.popBlock()
	p.next()
	return s
}

// parseClassHead reports a class: V8's error for a malformed head, otherwise that
// classes are not supported.
func (p *parser) parseClassHead(declaration bool) {
	p.next()
	if p.tok.kind == tIdent {
		p.next()
	} else if declaration {
		p.unexpected(p.tok)
	}
	if p.eat("extends") {
		p.parseLHS()
	}
	if !p.is("{") {
		p.unexpected(p.tok)
	}
	p.fail("class syntax is not supported")
}

// Functions.

func (p *parser) parseFunctionDeclaration() stmt {
	pos := p.tok.pos
	p.next()
	if p.is("*") {
		p.fail("generator functions are not supported")
	}
	if p.tok.kind != tIdent {
		p.unexpected(p.tok)
	}
	name := p.tok.text
	p.next()
	fi := p.parseFunctionRest(pos, name, false)
	p.declareFunction(fi)
	return &funcDecl{base{pos}, fi}
}

// parseFunctionRest parses the parameter list and body that follow a function's
// name.
func (p *parser) parseFunctionRest(start int, name string, method bool) *funcInfo {
	fi := &funcInfo{name: name, method: method, simple: true}
	outer := p.fn
	p.fn = &funcCtx{info: fi, params: map[string]bool{}}
	defer func() { p.fn = outer }()
	p.expect("(")
	p.parseParams(fi)
	p.expect(")")
	p.parseFunctionBody(fi)
	fi.source = p.lx.src[start:p.prevEnd]
	return fi
}

func (p *parser) parseParams(fi *funcInfo) {
	counting := true
	for !p.is(")") {
		if p.eat("...") {
			fi.rest = p.parseBindingTarget()
			fi.simple = false
			if !p.is(")") {
				p.fail("Rest parameter must be last formal parameter")
			}
			break
		}
		el := p.parseBindingElement()
		if _, ok := el.target.(*ident); !ok || el.def != nil {
			fi.simple = false
		}
		if el.def != nil {
			counting = false
		}
		if counting {
			fi.length++
		}
		fi.params = append(fi.params, el)
		if !p.is(")") {
			p.expect(",")
		}
	}
	p.checkParams(fi)
}

// checkParams records parameter names; duplicates are allowed only in a sloppy
// simple parameter list.
func (p *parser) checkParams(fi *funcInfo) {
	var names []string
	for _, el := range fi.params {
		names = boundNames(el.target, names)
	}
	if fi.rest != nil {
		names = boundNames(fi.rest, names)
	}
	for _, n := range names {
		if p.fn.params[n] && (!fi.simple || fi.arrow || fi.method) {
			p.fail("Duplicate parameter name not allowed in this context")
		}
		p.fn.params[n] = true
	}
}

func (p *parser) parseFunctionBody(fi *funcInfo) {
	p.expect("{")
	p.pushBlock(true)
	fi.body = p.parseStatementList("}")
	fi.scope = p.popBlock()
	p.expect("}")
}

// parseArrowFunction parses "=> body" for already-parsed parameters.
func (p *parser) parseArrowFunction(start int, params []*patternElem, rest node) expr {
	if p.tok.nlBefore {
		p.unexpected(p.tok)
	}
	p.expect("=>")
	fi := &funcInfo{arrow: true, params: params, rest: rest, simple: rest == nil}
	counting := true
	for _, el := range params {
		if _, ok := el.target.(*ident); !ok || el.def != nil {
			fi.simple = false
		}
		if el.def != nil {
			counting = false
		}
		if counting {
			fi.length++
		}
	}
	outer := p.fn
	p.fn = &funcCtx{info: fi, params: map[string]bool{}}
	p.checkParams(fi)
	if p.is("{") {
		p.parseFunctionBody(fi)
	} else {
		p.pushBlock(true)
		fi.exprBody = p.parseAssignment(false)
		fi.scope = p.popBlock()
	}
	p.fn = outer
	fi.source = p.lx.src[start:p.prevEnd]
	return &funcLit{base{start}, fi}
}

// Expressions.

func (p *parser) parseExpression(noIn bool) expr {
	pos := p.tok.pos
	if p.is("...") {
		// V8 parses this as the rest parameter of an arrow function, which
		// cannot follow here.
		p.parseArrowRest()
	}
	x := p.parseAssignment(noIn)
	if !p.is(",") {
		return x
	}
	list := []expr{x}
	for p.eat(",") {
		list = append(list, p.parseAssignment(noIn))
	}
	return &sequence{base{pos}, list}
}

func (p *parser) parseExpressionNoIn() expr { return p.parseExpression(true) }

var assignOps = map[string]bool{
	"=": true, "+=": true, "-=": true, "*=": true, "/=": true, "%=": true, "**=": true,
	"<<=": true, ">>=": true, ">>>=": true, "&=": true, "|=": true, "^=": true,
	"&&=": true, "||=": true, "??=": true,
}

func (p *parser) parseAssignment(noIn bool) expr {
	return p.parseAssignmentCover(noIn, false)
}

// parseAssignmentCover parses an AssignmentExpression. With allowCover it accepts
// the shorthand initializers "{a = 1}" that are valid only if an enclosing literal
// turns out to be a destructuring pattern; the enclosing parse checks them.
func (p *parser) parseAssignmentCover(noIn, allowCover bool) expr {
	pos := p.tok.pos
	// An identifier directly followed by => is an arrow function's parameter.
	if p.tok.kind == tIdent {
		if t := p.peek(); t.kind == tPunct && t.text == "=>" && !t.nlBefore {
			name := p.tok.text
			p.next()
			return p.parseArrowFunction(pos, []*patternElem{{target: &ident{base{pos}, name}}}, nil)
		}
		if p.isIdent("async") {
			if t := p.peek(); !t.nlBefore && (t.kind == tIdent || (t.kind == tKeyword && t.text == "function")) {
				p.fail("async functions are not supported")
			}
		}
	}
	left := p.parseConditional(noIn)
	if arrow, ok := left.(*arrowParams); ok {
		if !p.is("=>") {
			p.unexpected(p.tok)
		}
		return p.parseArrowFunction(arrow.pos, arrow.params, arrow.rest)
	}
	if p.is("=>") {
		if p.tok.nlBefore {
			p.unexpected(p.tok)
		}
		p.fail("Malformed arrow function parameter list")
	}
	if p.tok.kind == tPunct && assignOps[p.tok.text] {
		op := p.tok.text
		var target node
		if op == "=" {
			target = p.toAssignTarget(left, false)
		} else {
			target = p.checkSimpleTarget(left, "Invalid left-hand side in assignment")
		}
		p.next()
		value := p.parseAssignment(noIn)
		return &assign{base{pos}, op, target, value}
	}
	if !allowCover {
		p.checkNoCoverInit(left)
	}
	return left
}

// arrowParams is the parenthesized list "(a, b = 1, ...c)" before "=>". It is a
// temporary node: parseAssignment always turns it into a function.
type arrowParams struct {
	base
	params []*patternElem
	rest   node
}

func (*arrowParams) isExpr() {}

// coverInit marks a shorthand property with an initializer, "{a = 1}", which is
// only valid when the object literal turns out to be a pattern.
type coverInit struct {
	base
	name string
	def  expr
}

func (*coverInit) isExpr() {}

func (p *parser) checkNoCoverInit(x expr) {
	switch e := x.(type) {
	case *objectLit:
		for _, pr := range e.props {
			if _, bad := pr.value.(*coverInit); bad {
				p.fail("Invalid shorthand property initializer")
			}
			if pr.kind == propInit {
				p.checkNoCoverInit(pr.value)
			}
		}
	case *arrayLit:
		for _, el := range e.elems {
			if el != nil {
				p.checkNoCoverInit(el)
			}
		}
	}
}

// checkSimpleTarget checks the target of compound assignment and ++/--. A call
// is accepted, as V8 does for web compatibility, and fails when evaluated.
func (p *parser) checkSimpleTarget(x expr, msg string) node {
	switch t := x.(type) {
	case *ident, *member:
		return t
	case *call:
		return &invalidTarget{base: t.base, call: t, msg: msg}
	}
	p.fail(msg)
	return nil
}

// toAssignTarget converts a parsed expression into an assignment target,
// reinterpreting array and object literals as destructuring patterns.
func (p *parser) toAssignTarget(x expr, forHead bool) node {
	msg := "Invalid left-hand side in assignment"
	if forHead {
		msg = "Invalid left-hand side in for-loop"
	}
	switch t := x.(type) {
	case *ident, *member:
		return t
	case *call:
		return &invalidTarget{base: t.base, call: t, msg: msg}
	case *arrayLit:
		if t.parens {
			p.fail(msg)
		}
		return p.toPattern(t)
	case *objectLit:
		if t.parens {
			p.fail(msg)
		}
		return p.toPattern(t)
	}
	p.fail(msg)
	return nil
}

func (p *parser) toPattern(x expr) node {
	const bad = "Invalid destructuring assignment target"
	switch t := x.(type) {
	case *ident, *member:
		return t
	case *arrayLit:
		if t.parens {
			p.fail(bad)
		}
		pat := &arrayPattern{base: t.base}
		for i, el := range t.elems {
			if el == nil {
				pat.elems = append(pat.elems, nil)
				continue
			}
			if sp, ok := el.(*spread); ok {
				if i != len(t.elems)-1 {
					p.fail("Rest element must be last element")
				}
				pat.rest = p.toPattern(sp.x)
				break
			}
			pat.elems = append(pat.elems, p.toPatternElem(el))
		}
		return pat
	case *objectLit:
		if t.parens {
			p.fail(bad)
		}
		pat := &objectPattern{base: t.base}
		for i, pr := range t.props {
			switch pr.kind {
			case propSpread:
				if i != len(t.props)-1 {
					p.fail("Rest element must be last element")
				}
				pat.rest = p.toPattern(pr.value)
			case propInit, propShorthand:
				pp := &patternProp{key: pr.key, computed: pr.computed}
				if ci, ok := pr.value.(*coverInit); ok {
					pp.elem = patternElem{target: &ident{ci.base, ci.name}, def: ci.def}
				} else {
					pp.elem = *p.toPatternElem(pr.value)
				}
				pat.props = append(pat.props, pp)
			default:
				p.fail(bad)
			}
		}
		return pat
	}
	p.fail(bad)
	return nil
}

func (p *parser) toPatternElem(x expr) *patternElem {
	if a, ok := x.(*assign); ok && a.op == "=" {
		return &patternElem{target: a.target, def: a.value}
	}
	return &patternElem{target: p.toPattern(x)}
}

func (p *parser) parseConditional(noIn bool) expr {
	pos := p.tok.pos
	test := p.parseLogical(noIn)
	if _, ok := test.(*arrowParams); ok {
		return test
	}
	if !p.eat("?") {
		return test
	}
	cons := p.parseAssignment(false)
	p.expect(":")
	alt := p.parseAssignment(noIn)
	return &conditional{base{pos}, test, cons, alt}
}

// binaryPrec is V8's precedence table for binary operators.
var binaryPrec = map[string]int{
	"??": 3, "||": 4, "&&": 5, "|": 6, "^": 7, "&": 8,
	"==": 9, "!=": 9, "===": 9, "!==": 9,
	"<": 10, ">": 10, "<=": 10, ">=": 10, "instanceof": 10, "in": 10,
	"<<": 11, ">>": 11, ">>>": 11,
	"+": 12, "-": 12, "*": 13, "/": 13, "%": 13, "**": 14,
}

func (p *parser) binaryOp(noIn bool) (string, int) {
	if p.tok.kind != tPunct && p.tok.kind != tKeyword {
		return "", 0
	}
	if p.tok.text == "in" && noIn {
		return "", 0
	}
	if p.tok.kind == tKeyword && p.tok.escaped {
		return "", 0
	}
	prec, ok := binaryPrec[p.tok.text]
	if !ok {
		return "", 0
	}
	return p.tok.text, prec
}

// parseLogical is V8's ParseLogicalExpression: an expression of BitwiseOR
// precedence or higher, continued either with || and && or as a chain of ??. A ??
// after || or && (or the reverse) is not consumed, so the caller reports it, which
// is how mixing them without parentheses fails.
func (p *parser) parseLogical(noIn bool) expr {
	pos, startTok := p.tok.pos, p.tok
	left := p.parseBinary(binaryPrec["|"]-1, noIn)
	if _, ok := left.(*arrowParams); ok {
		return left
	}
	switch op, _ := p.binaryOp(noIn); op {
	case "&&", "||":
		return p.binaryTail(pos, startTok, left, binaryPrec["??"], noIn)
	case "??":
		for p.is("??") {
			p.next()
			right := p.parseBinary(binaryPrec["|"]-1, noIn)
			left = &logical{base{pos}, "??", left, right, false}
		}
	}
	return left
}

// parseBinary is precedence climbing over the binary operators with a precedence
// above minPrec.
func (p *parser) parseBinary(minPrec int, noIn bool) expr {
	pos, startTok := p.tok.pos, p.tok
	left := p.parseUnary()
	if _, ok := left.(*arrowParams); ok {
		return left
	}
	return p.binaryTail(pos, startTok, left, minPrec, noIn)
}

// binaryTail continues a binary expression whose left operand is parsed.
func (p *parser) binaryTail(pos int, startTok token, left expr, minPrec int, noIn bool) expr {
	const unaryBeforeExp = "Unary operator used immediately before exponentiation expression. Parenthesis must be used to disambiguate operator precedence"
	for {
		op, prec := p.binaryOp(noIn)
		if prec == 0 || prec <= minPrec {
			return left
		}
		if op == "**" {
			// -x ** y is an early error; V8 checks the folded literal too.
			if u, ok := left.(*unary); ok && startTok.pos == u.pos {
				p.fail(unaryBeforeExp)
			}
			isUnaryTok := startTok.kind == tPunct && (startTok.text == "-" || startTok.text == "+" || startTok.text == "~" || startTok.text == "!")
			switch left.(type) {
			case *numberLit, *boolLit:
				if isUnaryTok {
					p.fail(unaryBeforeExp)
				}
			}
		}
		p.next()
		var right expr
		if op == "**" {
			// Exponentiation is right-associative.
			right = p.parseBinary(prec-1, noIn)
		} else {
			right = p.parseBinary(prec, noIn)
		}
		if op == "&&" || op == "||" {
			left = &logical{base{pos}, op, left, right, false}
		} else {
			left = foldBinary(pos, op, left, right)
		}
	}
}

// foldBinary builds a binary node, folding operations on two number literals as
// V8's parser does (the folded literal is what V8 prints in error messages).
func foldBinary(pos int, op string, l, r expr) expr {
	ln, lok := l.(*numberLit)
	rn, rok := r.(*numberLit)
	if lok && rok {
		x, y := ln.value, rn.value
		var v float64
		switch op {
		case "+":
			v = x + y
		case "-":
			v = x - y
		case "*":
			v = x * y
		case "/":
			v = x / y
		case "**":
			v = jsnum.Pow(x, y)
		case "|":
			v = float64(js.ToInt32(x) | js.ToInt32(y))
		case "&":
			v = float64(js.ToInt32(x) & js.ToInt32(y))
		case "^":
			v = float64(js.ToInt32(x) ^ js.ToInt32(y))
		case "<<":
			v = float64(js.ToInt32(x) << (js.ToUint32(y) & 31))
		case ">>":
			v = float64(js.ToInt32(x) >> (js.ToUint32(y) & 31))
		case ">>>":
			v = float64(js.ToUint32(x) >> (js.ToUint32(y) & 31))
		default:
			return &binary{base{pos}, op, l, r}
		}
		return &numberLit{base{pos}, v}
	}
	return &binary{base{pos}, op, l, r}
}

func (p *parser) parseUnary() expr {
	pos := p.tok.pos
	if p.tok.kind == tPunct || p.tok.kind == tKeyword && !p.tok.escaped {
		switch op := p.tok.text; op {
		case "!", "-", "+", "~", "typeof", "void", "delete":
			p.next()
			x := p.parseUnary()
			if _, ok := x.(*arrowParams); ok {
				p.unexpected(p.tok)
			}
			return foldUnary(pos, op, x)
		case "++", "--":
			p.next()
			x := p.parseUnary()
			target := p.checkSimpleTarget(x, "Invalid left-hand side expression in prefix operation")
			return &update{base{pos}, op, true, target.(expr)}
		}
	}
	x := p.parseLHS()
	if _, ok := x.(*arrowParams); ok {
		return x
	}
	if p.tok.kind == tPunct && (p.tok.text == "++" || p.tok.text == "--") && !p.tok.nlBefore {
		target := p.checkSimpleTarget(x, "Invalid left-hand side expression in postfix operation")
		op := p.tok.text
		p.next()
		return &update{base{pos}, op, false, target.(expr)}
	}
	return x
}

// foldUnary applies V8's parse-time folding of unary operators on literals.
func foldUnary(pos int, op string, x expr) expr {
	if op == "!" {
		switch l := x.(type) {
		case *numberLit:
			return &boolLit{base{pos}, l.value == 0 || math.IsNaN(l.value)}
		case *stringLit:
			return &boolLit{base{pos}, l.value == ""}
		case *boolLit:
			return &boolLit{base{pos}, !l.value}
		case *nullLit:
			return &boolLit{base{pos}, true}
		}
	}
	if n, ok := x.(*numberLit); ok {
		switch op {
		case "+":
			return n
		case "-":
			return &numberLit{base{pos}, -n.value}
		case "~":
			return &numberLit{base{pos}, float64(^js.ToInt32(n.value))}
		}
	}
	return &unary{base{pos}, op, x}
}

// parseLHS parses member accesses, calls, new and optional chains.
func (p *parser) parseLHS() expr {
	pos := p.tok.pos
	var x expr
	if p.is("new") {
		x = p.parseNew()
	} else {
		x = p.parsePrimary()
	}
	if _, ok := x.(*arrowParams); ok {
		return x
	}
	return p.parseCallTail(pos, x, true)
}

func (p *parser) parseNew() expr {
	pos := p.tok.pos
	p.next()
	if p.is(".") {
		p.fail("new.target is not supported")
	}
	var callee expr
	if p.is("new") {
		callee = p.parseNew()
	} else {
		callee = p.parsePrimary()
		if _, ok := callee.(*arrowParams); ok {
			p.unexpected(p.tok)
		}
	}
	callee = p.parseCallTail(pos, callee, false)
	var args []expr
	if p.is("(") {
		args = p.parseArguments()
	}
	return &newExpr{base{pos}, callee, args}
}

// parseCallTail parses the ".x", "[x]", "(args)", template and "?." suffixes. With
// allowCall false it stops at the first argument list (the callee of new).
func (p *parser) parseCallTail(pos int, x expr, allowCall bool) expr {
	inChain := false
	for {
		switch {
		case p.is("."):
			p.next()
			x = &member{base: base{pos}, obj: x, name: p.parseIdentifierName()}
		case p.is("?."):
			if !allowCall {
				p.fail("Invalid optional chain from new expression")
			}
			inChain = true
			p.next()
			switch {
			case p.is("("):
				x = &call{base{pos}, x, p.parseArguments(), true}
			case p.is("["):
				p.next()
				key := p.parseExpression(false)
				p.expect("]")
				x = &member{base: base{pos}, obj: x, key: key, optional: true}
			case p.tok.kind == tTemplate:
				p.fail("Invalid tagged template on optional chain")
			default:
				x = &member{base: base{pos}, obj: x, name: p.parseIdentifierName(), optional: true}
			}
		case p.is("["):
			p.next()
			key := p.parseExpression(false)
			p.expect("]")
			x = &member{base: base{pos}, obj: x, key: key}
		case p.is("(") && allowCall:
			x = &call{base{pos}, x, p.parseArguments(), false}
		case p.tok.kind == tTemplate:
			if inChain {
				p.fail("Invalid tagged template on optional chain")
			}
			x = &taggedTemplate{base{pos}, x, p.parseTemplate(true)}
		default:
			if inChain {
				return &optionalChain{base{pos}, x}
			}
			return x
		}
	}
}

func (p *parser) parseIdentifierName() string {
	if p.tok.kind == tPrivate {
		p.fail("Private field '" + p.tok.text + "' must be declared in an enclosing class")
	}
	if p.tok.kind != tIdent && p.tok.kind != tKeyword {
		p.unexpected(p.tok)
	}
	name := p.tok.text
	p.next()
	return name
}

func (p *parser) parseArguments() []expr {
	p.expect("(")
	var args []expr
	for !p.is(")") {
		if p.is("...") {
			pos := p.tok.pos
			p.next()
			args = append(args, &spread{base{pos}, p.parseAssignment(false)})
		} else {
			args = append(args, p.parseAssignment(false))
		}
		if p.is(")") {
			break
		}
		if !p.is(",") {
			// Any other token after an argument, even an illegal one.
			p.fail("missing ) after argument list")
		}
		p.next()
	}
	p.next()
	return args
}

func (p *parser) parsePrimary() expr {
	t := p.tok
	pos := t.pos
	switch t.kind {
	case tNumber:
		p.next()
		if t.bigint {
			return &bigintLit{base{pos}}
		}
		return &numberLit{base{pos}, t.num}
	case tPrivate:
		p.fail("Private field '" + t.text + "' must be declared in an enclosing class")
	case tString:
		p.next()
		return &stringLit{base{pos}, t.str}
	case tTemplate:
		return p.parseTemplate(false)
	case tIdent:
		p.next()
		return &ident{base{pos}, t.text}
	case tKeyword:
		if t.escaped {
			p.unexpected(t)
		}
		switch t.text {
		case "this":
			p.next()
			return &thisExpr{base{pos}}
		case "null":
			p.next()
			return &nullLit{base{pos}}
		case "true", "false":
			p.next()
			return &boolLit{base{pos}, t.text == "true"}
		case "function":
			p.next()
			if p.is("*") {
				p.fail("generator functions are not supported")
			}
			name := ""
			if p.tok.kind == tIdent {
				name = p.tok.text
				p.next()
			}
			fi := p.parseFunctionRest(pos, name, false)
			return &funcLit{base{pos}, fi}
		case "class":
			p.parseClassHead(false)
		case "super":
			p.fail("'super' keyword unexpected here")
		}
	case tPunct:
		switch t.text {
		case "(":
			return p.parseParenthesized()
		case "[", "{":
			return p.parseLiteral(false)
		case "/", "/=":
			rt := p.lx.scanRegExp(t.pos)
			if rt.kind == tIllegal {
				p.unexpected(rt)
			}
			if _, err := jsregexp.CompileLiteral(rt.text, rt.flags); err != nil {
				p.fail(err.(*js.Error).Message)
			}
			p.tok = rt
			p.next()
			return &regexpLit{base{pos}, rt.text, rt.flags}
		}
	}
	p.unexpected(t)
	return nil
}

func (p *parser) parseTemplate(tagged bool) *templateLit {
	lit := &templateLit{base: base{p.tok.pos}}
	for {
		t := p.tok
		if t.unterminated {
			p.fail("Unexpected end of input")
		}
		if !tagged && !t.cookedOK {
			p.fail(t.templErr)
		}
		lit.chunks = append(lit.chunks, templateChunk{t.str, t.cookedOK, t.raw})
		if t.tail {
			p.next()
			return lit
		}
		p.next()
		lit.exprs = append(lit.exprs, p.parseExpression(false))
		if !p.is("}") {
			p.unexpected(p.tok)
		}
		// Continue the template after the closing brace.
		p.lx.pos = p.tok.pos + 1
		start := p.tok.pos
		p.tok = p.lx.scanTemplate()
		p.tok.pos = start
		p.tok.end = p.lx.pos
	}
}

// parseParenthesized parses "( ... )": a parenthesized expression or the
// parameter list of an arrow function.
func (p *parser) parseParenthesized() expr {
	pos := p.tok.pos
	p.next()
	if p.is(")") {
		closeTok := p.tok
		p.next()
		if !p.is("=>") || p.tok.nlBefore {
			p.unexpected(closeTok)
		}
		return &arrowParams{base{pos}, nil, nil}
	}
	var items []expr
	var rest node
	for {
		if p.is("...") {
			rest = p.parseArrowRest()
			break
		}
		items = append(items, p.parseAssignmentCover(false, true))
		if !p.is(",") {
			break
		}
		p.next()
		if p.is(")") {
			closeTok := p.tok
			p.next()
			if !p.is("=>") || p.tok.nlBefore {
				p.unexpected(closeTok)
			}
			p.restoreClose(closeTok)
			break
		}
	}
	p.expect(")")
	if p.is("=>") && !p.tok.nlBefore {
		params := make([]*patternElem, len(items))
		for i, it := range items {
			params[i] = p.toParam(it)
		}
		return &arrowParams{base{pos}, params, rest}
	}
	var x expr
	if len(items) == 1 {
		x = items[0]
	} else {
		x = &sequence{base{items[0].position()}, items}
	}
	for _, it := range items {
		p.checkNoCoverInit(it)
	}
	p.markParens(x)
	return x
}

// restoreClose steps back to a closing parenthesis already consumed.
func (p *parser) restoreClose(closeTok token) {
	p.lx.pos = closeTok.end
	p.tok = closeTok
}

// parseArrowRest parses "...target" in a parenthesized list, which is only valid
// as the last parameter of an arrow function; V8 reports the ellipsis otherwise.
func (p *parser) parseArrowRest() node {
	ellipsis := p.tok
	p.next()
	var rest node
	if err := p.tryParse(func() { rest = p.parseBindingTarget() }); err != nil {
		// V8 goes on to report the ellipsis itself, and a report at an earlier
		// position replaces the pending one unless the two touch.
		if ellipsis.end < err.pos {
			p.unexpected(ellipsis)
		}
		panic(err)
	}
	if p.is("=") {
		p.fail("Rest parameter may not have a default initializer")
	}
	if p.is(",") {
		p.fail("Rest parameter must be last formal parameter")
	}
	if t := p.peek(); !p.is(")") || t.kind != tPunct || t.text != "=>" || t.nlBefore {
		p.unexpected(ellipsis)
	}
	return rest
}

// markParens records that a logical expression was parenthesized, which allows it
// to be mixed with ??.
func (p *parser) markParens(x expr) {
	switch t := x.(type) {
	case *ident:
		if p.parenIdents == nil {
			p.parenIdents = map[*ident]bool{}
		}
		p.parenIdents[t] = true
	case *logical:
		t.parens = true
	case *arrayLit:
		t.parens = true
	case *objectLit:
		t.parens = true
	}
}

func (p *parser) toParam(x expr) *patternElem {
	const bad = "Invalid destructuring assignment target"
	switch t := x.(type) {
	case *ident:
		return &patternElem{target: t}
	case *assign:
		if t.op == "=" {
			if _, isMember := t.target.(*member); !isMember {
				return &patternElem{target: t.target, def: t.value}
			}
		}
	case *arrayLit, *objectLit:
		pat := p.toPattern(t)
		if len(memberTargets(pat, nil)) > 0 {
			p.fail(bad)
		}
		return &patternElem{target: pat}
	}
	p.fail(bad)
	return nil
}

// memberTargets lists member-expression targets inside a pattern, which are not
// allowed in parameter lists.
func memberTargets(n node, out []node) []node {
	switch x := n.(type) {
	case *member:
		return append(out, x)
	case *arrayPattern:
		for _, e := range x.elems {
			if e != nil {
				out = memberTargets(e.target, out)
			}
		}
		if x.rest != nil {
			out = memberTargets(x.rest, out)
		}
	case *objectPattern:
		for _, pr := range x.props {
			out = memberTargets(pr.elem.target, out)
		}
		if x.rest != nil {
			out = memberTargets(x.rest, out)
		}
	}
	return out
}

// parseLiteral parses an array or object literal. In a declaration (binding)
// the literal is a pattern, and V8 rejects an invalid element as soon as it has
// been parsed, before any later syntax error.
func (p *parser) parseLiteral(binding bool) expr {
	if p.is("[") {
		return p.parseArrayLiteral(binding)
	}
	return p.parseObjectLiteral(binding)
}

// parseElement parses an element of an array or object literal; in a binding it
// is validated as a pattern element immediately.
func (p *parser) parseElement(binding bool) expr {
	if !binding {
		return p.parseAssignmentCover(false, true)
	}
	if p.is("[") || p.is("{") {
		lit := p.parseLiteral(true)
		if p.is("=") {
			pos := p.tok.pos
			p.next()
			return &assign{base{pos}, "=", p.toPattern(lit), p.parseAssignment(false)}
		}
		return lit
	}
	x := p.parseAssignmentCover(false, true)
	p.toPatternElem(x)
	return x
}

func (p *parser) parseArrayLiteral(binding bool) expr {
	lit := &arrayLit{base: base{p.tok.pos}}
	p.next()
	for !p.is("]") {
		if p.is(",") {
			p.next()
			lit.elems = append(lit.elems, nil)
			continue
		}
		if p.is("...") {
			pos := p.tok.pos
			p.next()
			lit.elems = append(lit.elems, &spread{base{pos}, p.parseElement(binding)})
		} else {
			lit.elems = append(lit.elems, p.parseElement(binding))
		}
		if !p.is("]") {
			p.expect(",")
		}
	}
	p.next()
	return lit
}

// parsePropertyName parses an object literal key: an identifier name, string,
// number or computed key.
func (p *parser) parsePropertyName() (string, expr) {
	t := p.tok
	switch t.kind {
	case tIdent, tKeyword:
		p.next()
		return t.text, nil
	case tString:
		p.next()
		return t.str, nil
	case tNumber:
		p.next()
		return js.NumberToString(t.num), nil
	case tPunct:
		if t.text == "[" {
			p.next()
			k := p.parseAssignment(false)
			p.expect("]")
			return "", k
		}
	}
	p.unexpected(t)
	return "", nil
}

func (p *parser) parseObjectLiteral(binding bool) expr {
	lit := &objectLit{base: base{p.tok.pos}}
	p.next()
	hasProto := false
	for !p.is("}") {
		pos := p.tok.pos
		if p.is("...") {
			p.next()
			lit.props = append(lit.props, &property{kind: propSpread, value: p.parseElement(binding), pos: pos})
		} else {
			prop := p.parseObjectProperty(binding)
			if prop.kind == propInit && prop.computed == nil && prop.key == "__proto__" {
				if hasProto {
					p.fail("Duplicate __proto__ fields are not allowed in object literals")
				}
				hasProto = true
				prop.kind = propProto
			}
			lit.props = append(lit.props, prop)
		}
		if !p.is("}") {
			p.expect(",")
		}
	}
	p.next()
	return lit
}

func (p *parser) parseObjectProperty(binding bool) *property {
	t := p.tok
	pos := t.pos
	if t.kind == tPunct && t.text == "*" {
		p.fail("generator methods are not supported")
	}
	if t.kind == tIdent && (t.text == "get" || t.text == "set" || t.text == "async") && !t.escaped {
		nt := p.peek()
		if !(nt.kind == tPunct && (nt.text == "," || nt.text == ":" || nt.text == "(" || nt.text == "}" || nt.text == "=")) {
			p.fail("getters, setters and async methods in object literals are not supported")
		}
	}
	key, computed := p.parsePropertyName()
	prop := &property{key: key, computed: computed, pos: pos}
	switch {
	case p.eat(":"):
		prop.kind = propInit
		prop.value = p.parseElement(binding)
	case p.is("(") && binding:
		p.fail("Invalid destructuring assignment target")
	case p.is("("):
		prop.kind = propMethod
		fi := p.parseFunctionRest(pos, key, true)
		prop.value = &funcLit{base{pos}, fi}
	case t.kind == tIdent && computed == nil && !t.escaped:
		prop.kind = propShorthand
		if p.is("=") {
			p.next()
			prop.value = &coverInit{base{pos}, key, p.parseAssignment(false)}
		} else {
			prop.value = &ident{base{pos}, key}
		}
	case computed == nil && t.kind != tIdent && (p.is(",") || p.is("}") || p.is("=")):
		// Only an identifier can be a shorthand property.
		p.unexpected(t)
	default:
		p.unexpected(p.tok)
	}
	return prop
}
