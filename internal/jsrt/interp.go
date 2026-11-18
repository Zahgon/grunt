package jsrt

import (
	"errors"
	"math"
	"slices"

	"github.com/gruntjs/grunt-go/js"
)

// Thrown is a JavaScript exception whose value is not an Error object, such as
// `throw 'boom'`. Errors themselves propagate as *js.Error.
type Thrown struct {
	Value any
}

// Error returns String(value), as an uncaught exception prints it.
func (t *Thrown) Error() string { return js.ToString(t.Value) }

// throwValue converts a JavaScript value being thrown into a Go error.
func throwValue(v any) error {
	if e, ok := v.(*js.Error); ok {
		return e
	}
	return &Thrown{Value: v}
}

// caughtValue converts a Go error into the value a catch clause binds. Errors that
// did not come from JavaScript become Error objects carrying the Go error.
func caughtValue(err error) any {
	var t *Thrown
	if errors.As(err, &t) {
		return t.Value
	}
	var e *js.Error
	if errors.As(err, &e) {
		return e
	}
	return &js.Error{Name: "Error", Message: err.Error(), Orig: err}
}

// binding is one declarative environment entry.
type binding struct {
	value       any
	mutable     bool
	initialized bool
}

// env is an environment record: declarative (vars) or, for a with statement or the
// global scope, backed by an object.
type env struct {
	vars   map[string]*binding
	obj    any
	parent *env
	// funcScope marks a function's variable environment; hasThis marks one that
	// binds this (arrow functions do not).
	funcScope bool
	hasThis   bool
	this      any
}

func newEnv(parent *env) *env {
	return &env{vars: map[string]*binding{}, parent: parent}
}

// closure is a function defined by JavaScript code.
type closure struct {
	fi  *funcInfo
	env *env
}

// ctrlKind is the kind of a statement completion.
type ctrlKind uint8

const (
	cNormal ctrlKind = iota
	cBreak
	cContinue
	cReturn
)

type ctrl struct {
	kind  ctrlKind
	label string
	value any
}

var normal = ctrl{}

// resolve finds the environment that binds name: a declarative binding or an
// object (with or global). Neither is set when name is unresolvable.
func (r *Realm) resolve(e *env, name string) (*binding, any) {
	for ; e != nil; e = e.parent {
		if e.obj != nil {
			if r.hasProperty(e.obj, name) {
				return nil, e.obj
			}
			continue
		}
		if b, ok := e.vars[name]; ok {
			return b, nil
		}
	}
	return nil, nil
}

func tdzError(name string) error {
	return js.NewReferenceError("Cannot access '" + name + "' before initialization")
}

func (r *Realm) getIdent(e *env, name string) (any, error) {
	b, obj := r.resolve(e, name)
	switch {
	case b != nil:
		if !b.initialized {
			return nil, tdzError(name)
		}
		return b.value, nil
	case obj != nil:
		v, _ := r.lookup(obj, name)
		return v, nil
	}
	return nil, js.NewReferenceError(name + " is not defined")
}

// setIdent assigns to an identifier; an unresolvable name creates a global
// property, as sloppy-mode code does.
func (r *Realm) setIdent(e *env, name string, v any) error {
	b, obj := r.resolve(e, name)
	switch {
	case b != nil:
		if !b.initialized {
			return tdzError(name)
		}
		if !b.mutable {
			return js.NewTypeError("Assignment to constant variable.")
		}
		b.value = v
		return nil
	case obj != nil:
		return r.put(obj, name, v)
	}
	return r.put(r.global, name, v)
}

// thisValue finds the this binding of the nearest non-arrow function; code outside
// any function sees the global object.
func thisValue(e *env, global any) any {
	for ; e != nil; e = e.parent {
		if e.hasThis {
			return e.this
		}
	}
	return global
}

// varScope returns the nearest function variable environment.
func varScope(e *env) *env {
	for ; e != nil; e = e.parent {
		if e.funcScope {
			return e
		}
	}
	return nil
}

// makeFunction creates the function object for fi closing over e.
func (r *Realm) makeFunction(fi *funcInfo, e *env, name string) *js.Function {
	c := &closure{fi: fi, env: e}
	f := &js.Function{Name: name, Length: fi.length}
	f.Call = func(this any, args []any) (any, error) {
		return r.callClosure(c, f, this, args)
	}
	r.closures[f] = c
	return f
}

// functionExpression evaluates a function or arrow literal. A named function
// expression sees its own name in an intermediate scope.
func (r *Realm) functionExpression(fi *funcInfo, e *env, inferred string) *js.Function {
	if fi.name != "" && !fi.arrow && !fi.method {
		fe := newEnv(e)
		f := r.makeFunction(fi, fe, fi.name)
		fe.vars[fi.name] = &binding{value: f, mutable: false, initialized: true}
		return f
	}
	name := fi.name
	if name == "" {
		name = inferred
	}
	return r.makeFunction(fi, e, name)
}

