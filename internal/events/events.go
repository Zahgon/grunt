// Package events ports EventEmitter2 0.4.14 as grunt configures it for grunt.event:
// new EventEmitter2({wildcard: true}). Event names are split on the delimiter (".")
// into a listener tree; "*" matches one segment and "**" any number of segments, on
// either the listening or the emitting side.
//
// Listeners are *js.Function values, compared by identity. They are called with the
// emitter as this; the event being emitted is available from Emitter.Event, and
// Listener wraps a Go function so that it receives the event as a parameter.
//
// Divergences, for event names grunt never uses: a segment named "_listeners" is an
// ordinary segment here (in the original it collides with the tree's bookkeeping
// property), and segments such as "constructor" or "__proto__" do not reach
// Object.prototype.
package events

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gruntjs/grunt-go/js"
)

// defaultMaxListeners is the listener count per event above which a warning is printed.
const defaultMaxListeners = 10

// Options is the subset of EventEmitter2's configuration that applies with
// wildcard: true. Zero fields keep the defaults, as falsy values do in the original.
type Options struct {
	// Delimiter separates event name segments; "" means ".".
	Delimiter string
	// MaxListeners is the per-event limit above which a leak warning is printed;
	// 0 means the default of 10.
	MaxListeners int
	// NewListener makes On emit a "newListener" event before adding a listener.
	// Without it, "newListener" is never delivered, even to its own listeners.
	NewListener bool
	// Stderr receives the leak warning (console.error); nil means os.Stderr.
	Stderr io.Writer
}

// node is one level of the listener tree.
type node struct {
	children *js.Object // segment -> *node, in JavaScript key order
	// listeners is the _listeners property: none, a single function, or an array
	// (which stays an array once it was one, until it is emptied).
	listeners []*js.Function
	isArray   bool
	warned    bool
}

func newNode() *node { return &node{children: js.NewObject()} }

func (n *node) child(name string) *node {
	if c, ok := n.children.Get(name).(*node); ok {
		return c
	}
	return nil
}

func (n *node) hasListeners() bool { return len(n.listeners) > 0 }

func (n *node) clearListeners() {
	n.listeners, n.isArray, n.warned = nil, false, false
}

// Emitter is an EventEmitter2 with wildcards enabled.
type Emitter struct {
	conf         Options
	delimiter    string
	newListener  bool
	maxListeners int
	maxSet       bool // _events.maxListeners !== undefined
	tree         *node
	all          []*js.Function
	allSet       bool // this._all is defined (even when empty)
	event        string
	// origins maps each Many/Once wrapper to the function it wraps (the wrapper's
	// _origin property in the original).
	origins map[*js.Function]*js.Function
}

// New returns an emitter configured like new EventEmitter2({wildcard: true, ...opts}).
func New(opts Options) *Emitter {
	e := &Emitter{conf: opts, origins: map[*js.Function]*js.Function{}}
	e.configure()
	return e
}

// configure is EventEmitter2's configure(this._conf) on fresh _events.
func (e *Emitter) configure() {
	e.delimiter = "."
	if e.conf.Delimiter != "" {
		e.delimiter = e.conf.Delimiter
	}
	e.maxListeners, e.maxSet = 0, false
	if e.conf.MaxListeners != 0 {
		e.maxListeners, e.maxSet = e.conf.MaxListeners, true
	}
	e.newListener = e.conf.NewListener
	e.tree = newNode()
}

// Event returns the name of the event most recently delivered (this.event).
func (e *Emitter) Event() string { return e.event }

// Listener wraps a Go function as a listener; it receives the emitted event name (the
// emitter's this.event at call time) and the arguments.
func Listener(name string, fn func(event string, args []any) error) *js.Function {
	return &js.Function{Name: name, Call: func(this any, args []any) (any, error) {
		event := ""
		if e, ok := this.(*Emitter); ok {
			event = e.event
		}
		return js.Undefined, fn(event, args)
	}}
}

