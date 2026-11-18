package jsrt

import (
	"math"

	"github.com/gruntjs/grunt-go/js"
)

// DefaultMaxCallDepth is the default bound on nested JavaScript function calls: the
// depth at which Node 24 overflows its stack for a simple recursive function.
const DefaultMaxCallDepth = 10450

// Options configures a Realm.
type Options struct {
	// Now returns the current time in milliseconds since the epoch, for Date.now()
	// and new Date(). When nil, both throw an Error, so output never depends on
	// the wall clock by accident.
	Now func() float64
	// MaxCallDepth bounds nested calls of JavaScript functions; deeper calls throw
	// RangeError "Maximum call stack size exceeded". Zero means DefaultMaxCallDepth.
	MaxCallDepth int
}

// Realm is a JavaScript global environment: the global object, the builtin
// constructors and prototypes, and the per-object state the Go value model has no
// room for. Templates compiled in the same realm share globals, as templates in one
// Node process do (sloppy-mode code that assigns an undeclared name creates a
// global). A Realm is not safe for concurrent use.
type Realm struct {
	global *js.Object

	objectProto, functionProto, arrayProto, stringProto *js.Object
	numberProto, booleanProto, dateProto, regexpProto   *js.Object
	iteratorProto                                       *js.Object
	errorProtos                                         map[string]*js.Object

	// protoLinks gives the prototype of builtin prototype objects that do not
	// inherit directly from Object.prototype.
	protoLinks map[*js.Object]any
	// nonEnum lists the non-enumerable keys of plain objects (builtins).
	nonEnum map[*js.Object]map[string]bool
	// frozen lists objects passed to Object.freeze; readOnly lists single
	// non-writable properties such as Math.PI.
	frozen   map[*js.Object]bool
	readOnly map[readOnlyKey]bool
	// extra holds properties of arrays, functions and regular expressions beyond
	// those their Go types store.
	extra map[any]*js.Object
	// closures maps functions defined in JavaScript to their code and scope.
	closures map[*js.Function]*closure
	// ctors holds the [[Construct]] behaviour of builtin constructors.
	ctors map[*js.Function]func(args []any) (any, error)
	// boundTargets and boundArgs describe functions made by bind.
	boundTargets map[*js.Function]*js.Function
	boundArgs    map[*js.Function][]any
	regexps      map[*js.RegExp]*regexpState

	// joining holds the arrays Array.prototype.join is currently joining.
	joining []*js.Array
	// templateCounter numbers compiled templates (lodash's templateCounter).
	templateCounter int

	now      func() float64
	depth    int
	maxDepth int
}

// NewRealm returns a realm with the ECMAScript builtins the template subset
// supports installed on its global object.
func NewRealm(opts Options) *Realm {
	r := &Realm{
		global:       js.NewObject(),
		errorProtos:  map[string]*js.Object{},
		protoLinks:   map[*js.Object]any{},
		nonEnum:      map[*js.Object]map[string]bool{},
		frozen:       map[*js.Object]bool{},
		readOnly:     map[readOnlyKey]bool{},
		extra:        map[any]*js.Object{},
		closures:     map[*js.Function]*closure{},
		ctors:        map[*js.Function]func([]any) (any, error){},
		boundTargets: map[*js.Function]*js.Function{},
		boundArgs:    map[*js.Function][]any{},
		regexps:      map[*js.RegExp]*regexpState{},
		now:          opts.Now,
		maxDepth:     opts.MaxCallDepth,
	}
	if r.maxDepth <= 0 {
		r.maxDepth = DefaultMaxCallDepth
	}
	r.objectProto = js.NewObject()
	r.functionProto = js.NewObject()
	r.arrayProto = js.NewObject()
	r.stringProto = js.NewObject()
	r.numberProto = js.NewObject()
	r.booleanProto = js.NewObject()
	r.dateProto = js.NewObject()
	r.regexpProto = js.NewObject()
	r.iteratorProto = js.NewObject()
	r.setupGlobals()
	r.setupObject()
	r.setupFunction()
	r.setupArray()
	r.setupString()
	r.setupNumber()
	r.setupBoolean()
	r.setupMath()
	r.setupJSON()
	r.setupDate()
	r.setupRegExp()
	r.setupErrors()
	r.setupIterator()
	return r
}

// Global returns the global object. Hosts may add globals (for example a process
// object) before running templates.
func (r *Realm) Global() *js.Object { return r.global }

// nativeFunc is the Go signature of a builtin function.
type nativeFunc func(this any, args []any) (any, error)

// newNative returns a builtin function object.
func newNative(name string, length int, fn nativeFunc) *js.Function {
	return &js.Function{Name: name, Length: length, Call: fn}
}

// define installs a non-enumerable property, as builtins are, on an object or on
// a constructor function.
func (r *Realm) define(target any, name string, v any) {
	o, ok := target.(*js.Object)
	if !ok {
		o = r.extraProps(target, true)
	}
	o.Set(name, v)
	r.markNonEnum(o, name)
}

// method installs a builtin method on an object or a constructor.
func (r *Realm) method(o any, name string, length int, fn nativeFunc) *js.Function {
	f := newNative(name, length, fn)
	r.define(o, name, f)
	return f
}

// constructor creates a builtin constructor with its prototype object, installs it
// as a global, and registers its [[Construct]] behaviour (nil: not constructible).
func (r *Realm) constructor(name string, length int, proto *js.Object, call nativeFunc, construct func([]any) (any, error)) *js.Function {
	f := newNative(name, length, call)
	if proto != nil {
		r.extraProps(f, true).Set("prototype", proto)
		r.define(proto, "constructor", f)
	}
	if construct != nil {
		r.ctors[f] = construct
	}
	r.define(r.global, name, f)
	return f
}

// staticObject creates a namespace object such as Math or JSON on the global.
func (r *Realm) staticObject(name string) *js.Object {
	o := js.NewObject()
	r.define(r.global, name, o)
	return o
}

// isUndefined reports whether v is undefined.
func isUndefined(v any) bool {
	_, ok := v.(js.UndefinedType)
	return ok
}

func (r *Realm) setupGlobals() {
	g := r.global
	r.define(g, "globalThis", g)
	r.define(g, "undefined", js.Undefined)
	r.define(g, "NaN", math.NaN())
	r.define(g, "Infinity", math.Inf(1))
	r.frozenValues(g, "undefined", "NaN", "Infinity")
	r.setupGlobalFunctions()
}

// frozenValues records keys that sloppy-mode assignment cannot change.
func (r *Realm) frozenValues(target any, keys ...string) {
	o, ok := target.(*js.Object)
	if !ok {
		o = r.extraProps(target, true)
	}
	for _, k := range keys {
		r.readOnly[readOnlyKey{o, k}] = true
	}
}

// readOnlyKey identifies a non-writable data property.
type readOnlyKey struct {
	obj *js.Object
	key string
}