// callClosure runs a JavaScript function: it instantiates the function scope,
// binds this, arguments and parameters, hoists declarations and runs the body.
func (r *Realm) callClosure(c *closure, f *js.Function, this any, args []any) (any, error) {
	if r.depth >= r.maxDepth {
		return nil, js.NewRangeError("Maximum call stack size exceeded")
	}
	r.depth++
	defer func() { r.depth-- }()
	fi := c.fi
	fe := newEnv(c.env)
	fe.funcScope = true
	if !fi.arrow {
		fe.hasThis = true
		// Sloppy-mode functions see the global object for a null or undefined this.
		if js.IsNullish(this) {
			this = r.global
		}
		fe.this = this
		fe.vars["arguments"] = &binding{value: &arguments{items: append([]any(nil), args...), callee: f}, mutable: true, initialized: true}
	}
	for _, el := range fi.params {
		for _, n := range boundNames(el.target, nil) {
			fe.vars[n] = &binding{value: js.Undefined, mutable: true, initialized: true}
		}
	}
	if fi.rest != nil {
		for _, n := range boundNames(fi.rest, nil) {
			fe.vars[n] = &binding{value: js.Undefined, mutable: true, initialized: true}
		}
	}
	for i, el := range fi.params {
		var v any = js.Undefined
		if i < len(args) {
			v = args[i]
		}
		if _, isUndef := v.(js.UndefinedType); isUndef && el.def != nil {
			d, err := r.evalNamed(el.def, fe, targetName(el.target))
			if err != nil {
				return nil, err
			}
			v = d
		}
		if err := r.bindPattern(el.target, v, fe, bindParam, nil); err != nil {
			return nil, err
		}
	}
	if fi.rest != nil {
		var rest []any
		if len(args) > len(fi.params) {
			rest = append(rest, args[len(fi.params):]...)
		}
		if err := r.bindPattern(fi.rest, js.NewArray(rest...), fe, bindParam, nil); err != nil {
			return nil, err
		}
	}
	for _, n := range fi.varNames {
		if _, ok := fe.vars[n]; !ok {
			fe.vars[n] = &binding{value: js.Undefined, mutable: true, initialized: true}
		}
	}
	r.instantiateScope(fi.scope, fe)
	if fi.exprBody != nil {
		return r.eval(fi.exprBody, fe)
	}
	res, err := r.execList(fi.body, fe)
	if err != nil {
		return nil, err
	}
	if res.kind == cReturn {
		return res.value, nil
	}
	return js.Undefined, nil
}

// instantiateScope creates the lexical bindings (uninitialized, in their temporal
// dead zone) and the hoisted function declarations of a scope in e.
func (r *Realm) instantiateScope(s *scopeInfo, e *env) {
	if s == nil {
		return
	}
	for _, n := range s.lets {
		e.vars[n] = &binding{mutable: true}
	}
	for _, n := range s.consts {
		e.vars[n] = &binding{}
	}
	for _, fi := range s.funcs {
		e.vars[fi.name] = &binding{value: r.makeFunction(fi, e, fi.name), mutable: true, initialized: true}
	}
}

// blockEnv returns the environment for a block: e itself when the block declares
// nothing.
func (r *Realm) blockEnv(s *scopeInfo, e *env) *env {
	if s == nil || len(s.lets)+len(s.consts)+len(s.funcs) == 0 {
		return e
	}
	be := newEnv(e)
	r.instantiateScope(s, be)
	return be
}

// Statements.

func (r *Realm) execList(list []stmt, e *env) (ctrl, error) {
	for _, s := range list {
		c, err := r.exec(s, e)
		if err != nil || c.kind != cNormal {
			return c, err
		}
	}
	return normal, nil
}

func (r *Realm) exec(s stmt, e *env) (ctrl, error) {
	switch x := s.(type) {
	case *exprStmt:
		_, err := r.eval(x.x, e)
		return normal, err
	case *varDecl:
		return normal, r.execVarDecl(x, e)
	case *funcDecl:
		// Annex B.3.2.1: evaluating a block-level function declaration copies the
		// block's binding to the function-scoped var of the same name.
		if vs := varScope(e); vs != nil && vs != e {
			if b, ok := vs.vars[x.fn.name]; ok {
				if cur, ok := e.vars[x.fn.name]; ok {
					b.value = cur.value
				}
			}
		}
		return normal, nil
	case *block:
		return r.execList(x.body, r.blockEnv(x.scope, e))
	case *emptyStmt:
		return normal, nil
	case *ifStmt:
		v, err := r.eval(x.test, e)
		if err != nil {
			return normal, err
		}
		if js.ToBoolean(v) {
			return r.exec(x.cons, e)
		}
		if x.alt != nil {
			return r.exec(x.alt, e)
		}
		return normal, nil
	case *returnStmt:
		if x.x == nil {
			return ctrl{kind: cReturn, value: js.Undefined}, nil
		}
		v, err := r.eval(x.x, e)
		return ctrl{kind: cReturn, value: v}, err
	case *throwStmt:
		v, err := r.eval(x.x, e)
		if err != nil {
			return normal, err
		}
		return normal, throwValue(v)
	case *breakStmt:
		return ctrl{kind: cBreak, label: x.label}, nil
	case *continueStmt:
		return ctrl{kind: cContinue, label: x.label}, nil
	case *labeled:
		labels := []string{x.label}
		body := x.body
		for {
			inner, ok := body.(*labeled)
			if !ok {
				break
			}
			labels = append(labels, inner.label)
			body = inner.body
		}
		c, err := r.execLabeled(body, e, labels)
		if err == nil && c.kind == cBreak && slices.Contains(labels, c.label) {
			return normal, nil
		}
		return c, err
	case *tryStmt:
		return r.execTry(x, e)
	case *switchStmt:
		return r.execSwitch(x, e)
	case *withStmt:
		v, err := r.eval(x.obj, e)
		if err != nil {
			return normal, err
		}
		if js.IsNullish(v) {
			return normal, js.NewTypeError("Cannot convert undefined or null to object")
		}
		return r.exec(x.body, &env{obj: v, parent: e})
	}
	return r.execLabeled(s, e, nil)
}

// loopContinues interprets a loop body's completion: whether to keep iterating, and
// the completion to return when not.
func loopContinues(c ctrl, labels []string) (bool, ctrl) {
	switch c.kind {
	case cBreak:
		if c.label == "" || slices.Contains(labels, c.label) {
			return false, normal
		}
		return false, c
	case cContinue:
		if c.label == "" || slices.Contains(labels, c.label) {
			return true, normal
		}
		return false, c
	case cReturn:
		return false, c
	}
	return true, normal
}