// SetMaxListeners sets the per-event listener limit; 0 means unlimited. The value
// survives RemoveAllListeners() only when it is not 0, as in the original.
func (e *Emitter) SetMaxListeners(n int) {
	e.maxListeners, e.maxSet = n, true
	e.conf.MaxListeners = n
}

func (e *Emitter) split(event string) []string {
	return strings.Split(event, e.delimiter)
}

// On adds a listener for event (which may contain "*" and "**" segments). It
// returns an error when fn is nil, when a newListener listener fails, or when the
// leak warning cannot be written; in the last case the listener has been added.
func (e *Emitter) On(event string, fn *js.Function) error {
	if fn == nil {
		return js.NewError("on only accepts instances of Function")
	}
	// Emit newListener before adding, so a "newListener" listener does not see itself.
	if _, err := e.Emit("newListener", event, fn); err != nil {
		return err
	}
	return e.grow(e.split(event), fn)
}

// AddListener is On.
func (e *Emitter) AddListener(event string, fn *js.Function) error { return e.On(event, fn) }

// grow is growListenerTree.
func (e *Emitter) grow(segments []string, fn *js.Function) error {
	// Two consecutive "**" segments are not added at all.
	for i := 0; i+1 < len(segments); i++ {
		if segments[i] == "**" && segments[i+1] == "**" {
			return nil
		}
	}
	tree := e.tree
	// The walk stops at the first empty segment: "a..b" or "" add nothing.
	for i, name := range segments {
		if name == "" {
			return nil
		}
		next := tree.child(name)
		if next == nil {
			next = newNode()
			tree.children.Set(name, next)
		}
		tree = next
		if i < len(segments)-1 {
			continue
		}
		switch {
		case !tree.hasListeners():
			tree.listeners, tree.isArray = []*js.Function{fn}, false
		case !tree.isArray:
			tree.listeners, tree.isArray = append(tree.listeners, fn), true
		default:
			tree.listeners = append(tree.listeners, fn)
			m := defaultMaxListeners
			if e.maxSet {
				m = e.maxListeners
			}
			if !tree.warned && m > 0 && len(tree.listeners) > m {
				tree.warned = true
				return e.warnLeak(len(tree.listeners))
			}
		}
		return nil
	}
	return nil
}

// warnLeak prints the leak warning with console.error, followed by console.trace's
// header. The stack frames console.trace prints after "Trace" are JavaScript frames
// and are not reproduced.
func (e *Emitter) warnLeak(count int) error {
	w := e.conf.Stderr
	if w == nil {
		w = os.Stderr
	}
	_, err := fmt.Fprintf(w, "(node) warning: possible EventEmitter memory leak detected. %d listeners added. "+
		"Use emitter.setMaxListeners() to increase limit.\nTrace\n", count)
	return err
}

// Once adds a listener that is removed before its first call.
func (e *Emitter) Once(event string, fn *js.Function) error { return e.Many(event, 1, fn) }

// Many adds a listener that is removed before its ttl-th call. As in the original, a
// ttl of 0 or less never reaches zero, so the listener stays forever. Off with fn
// removes the wrapper.
func (e *Emitter) Many(event string, ttl int, fn *js.Function) error {
	if fn == nil {
		return js.NewError("many only accepts instances of Function")
	}
	w := &js.Function{Name: "listener"}
	e.origins[w] = fn
	w.Call = func(this any, args []any) (any, error) {
		ttl--
		if ttl == 0 {
			if err := e.Off(event, w); err != nil {
				return nil, err
			}
		}
		return fn.Call(this, args)
	}
	return e.On(event, w)
}

func (e *Emitter) matches(h, fn *js.Function) bool {
	return h == fn || e.origins[h] == fn
}

// drop forgets the wrapper origins of listeners leaving the tree.
func (e *Emitter) drop(listeners ...*js.Function) {
	for _, l := range listeners {
		delete(e.origins, l)
	}
}

