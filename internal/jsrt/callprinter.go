package jsrt

import (
	"strings"

	"github.com/gruntjs/grunt-go/js"
)

// intermediate is what V8's CallPrinter prints for a sub-expression it does not
// render.
const intermediate = "(intermediate value)"

// renderExpr reproduces V8's CallPrinter, which re-prints the source expression of
// a failing call, construction or iteration in error messages such as
// "a.b is not a function". It prints the folded AST, not the source text.
func renderExpr(e expr) string {
	p := &callPrinter{}
	p.expr(e, true)
	return p.b.String()
}

// renderIterable prints the source of a value that is not iterable. V8 prints it
// in iterator-error mode, where calls inside it show no "(...)".
func renderIterable(e expr) string {
	p := &callPrinter{iterator: true}
	p.expr(e, true)
	return p.b.String()
}

// callPrinter accumulates the rendering of an expression.
type callPrinter struct {
	b        strings.Builder
	iterator bool
}

// expr appends e. With print false (V8's Find without printing) every node is
// rendered as "(intermediate value)"; nodes that print nothing fall back to it too.
func (p *callPrinter) expr(e expr, print bool) {
	b := &p.b
	if !print {
		b.WriteString(intermediate)
		return
	}
	switch x := e.(type) {
	case *ident:
		b.WriteString(x.name)
	case *thisExpr:
		b.WriteString("this")
	case *numberLit:
		b.WriteString(js.NumberToString(x.value))
	case *stringLit:
		b.WriteString(`"` + x.value + `"`)
	case *boolLit:
		if x.value {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case *nullLit:
		b.WriteString("null")
	case *regexpLit:
		b.WriteString("/" + x.pattern + "/" + canonicalFlags(x.flags))
	case *templateLit:
		if len(x.exprs) == 0 {
			b.WriteString(`"` + x.chunks[0].cooked + `"`)
			return
		}
		// V8 desugars a template into concatenations; the printer shows only the
		// substitutions.
		for _, sub := range x.exprs {
			p.expr(sub, true)
		}
	case *member:
		p.expr(x.obj, true)
		key, isName := memberKeyName(x)
		if isName {
			if x.optional {
				b.WriteString("?")
			}
			b.WriteString("." + key)
			return
		}
		if x.optional {
			b.WriteString("?.")
		}
		b.WriteString("[")
		p.expr(x.key, true)
		b.WriteString("]")
	case *call:
		p.expr(x.callee, true)
		if !p.iterator {
			b.WriteString("(...)")
		}
	case *arrayLit:
		b.WriteString("[")
		for i, el := range x.elems {
			if i > 0 {
				b.WriteString(",")
			}
			if el == nil {
				b.WriteString(intermediate)
				continue
			}
			p.expr(el, true)
		}
		b.WriteString("]")
	case *objectLit:
		b.WriteString("{")
		for range x.props {
			b.WriteString(intermediate)
		}
		b.WriteString("}")
	case *spread:
		b.WriteString("(...")
		p.expr(x.x, true)
		b.WriteString(")")
	case *unary:
		b.WriteString("(" + x.op)
		if x.op == "typeof" || x.op == "void" || x.op == "delete" {
			b.WriteString(" ")
		}
		p.expr(x.x, true)
		b.WriteString(")")
	case *update:
		b.WriteString("(")
		if x.prefix {
			b.WriteString(x.op)
		}
		p.expr(x.x, true)
		if !x.prefix {
			b.WriteString(x.op)
		}
		b.WriteString(")")
	case *binary:
		b.WriteString("(")
		p.nary(x.op, x.l)
		b.WriteString(" " + x.op + " ")
		p.expr(x.r, true)
		b.WriteString(")")
	case *logical:
		b.WriteString("(")
		p.nary(x.op, x.l)
		b.WriteString(" " + x.op + " ")
		p.expr(x.r, true)
		b.WriteString(")")
	case *sequence:
		b.WriteString("(")
		for i, it := range x.list {
			if i > 0 {
				b.WriteString(" , ")
			}
			p.expr(it, true)
		}
		b.WriteString(")")
	case *conditional:
		b.WriteString(intermediate + intermediate + intermediate)
	case *assign:
		if t, ok := x.target.(expr); ok {
			p.expr(t, true)
			return
		}
		b.WriteString(intermediate)
	default:
		// Function literals, new expressions, optional chains in parentheses and
		// tagged templates print nothing of their own.
		b.WriteString(intermediate)
	}
}

// nary flattens a left operand built with the same operator, as V8's
// NaryOperation prints "a + b + c" without inner parentheses.
func (p *callPrinter) nary(op string, l expr) {
	b := &p.b
	switch x := l.(type) {
	case *binary:
		if x.op == op {
			p.nary(op, x.l)
			b.WriteString(" " + op + " ")
			p.expr(x.r, true)
			return
		}
	case *logical:
		if x.op == op && !x.parens {
			p.nary(op, x.l)
			b.WriteString(" " + op + " ")
			p.expr(x.r, true)
			return
		}
	}
	p.expr(l, true)
}

// memberKeyName returns the property name of a member expression whose key is a
// string literal (dot access, a['x'] or a[`x`]), which V8 prints after a dot.
func memberKeyName(m *member) (string, bool) {
	if m.key == nil {
		return m.name, true
	}
	switch k := m.key.(type) {
	case *stringLit:
		return k.value, true
	case *templateLit:
		if len(k.exprs) == 0 {
			return k.chunks[0].cooked, true
		}
	}
	return "", false
}

// canonicalFlags orders regular expression flags as V8 prints them.
func canonicalFlags(flags string) string {
	var b strings.Builder
	for _, c := range "dgimsuvy" {
		if strings.ContainsRune(flags, c) {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// describeValue renders a value as V8 does when a builtin reports it without
// source text, e.g. `string "x" is not a function` or "object null".
func describeValue(v any) string {
	switch x := v.(type) {
	case js.UndefinedType:
		return "undefined"
	case nil:
		return "object null"
	case string:
		return `string "` + x + `"`
	case float64:
		return "number " + js.NumberToString(x)
	case bool:
		return "boolean " + js.ToString(x)
	case *js.Function:
		return "function"
	}
	return "object"
}