// execLabeled runs a statement that may be the target of labelled continue.
func (r *Realm) execLabeled(s stmt, e *env, labels []string) (ctrl, error) {
	switch x := s.(type) {
	case *whileStmt:
		for {
			v, err := r.eval(x.test, e)
			if err != nil {
				return normal, err
			}
			if !js.ToBoolean(v) {
				return normal, nil
			}
			c, err := r.exec(x.body, e)
			if err != nil {
				return normal, err
			}
			if more, out := loopContinues(c, labels); !more {
				return out, nil
			}
		}
	case *doWhile:
		for {
			c, err := r.exec(x.body, e)
			if err != nil {
				return normal, err
			}
			if more, out := loopContinues(c, labels); !more {
				return out, nil
			}
			v, err := r.eval(x.test, e)
			if err != nil {
				return normal, err
			}
			if !js.ToBoolean(v) {
				return normal, nil
			}
		}
	case *forStmt:
		return r.execFor(x, e, labels)
	case *forInOf:
		return r.execForInOf(x, e, labels)
	}
	return r.exec(s, e)
}

func (r *Realm) execVarDecl(d *varDecl, e *env) error {
	for _, dc := range d.decls {
		if dc.init == nil {
			if d.kind == "var" {
				continue
			}
			// let x; initializes x to undefined.
			if err := r.bindPattern(dc.target, js.Undefined, e, bindLexical, nil); err != nil {
				return err
			}
			continue
		}
		v, err := r.evalNamed(dc.init, e, targetName(dc.target))
		if err != nil {
			return err
		}
		mode := bindLexical
		if d.kind == "var" {
			mode = bindVar
		}
		if err := r.bindPattern(dc.target, v, e, mode, dc.init); err != nil {
			return err
		}
	}
	return nil
}

// targetName is the name inferred for an anonymous function assigned to target.
func targetName(target node) string {
	if id, ok := target.(*ident); ok {
		return id.name
	}
	return ""
}

func (r *Realm) execFor(x *forStmt, e *env, labels []string) (ctrl, error) {
	le := r.blockEnv(x.scope, e)
	var perIteration []string
	switch init := x.init.(type) {
	case *varDecl:
		if err := r.execVarDecl(init, le); err != nil {
			return normal, err
		}
		if init.kind == "let" {
			for _, dc := range init.decls {
				perIteration = boundNames(dc.target, perIteration)
			}
		}
	case expr:
		if _, err := r.eval(init, le); err != nil {
			return normal, err
		}
	}
	// CreatePerIterationEnvironment: each iteration of a let loop gets fresh
	// bindings, so closures capture that iteration's values.
	copyEnv := func() {
		if len(perIteration) == 0 {
			return
		}
		ne := newEnv(le.parent)
		for k, b := range le.vars {
			nb := *b
			ne.vars[k] = &nb
		}
		le = ne
	}
	copyEnv()
	for {
		if x.test != nil {
			v, err := r.eval(x.test, le)
			if err != nil {
				return normal, err
			}
			if !js.ToBoolean(v) {
				return normal, nil
			}
		}
		c, err := r.exec(x.body, le)
		if err != nil {
			return normal, err
		}
		if more, out := loopContinues(c, labels); !more {
			return out, nil
		}
		copyEnv()
		if x.update != nil {
			if _, err := r.eval(x.update, le); err != nil {
				return normal, err
			}
		}
	}
}

func (r *Realm) execForInOf(x *forInOf, e *env, labels []string) (ctrl, error) {
	// The right-hand side is evaluated with the loop's lexical names in TDZ.
	te := r.blockEnv(x.scope, e)
	var rv any
	var err error
	if c, isCall := x.right.(*call); isCall && x.of {
		// V8 words a failing call in a for-of head as the iteration failing.
		rv, err = r.evalCallReporting(c, te, " is not a function or its return value is not iterable")
	} else {
		rv, err = r.eval(x.right, te)
	}
	if err != nil {
		return normal, err
	}
	var items []any
	var arr *js.Array
	if x.of {
		if a, ok := rv.(*js.Array); ok {
			// Array iteration reads the live array, so pushes during the loop are seen.
			arr = a
		} else {
			items, err = r.iterate(rv, func() error { return r.forOfNotIterable(x.right, rv) })
			if err != nil {
				return normal, err
			}
		}
	} else {
		if js.IsNullish(rv) {
			return normal, nil
		}
		for _, k := range r.forInKeys(rv) {
			items = append(items, k)
		}
	}
	for i := 0; ; i++ {
		var v any
		if arr != nil {
			if i >= len(arr.Items) {
				return normal, nil
			}
			v = arr.Items[i]
		} else {
			if i >= len(items) {
				return normal, nil
			}
			v = items[i]
			if !x.of && !r.hasProperty(rv, v.(string)) {
				// Keys deleted during enumeration are skipped.
				continue
			}
		}
		ie := e
		switch left := x.left.(type) {
		case *varDecl:
			target := left.decls[0].target
			if left.kind == "var" {
				err = r.bindPattern(target, v, e, bindVar, nil)
			} else {
				ie = newEnv(e)
				r.instantiateScope(x.scope, ie)
				err = r.bindPattern(target, v, ie, bindLexical, nil)
			}
		default:
			err = r.bindPattern(left, v, e, bindAssign, nil)
		}
		if err != nil {
			return normal, err
		}
		c, err := r.exec(x.body, ie)
		if err != nil {
			return normal, err
		}
		if more, out := loopContinues(c, labels); !more {
			return out, nil
		}
	}
}