// Off removes fn (or a Many/Once wrapper of fn) from the listeners event matches.
// Like the original, it removes from every matching single-listener leaf but stops
// after the first array leaf it removes from.
func (e *Emitter) Off(event string, fn *js.Function) error {
	if fn == nil {
		return js.NewError("removeListener only takes instances of Function")
	}
	for _, leaf := range e.search(nil, e.split(event), e.tree, 0) {
		if !leaf.isArray {
			if leaf.hasListeners() && e.matches(leaf.listeners[0], fn) {
				e.drop(leaf.listeners...)
				leaf.clearListeners()
			}
			continue
		}
		pos := -1
		for i, h := range leaf.listeners {
			if e.matches(h, fn) {
				pos = i
				break
			}
		}
		if pos < 0 {
			continue
		}
		e.drop(leaf.listeners[pos])
		leaf.listeners = append(leaf.listeners[:pos:pos], leaf.listeners[pos+1:]...)
		if len(leaf.listeners) == 0 {
			leaf.clearListeners()
		}
		return nil
	}
	return nil
}

// RemoveListener is Off.
func (e *Emitter) RemoveListener(event string, fn *js.Function) error { return e.Off(event, fn) }

// OnAny adds a listener for every event.
func (e *Emitter) OnAny(fn *js.Function) error {
	if fn == nil {
		return js.NewError("onAny only accepts instances of Function")
	}
	e.all = append(e.all, fn)
	e.allSet = true
	return nil
}

// OffAny removes fn from the catch-all listeners; with a nil fn, or when there are
// none, it clears them. Either way the catch-all list then exists, and an emitter
// with a catch-all list, even an empty one, reports every Emit as handled and never
// throws for "error" — a quirk of the original kept here.
func (e *Emitter) OffAny(fn *js.Function) {
	if fn != nil && len(e.all) > 0 {
		for i, f := range e.all {
			if f == fn {
				e.all = append(e.all[:i:i], e.all[i+1:]...)
				return
			}
		}
		return
	}
	e.all, e.allSet = []*js.Function{}, true
}

// ListenersAny returns the catch-all listeners.
func (e *Emitter) ListenersAny() []*js.Function {
	return append([]*js.Function(nil), e.all...)
}

// RemoveAllListeners with no event resets the listener tree (keeping catch-all
// listeners, and the max-listeners setting unless it is 0); with an event it empties
// the listener lists the event matches.
func (e *Emitter) RemoveAllListeners(event ...string) {
	if len(event) == 0 {
		e.configure()
		e.origins = map[*js.Function]*js.Function{}
		return
	}
	for _, leaf := range e.search(nil, e.split(event[0]), e.tree, 0) {
		e.drop(leaf.listeners...)
		leaf.clearListeners()
	}
}

// Listeners returns the listeners event would call, in call order.
func (e *Emitter) Listeners(event string) []*js.Function {
	var handlers []*js.Function
	e.search(&handlers, e.split(event), e.tree, 0)
	return handlers
}

// Emit calls the catch-all listeners and then the listeners matching event with
// args, and reports whether any listener ran (or a catch-all list exists). A
// listener's error stops the emit and is returned. Emitting "error" with no
// listeners at all returns args[0] when it is an error, or the error
// "Uncaught, unspecified 'error' event.", as the original throws them.
func (e *Emitter) Emit(event string, args ...any) (bool, error) {
	if event == "newListener" && !e.newListener {
		return false, nil
	}
	if e.allSet {
		for i, n := 0, len(e.all); i < n; i++ {
			if i >= len(e.all) {
				// A listener removed catch-all listeners mid-loop: this._all[i] is undefined.
				return false, js.NewTypeError("Cannot read properties of undefined (reading 'apply')")
			}
			e.event = event
			if _, err := e.all[i].Call(e, args); err != nil {
				return false, err
			}
		}
	}
	if event == "error" && !e.allSet && e.tree.child("error") == nil {
		if len(args) > 0 {
			if err, ok := args[0].(error); ok {
				return false, err
			}
		}
		return false, js.NewError("Uncaught, unspecified 'error' event.")
	}
	var handlers []*js.Function
	e.search(&handlers, e.split(event), e.tree, 0)
	for _, h := range handlers {
		e.event = event
		if _, err := h.Call(e, args); err != nil {
			return false, err
		}
	}
	return len(handlers) > 0 || e.allSet, nil
}