func (r *Realm) execTry(x *tryStmt, e *env) (ctrl, error) {
	c, err := r.exec(x.body, e)
	if err != nil && x.handler != nil {
		ce := newEnv(e)
		r.instantiateScope(x.catchScope, ce)
		if x.param != nil {
			err = r.bindPattern(x.param, caughtValue(err), ce, bindLexical, nil)
		} else {
			err = nil
		}
		if err == nil {
			c, err = r.exec(x.handler, ce)
		}
	}
	if x.finalizer != nil {
		fc, ferr := r.exec(x.finalizer, e)
		if ferr != nil || fc.kind != cNormal {
			return fc, ferr
		}
	}
	return c, err
}

func (r *Realm) execSwitch(x *switchStmt, e *env) (ctrl, error) {
	d, err := r.eval(x.disc, e)
	if err != nil {
		return normal, err
	}
	se := r.blockEnv(x.scope, e)
	start := -1
	for i, c := range x.cases {
		if c.test == nil {
			continue
		}
		v, err := r.eval(c.test, se)
		if err != nil {
			return normal, err
		}
		if js.StrictEquals(d, v) {
			start = i
			break
		}
	}
	if start < 0 {
		for i, c := range x.cases {
			if c.test == nil {
				start = i
			}
		}
	}
	if start < 0 {
		return normal, nil
	}
	for _, c := range x.cases[start:] {
		res, err := r.execList(c.body, se)
		if err != nil {
			return normal, err
		}
		if res.kind == cBreak && res.label == "" {
			return normal, nil
		}
		if res.kind != cNormal {
			return res, nil
		}
	}
	return normal, nil
}

// Binding patterns.

type bindMode int

const (
	// bindVar assigns through the scope chain, as a var initializer does.
	bindVar bindMode = iota
	// bindLexical initializes a let/const binding of the given environment.
	bindLexical
	// bindParam initializes a parameter binding.
	bindParam
	// bindAssign is destructuring assignment: targets may be members.
	bindAssign
)

// nestedFlag marks a pattern nested inside another one, which V8 reports with
// different messages than a top-level pattern.
const nestedFlag bindMode = 1 << 4

func baseMode(mode bindMode) bindMode { return mode &^ nestedFlag }

// bindPattern binds or assigns v to target. src is the initializer expression of a
// declaration, used to render V8's destructuring error messages.
func (r *Realm) bindPattern(target node, v any, e *env, mode bindMode, src expr) error {
	switch t := target.(type) {
	case *ident:
		switch baseMode(mode) {
		case bindLexical, bindParam:
			for s := e; s != nil; s = s.parent {
				if b, ok := s.vars[t.name]; ok {
					b.value, b.initialized = v, true
					return nil
				}
			}
			e.vars[t.name] = &binding{value: v, mutable: true, initialized: true}
			return nil
		}
		return r.setIdent(e, t.name, v)
	case *member:
		obj, err := r.eval(t.obj, e)
		if err != nil {
			return err
		}
		key, err := r.memberKey(t, e)
		if err != nil {
			return err
		}
		return r.put(obj, key, v)
	case *invalidTarget:
		return r.failInvalidTarget(t, e)
	case *objectPattern:
		return r.bindObjectPattern(t, v, e, mode, src)
	case *arrayPattern:
		return r.bindArrayPattern(t, v, e, mode, src)
	}
	return js.NewSyntaxError("Invalid destructuring assignment target")
}

func (r *Realm) bindObjectPattern(t *objectPattern, v any, e *env, mode bindMode, src expr) error {
	if js.IsNullish(v) {
		if mode&nestedFlag != 0 {
			key := "undefined"
			if len(t.props) > 0 && t.props[0].computed == nil {
				key = t.props[0].key
			}
			return js.NewTypeError("Cannot read properties of " + js.ToString(v) + " (reading '" + key + "')")
		}
		what := js.ToString(v)
		switch {
		case src != nil:
			what = renderExpr(src)
		case baseMode(mode) == bindParam && v == nil:
			what = "object null"
		}
		if len(t.props) == 0 || t.props[0].computed != nil {
			return js.NewTypeError("Cannot destructure '" + what + "' as it is " + js.ToString(v) + ".")
		}
		return js.NewTypeError("Cannot destructure property '" + t.props[0].key + "' of '" + what + "' as it is " + js.ToString(v) + ".")
	}
	used := map[string]bool{}
	sub := mode | nestedFlag
	for _, p := range t.props {
		key := p.key
		if p.computed != nil {
			kv, err := r.eval(p.computed, e)
			if err != nil {
				return err
			}
			if key, err = r.toPropertyKey(kv); err != nil {
				return err
			}
		}
		used[key] = true
		val, _ := r.lookup(v, key)
		if err := r.bindElement(&p.elem, val, e, sub, src); err != nil {
			return err
		}
	}
	if t.rest != nil {
		rest := js.NewObject()
		for _, k := range r.ownKeys(v) {
			if !used[k] {
				val, _ := r.getOwn(v, k)
				rest.Set(k, val)
			}
		}
		return r.bindPattern(t.rest, rest, e, sub, nil)
	}
	return nil
}

func (r *Realm) bindElement(el *patternElem, v any, e *env, mode bindMode, src expr) error {
	if _, isUndef := v.(js.UndefinedType); isUndef && el.def != nil {
		d, err := r.evalNamed(el.def, e, targetName(el.target))
		if err != nil {
			return err
		}
		v = d
	}
	return r.bindPattern(el.target, v, e, mode, src)
}

func (r *Realm) bindArrayPattern(t *arrayPattern, v any, e *env, mode bindMode, src expr) error {
	items, err := r.iterate(v, func() error {
		const suffix = " is not iterable (cannot read property Symbol(Symbol.iterator))"
		// V8 prints a declaration's initializer from source where its printer finds
		// it, and describes the value otherwise.
		b := baseMode(mode)
		if src != nil && (b == bindVar || b == bindLexical) {
			if mode&nestedFlag != 0 {
				if _, isObj := src.(*objectLit); isObj {
					return js.NewTypeError(renderExpr(src) + suffix)
				}
				return js.NewTypeError(describeValue(v) + suffix)
			}
			switch src.(type) {
			case *nullLit:
				return js.NewTypeError("null is not iterable")
			case *ident:
				return js.NewTypeError(renderExpr(src) + " is not iterable")
			}
			if !js.IsNullish(v) {
				return js.NewTypeError(renderExpr(src) + " is not iterable")
			}
		}
		return js.NewTypeError(describeValue(v) + suffix)
	})
	if err != nil {
		return err
	}
	sub := mode | nestedFlag
	for i, el := range t.elems {
		if el == nil {
			continue
		}
		var val any = js.Undefined
		if i < len(items) {
			val = items[i]
		}
		if err := r.bindElement(el, val, e, sub, src); err != nil {
			return err
		}
	}
	if t.rest != nil {
		var rest []any
		if len(items) > len(t.elems) {
			rest = append(rest, items[len(t.elems):]...)
		}
		return r.bindPattern(t.rest, js.NewArray(rest...), e, sub, nil)
	}
	return nil
}

// failInvalidTarget evaluates a call used as an assignment target and throws the
// ReferenceError V8 defers to run time.
func (r *Realm) failInvalidTarget(t *invalidTarget, e *env) error {
	if _, err := r.evalCall(t.call, e); err != nil {
		return err
	}
	return js.NewReferenceError(t.msg)
}

// iterate collects the values the iteration protocol yields for v: array
// elements, string code points or arguments. Other values are not iterable and
// fail with the error notIterable builds.
func (r *Realm) iterate(v any, notIterable func() error) ([]any, error) {
	switch x := v.(type) {
	case *js.Array:
		return append([]any(nil), x.Items...), nil
	case string:
		return stringCodePoints(x), nil
	case *arguments:
		return append([]any(nil), x.items...), nil
	case *iterator:
		rest := x.items[x.pos:]
		x.pos = len(x.items)
		return append([]any(nil), rest...), nil
	case []byte:
		out := make([]any, len(x))
		for i, b := range x {
			out[i] = float64(b)
		}
		return out, nil
	}
	return nil, notIterable()
}

// notIterable builds "x is not iterable" for a spread source, or V8's variant for
// a call whose result is not iterable.
func (r *Realm) notIterable(src expr, v any) error {
	switch c := src.(type) {
	case *call:
		return js.NewTypeError(renderIterable(c.callee) + " is not a function or its return value is not iterable")
	case *newExpr:
		return js.NewTypeError(renderIterable(c.callee) + " is not a function or its return value is not iterable")
	}
	return js.NewTypeError(renderIterable(src) + " is not iterable")
}

// forOfNotIterable is notIterable for a for-of source. V8 finds no source text for
// && and || expressions, conditionals and optional chains there and describes the
// value instead.
func (r *Realm) forOfNotIterable(src expr, v any) error {
	switch x := src.(type) {
	case *logical:
		if x.op == "??" {
			break
		}
		return js.NewTypeError(describeValue(v) + " is not iterable (cannot read property Symbol(Symbol.iterator))")
	case *conditional, *optionalChain:
		return js.NewTypeError(describeValue(v) + " is not iterable (cannot read property Symbol(Symbol.iterator))")
	}
	return r.notIterable(src, v)
}

// Expressions.

// evalNamed evaluates x, naming an anonymous function or arrow literal name (the
// NamedEvaluation of an initializer).
func (r *Realm) evalNamed(x expr, e *env, name string) (any, error) {
	if f, ok := x.(*funcLit); ok && name != "" {
		return r.functionExpression(f.fn, e, name), nil
	}
	return r.eval(x, e)
}

func (r *Realm) eval(x expr, e *env) (any, error) {
	switch n := x.(type) {
	case *numberLit:
		return n.value, nil
	case *bigintLit:
		return nil, js.NewTypeError("BigInt values are not supported")
	case *stringLit:
		return n.value, nil
	case *boolLit:
		return n.value, nil
	case *nullLit:
		return nil, nil
	case *ident:
		return r.getIdent(e, n.name)
	case *thisExpr:
		return thisValue(e, r.global), nil
	case *templateLit:
		return r.evalTemplate(n, e)
	case *taggedTemplate:
		return r.evalTagged(n, e)
	case *regexpLit:
		return r.newRegExp(n.pattern, n.flags)
	case *arrayLit:
		return r.evalArray(n, e)
	case *objectLit:
		return r.evalObject(n, e)
	case *funcLit:
		return r.functionExpression(n.fn, e, ""), nil
	case *member, *call, *optionalChain:
		v, _, err := r.evalRef(x, e)
		return v, err
	case *newExpr:
		return r.evalNew(n, e)
	case *unary:
		return r.evalUnary(n, e)
	case *update:
		return r.evalUpdate(n, e)
	case *binary:
		return r.evalBinary(n, e)
	case *logical:
		l, err := r.eval(n.l, e)
		if err != nil {
			return nil, err
		}
		switch n.op {
		case "&&":
			if !js.ToBoolean(l) {
				return l, nil
			}
		case "||":
			if js.ToBoolean(l) {
				return l, nil
			}
		default:
			if !js.IsNullish(l) {
				return l, nil
			}
		}
		return r.eval(n.r, e)
	case *conditional:
		t, err := r.eval(n.test, e)
		if err != nil {
			return nil, err
		}
		if js.ToBoolean(t) {
			return r.eval(n.cons, e)
		}
		return r.eval(n.alt, e)
	case *assign:
		return r.evalAssign(n, e)
	case *sequence:
		var v any
		for _, it := range n.list {
			var err error
			if v, err = r.eval(it, e); err != nil {
				return nil, err
			}
		}
		return v, nil
	}
	return nil, js.NewSyntaxError("Unexpected token")
}