// segment returns typ[i]; ok is false past the end, where JavaScript reads undefined
// (which as a property key is the string "undefined").
func segment(typ []string, i int) (string, bool) {
	if i < len(typ) {
		return typ[i], true
	}
	return "", false
}

// search is searchListenerTree: it collects the listeners of every leaf typ matches
// into handlers (when not nil) and returns the leaves found by direct and "*"/"**"
// matching of the emitted pattern. Leaves reached through "*" and "**" branches of
// the tree add their listeners but are not returned, as in the original, so Off and
// RemoveAllListeners only reach the latter kind through the emitted pattern.
func (e *Emitter) search(handlers *[]*js.Function, typ []string, tree *node, i int) []*node {
	if tree == nil {
		return nil
	}
	n := len(typ)
	if i == n && tree.hasListeners() {
		if handlers != nil {
			*handlers = append(*handlers, tree.listeners...)
		}
		return []*node{tree}
	}
	current, curOK := segment(typ, i)
	next, nextOK := segment(typ, i+1)
	key := current
	if !curOK {
		key = "undefined"
	}
	var leaves []*node
	if (curOK && (current == "*" || current == "**")) || tree.child(key) != nil {
		switch {
		case curOK && current == "*":
			for _, branch := range tree.children.Keys() {
				leaves = append(leaves, e.search(handlers, typ, tree.child(branch), i+1)...)
			}
			return leaves
		case curOK && current == "**":
			endReached := i+1 == n || (i+2 == n && nextOK && next == "*")
			if endReached && tree.hasListeners() {
				leaves = append(leaves, e.search(handlers, typ, tree, n)...)
			}
			for _, branch := range tree.children.Keys() {
				sub := tree.child(branch)
				switch {
				case branch == "*" || branch == "**":
					if sub.hasListeners() && !endReached {
						leaves = append(leaves, e.search(handlers, typ, sub, n)...)
					}
					leaves = append(leaves, e.search(handlers, typ, sub, i)...)
				case nextOK && branch == next:
					leaves = append(leaves, e.search(handlers, typ, sub, i+2)...)
				default:
					// No match on this branch: move into the tree but not along the name.
					leaves = append(leaves, e.search(handlers, typ, sub, i)...)
				}
			}
			return leaves
		}
		leaves = append(leaves, e.search(handlers, typ, tree.child(key), i+1)...)
	}

	if x := tree.child("*"); x != nil {
		// Any segment matches here: explore the branch for the next one.
		e.search(handlers, typ, x, i+1)
	}
	if xx := tree.child("**"); xx != nil {
		switch {
		case i < n:
			if xx.hasListeners() {
				// A listener on "**" catches everything below.
				e.search(handlers, typ, xx, n)
			}
			for _, branch := range xx.children.Keys() {
				sub := xx.child(branch)
				switch {
				case nextOK && branch == next:
					// The next segment matches, so jump twice.
					e.search(handlers, typ, sub, i+2)
				case curOK && branch == current:
					e.search(handlers, typ, sub, i+1)
				default:
					isolated := newNode()
					isolated.children.Set(branch, sub)
					wrapper := newNode()
					wrapper.children.Set("**", isolated)
					e.search(handlers, typ, wrapper, i+1)
				}
			}
		case xx.hasListeners():
			// The end is reached while still on a "**".
			e.search(handlers, typ, xx, n)
		default:
			if s := xx.child("*"); s != nil && s.hasListeners() {
				e.search(handlers, typ, s, n)
			}
		}
	}
	return leaves
}