// errShortCircuit signals that an optional chain met null or undefined.
var errShortCircuit = errors.New("optional chain short circuit")

// evalRef evaluates member accesses, calls and optional chains, returning the
// value and the this value a call on it would use.
func (r *Realm) evalRef(x expr, e *env) (val, this any, err error) {
	switch n := x.(type) {
	case *optionalChain:
		v, t, err := r.evalRef(n.x, e)
		if err == errShortCircuit {
			return js.Undefined, js.Undefined, nil
		}
		// A parenthesized chain used as a callee keeps its this value.
		return v, t, err
	case *member:
		obj, _, err := r.evalRef(n.obj, e)
		if err != nil {
			return nil, nil, err
		}
		if n.optional && js.IsNullish(obj) {
			return nil, nil, errShortCircuit
		}
		v, err := r.getMember(n, obj, e)
		return v, obj, err
	case *call:
		v, err := r.evalCall(n, e)
		return v, js.Undefined, err
	case *ident:
		b, obj := r.resolve(e, n.name)
		if b == nil && obj != nil && obj != r.global {
			// A function found on a with object is called with that object as this.
			v, _ := r.lookup(obj, n.name)
			return v, obj, nil
		}
		v, err := r.eval(n, e)
		return v, js.Undefined, err
	}
	v, err := r.eval(x, e)
	return v, js.Undefined, err
}

// memberKey evaluates the property key of a member expression.
func (r *Realm) memberKey(m *member, e *env) (string, error) {
	if m.key == nil {
		return m.name, nil
	}
	k, err := r.eval(m.key, e)
	if err != nil {
		return "", err
	}
	return r.toPropertyKey(k)
}

func (r *Realm) getMember(m *member, obj any, e *env) (any, error) {
	if m.key != nil {
		k, err := r.eval(m.key, e)
		if err != nil {
			return nil, err
		}
		// Fast path: an integer index into an array.
		if a, ok := obj.(*js.Array); ok {
			if f, ok := k.(float64); ok && f >= 0 && f < float64(len(a.Items)) && f == math.Trunc(f) {
				return a.Items[int(f)], nil
			}
		}
		key, err := r.toPropertyKey(k)
		if err != nil {
			return nil, err
		}
		return r.getV(obj, key)
	}
	return r.getV(obj, m.name)
}

func (r *Realm) evalArgs(args []expr, e *env) ([]any, error) {
	out := make([]any, 0, len(args))
	for _, a := range args {
		if sp, ok := a.(*spread); ok {
			v, err := r.eval(sp.x, e)
			if err != nil {
				return nil, err
			}
			items, err := r.iterate(v, func() error {
				if js.IsNullish(v) {
					return js.NewTypeError(renderExpr(sp.x) + " is not iterable (cannot read property " + js.ToString(v) + ")")
				}
				return js.NewTypeError("Spread syntax requires ...iterable[Symbol.iterator] to be a function")
			})
			if err != nil {
				return nil, err
			}
			out = append(out, items...)
			continue
		}
		v, err := r.eval(a, e)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (r *Realm) evalCall(c *call, e *env) (any, error) {
	return r.evalCallReporting(c, e, " is not a function")
}

// evalCallReporting evaluates a call, reporting a non-callable callee as its
// rendering followed by notFunc.
func (r *Realm) evalCallReporting(c *call, e *env, notFunc string) (any, error) {
	fn, this, err := r.evalRef(c.callee, e)
	if err != nil {
		return nil, err
	}
	if c.optional && js.IsNullish(fn) {
		return nil, errShortCircuit
	}
	args, err := r.evalArgs(c.args, e)
	if err != nil {
		return nil, err
	}
	f, ok := fn.(*js.Function)
	if !ok {
		return nil, js.NewTypeError(renderExpr(c.callee) + notFunc)
	}
	return r.call(f, this, args)
}

// call invokes a function value.
func (r *Realm) call(f *js.Function, this any, args []any) (any, error) {
	return f.Call(this, args)
}

// callValue invokes v, which builtins received as a callback; a non-function is
// reported as V8 reports it without source text.
func (r *Realm) callValue(v, this any, args []any) (any, error) {
	f, ok := v.(*js.Function)
	if !ok {
		return nil, js.NewTypeError(describeValue(v) + " is not a function")
	}
	return r.call(f, this, args)
}

func (r *Realm) evalNew(n *newExpr, e *env) (any, error) {
	callee, err := r.eval(n.callee, e)
	if err != nil {
		return nil, err
	}
	args, err := r.evalArgs(n.args, e)
	if err != nil {
		return nil, err
	}
	f, ok := callee.(*js.Function)
	if !ok {
		return nil, js.NewTypeError(renderExpr(n.callee) + " is not a constructor")
	}
	return r.construct(f, args, renderExpr(n.callee))
}

// construct is [[Construct]]: builtin constructors build their own values; user
// functions get a fresh object inheriting from their prototype property.
func (r *Realm) construct(f *js.Function, args []any, shown string) (any, error) {
	if ctor, ok := r.ctors[f]; ok {
		return ctor(args)
	}
	if target, ok := r.boundTargets[f]; ok {
		bound := r.boundArgs[f]
		return r.construct(target, append(append([]any(nil), bound...), args...), shown)
	}
	c, ok := r.closures[f]
	if !ok || c.fi.arrow || c.fi.method {
		return nil, js.NewTypeError(shown + " is not a constructor")
	}
	proto, _ := r.lookup(f, "prototype")
	if !js.IsObject(proto) {
		proto = r.objectProto
	}
	obj := &Derived{Own: js.NewObject(), Proto: proto}
	res, err := r.callClosure(c, f, obj, args)
	if err != nil {
		return nil, err
	}
	if js.IsObject(res) {
		return res, nil
	}
	return obj, nil
}

func (r *Realm) evalTemplate(n *templateLit, e *env) (any, error) {
	s := n.chunks[0].cooked
	for i, x := range n.exprs {
		v, err := r.eval(x, e)
		if err != nil {
			return nil, err
		}
		str, err := r.toString(v)
		if err != nil {
			return nil, err
		}
		if s, err = js.Concat(s, str+n.chunks[i+1].cooked); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (r *Realm) evalTagged(n *taggedTemplate, e *env) (any, error) {
	fn, this, err := r.evalRef(n.tag, e)
	if err != nil {
		return nil, err
	}
	strs := js.NewArray()
	raw := js.NewArray()
	for _, c := range n.quasi.chunks {
		if c.cookedOK {
			strs.Items = append(strs.Items, c.cooked)
		} else {
			strs.Items = append(strs.Items, js.Undefined)
		}
		raw.Items = append(raw.Items, c.raw)
	}
	r.extraProps(strs, true).Set("raw", raw)
	args := []any{strs}
	for _, x := range n.quasi.exprs {
		v, err := r.eval(x, e)
		if err != nil {
			return nil, err
		}
		args = append(args, v)
	}
	f, ok := fn.(*js.Function)
	if !ok {
		return nil, js.NewTypeError(renderExpr(n.tag) + " is not a function")
	}
	return r.call(f, this, args)
}

func (r *Realm) evalArray(n *arrayLit, e *env) (any, error) {
	a := &js.Array{Items: make([]any, 0, len(n.elems))}
	for _, el := range n.elems {
		switch x := el.(type) {
		case nil:
			a.Items = append(a.Items, js.Undefined)
		case *spread:
			v, err := r.eval(x.x, e)
			if err != nil {
				return nil, err
			}
			items, err := r.iterate(v, func() error { return r.notIterable(x.x, v) })
			if err != nil {
				return nil, err
			}
			a.Items = append(a.Items, items...)
		default:
			v, err := r.eval(x, e)
			if err != nil {
				return nil, err
			}
			a.Items = append(a.Items, v)
		}
	}
	return a, nil
}

func (r *Realm) evalObject(n *objectLit, e *env) (any, error) {
	o := js.NewObject()
	var proto any = r.objectProto
	for _, p := range n.props {
		if p.kind == propProto {
			v, err := r.eval(p.value, e)
			if err != nil {
				return nil, err
			}
			// __proto__: sets the prototype when the value is an object or null.
			if js.IsObject(v) || v == nil {
				proto = v
			}
			continue
		}
		if p.kind == propSpread {
			v, err := r.eval(p.value, e)
			if err != nil {
				return nil, err
			}
			if !js.IsNullish(v) {
				for _, k := range r.ownKeys(v) {
					val, _ := r.getOwn(v, k)
					o.Set(k, val)
				}
			}
			continue
		}
		key := p.key
		if p.computed != nil {
			kv, err := r.eval(p.computed, e)
			if err != nil {
				return nil, err
			}
			if key, err = r.toPropertyKey(kv); err != nil {
				return nil, err
			}
		}
		var v any
		var err error
		if p.kind == propMethod {
			v = r.makeFunction(p.value.(*funcLit).fn, e, key)
		} else {
			v, err = r.evalNamed(p.value, e, key)
		}
		if err != nil {
			return nil, err
		}
		o.Set(key, v)
	}
	if proto != r.objectProto {
		return &Derived{Own: o, Proto: proto}, nil
	}
	return o, nil
}

func (r *Realm) evalUnary(n *unary, e *env) (any, error) {
	switch n.op {
	case "typeof":
		if id, ok := n.x.(*ident); ok {
			b, obj := r.resolve(e, id.name)
			if b == nil && obj == nil {
				return "undefined", nil
			}
		}
		v, err := r.eval(n.x, e)
		if err != nil {
			return nil, err
		}
		return js.TypeOf(v), nil
	case "delete":
		switch t := n.x.(type) {
		case *member:
			obj, err := r.eval(t.obj, e)
			if err != nil {
				return nil, err
			}
			key, err := r.memberKey(t, e)
			if err != nil {
				return nil, err
			}
			return r.deleteProp(obj, key)
		case *ident:
			b, obj := r.resolve(e, t.name)
			if b != nil {
				return false, nil
			}
			if obj != nil {
				return r.deleteProp(obj, t.name)
			}
			return true, nil
		case *optionalChain:
			m, ok := t.x.(*member)
			if !ok {
				break
			}
			obj, _, err := r.evalRef(m.obj, e)
			if err == errShortCircuit || err == nil && m.optional && js.IsNullish(obj) {
				return true, nil
			}
			if err != nil {
				return nil, err
			}
			key, err := r.memberKey(m, e)
			if err != nil {
				return nil, err
			}
			return r.deleteProp(obj, key)
		}
		if _, err := r.eval(n.x, e); err != nil {
			return nil, err
		}
		return true, nil
	}
	v, err := r.eval(n.x, e)
	if err != nil {
		return nil, err
	}
	switch n.op {
	case "!":
		return !js.ToBoolean(v), nil
	case "void":
		return js.Undefined, nil
	case "-":
		f, err := r.toNumber(v)
		return -f, err
	case "+":
		return r.toNumber(v)
	default: // "~"
		f, err := r.toNumber(v)
		return float64(^js.ToInt32(f)), err
	}
}

func (r *Realm) evalUpdate(n *update, e *env) (any, error) {
	delta := 1.0
	if n.op == "--" {
		delta = -1
	}
	switch t := n.x.(type) {
	case *ident:
		old, err := r.getIdent(e, t.name)
		if err != nil {
			return nil, err
		}
		f, err := r.toNumber(old)
		if err != nil {
			return nil, err
		}
		if err := r.setIdent(e, t.name, f+delta); err != nil {
			return nil, err
		}
		if n.prefix {
			return f + delta, nil
		}
		return f, nil
	case *member:
		obj, err := r.eval(t.obj, e)
		if err != nil {
			return nil, err
		}
		key, err := r.memberKey(t, e)
		if err != nil {
			return nil, err
		}
		old, err := r.getV(obj, key)
		if err != nil {
			return nil, err
		}
		f, err := r.toNumber(old)
		if err != nil {
			return nil, err
		}
		if err := r.put(obj, key, f+delta); err != nil {
			return nil, err
		}
		if n.prefix {
			return f + delta, nil
		}
		return f, nil
	case *invalidTarget:
		return nil, r.failInvalidTarget(t, e)
	}
	return nil, js.NewSyntaxError("Invalid left-hand side expression in prefix operation")
}

func (r *Realm) evalBinary(n *binary, e *env) (any, error) {
	l, err := r.eval(n.l, e)
	if err != nil {
		return nil, err
	}
	rv, err := r.eval(n.r, e)
	if err != nil {
		return nil, err
	}
	return r.binaryOp(n.op, l, rv)
}

// binaryOp applies a binary operator to evaluated operands.
func (r *Realm) binaryOp(op string, l, rv any) (any, error) {
	switch op {
	case "+":
		return r.add(l, rv)
	case "==":
		return r.looseEquals(l, rv)
	case "!=":
		eq, err := r.looseEquals(l, rv)
		return !eq, err
	case "===":
		return js.StrictEquals(l, rv), nil
	case "!==":
		return !js.StrictEquals(l, rv), nil
	case "<", ">", "<=", ">=":
		return r.relational(op, l, rv)
	case "in":
		if !js.IsObject(rv) {
			key, err := r.toPropertyKey(l)
			if err != nil {
				return nil, err
			}
			return nil, js.NewTypeError("Cannot use 'in' operator to search for '" + key + "' in " + js.ToString(rv))
		}
		key, err := r.toPropertyKey(l)
		if err != nil {
			return nil, err
		}
		return r.hasProperty(rv, key), nil
	case "instanceof":
		if !js.IsObject(rv) {
			return nil, js.NewTypeError("Right-hand side of 'instanceof' is not an object")
		}
		f, ok := rv.(*js.Function)
		if !ok {
			return nil, js.NewTypeError("Right-hand side of 'instanceof' is not callable")
		}
		if _, bound := r.boundTargets[f]; !bound {
			if proto, _ := r.lookup(f, "prototype"); !js.IsObject(proto) && js.IsObject(l) {
				return nil, js.NewTypeError("Function has non-object prototype '" + js.ToString(proto) + "' in instanceof check")
			}
		}
		return r.instanceOf(l, f), nil
	}
	return r.arith(op, l, rv)
}

func (r *Realm) evalAssign(n *assign, e *env) (any, error) {
	if n.op == "=" {
		switch t := n.target.(type) {
		case *ident:
			// The reference is resolved before the value is evaluated.
			v, err := r.evalNamed(n.value, e, t.name)
			if err != nil {
				return nil, err
			}
			return v, r.setIdent(e, t.name, v)
		case *member:
			obj, err := r.eval(t.obj, e)
			if err != nil {
				return nil, err
			}
			key, err := r.memberKey(t, e)
			if err != nil {
				return nil, err
			}
			v, err := r.eval(n.value, e)
			if err != nil {
				return nil, err
			}
			return v, r.put(obj, key, v)
		}
		if t, ok := n.target.(*invalidTarget); ok {
			return nil, r.failInvalidTarget(t, e)
		}
		v, err := r.eval(n.value, e)
		if err != nil {
			return nil, err
		}
		return v, r.bindPattern(n.target, v, e, bindAssign, nil)
	}
	// Compound and logical assignment read the target first.
	var get func() (any, error)
	var set func(any) error
	switch t := n.target.(type) {
	case *ident:
		get = func() (any, error) { return r.getIdent(e, t.name) }
		set = func(v any) error { return r.setIdent(e, t.name, v) }
	case *member:
		obj, err := r.eval(t.obj, e)
		if err != nil {
			return nil, err
		}
		key, err := r.memberKey(t, e)
		if err != nil {
			return nil, err
		}
		get = func() (any, error) { return r.getV(obj, key) }
		set = func(v any) error { return r.put(obj, key, v) }
	case *invalidTarget:
		return nil, r.failInvalidTarget(t, e)
	default:
		return nil, js.NewSyntaxError("Invalid left-hand side in assignment")
	}
	old, err := get()
	if err != nil {
		return nil, err
	}
	switch n.op {
	case "&&=", "||=", "??=":
		keep := false
		switch n.op {
		case "&&=":
			keep = !js.ToBoolean(old)
		case "||=":
			keep = js.ToBoolean(old)
		default:
			keep = !js.IsNullish(old)
		}
		if keep {
			return old, nil
		}
		v, err := r.evalNamed(n.value, e, targetName(n.target))
		if err != nil {
			return nil, err
		}
		return v, set(v)
	}
	rv, err := r.eval(n.value, e)
	if err != nil {
		return nil, err
	}
	v, err := r.binaryOp(n.op[:len(n.op)-1], old, rv)
	if err != nil {
		return nil, err
	}
	return v, set(v)
}
