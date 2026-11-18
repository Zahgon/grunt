package grunt

import (
	"io"

	"github.com/gruntjs/grunt-go/internal/events"
	"github.com/gruntjs/grunt-go/js"
)

// Event is grunt.event: an EventEmitter2 with wildcards, so "build.*" matches
// "build.start" and "a.**" matches "a.b.c".
//
// Listeners are *js.Function values (compared by identity, so Off needs the value
// On was given) or Go functions of type func(event string, args ...any), which
// receive the name of the event being emitted (this.event in JavaScript). A Go
// function is wrapped anew on every call; to remove it later, wrap it once with
// EventListener and pass that value to both On and Off.
type Event struct {
	e *events.Emitter
}

func newEvent(stderr io.Writer) *Event {
	return &Event{e: events.New(events.Options{Stderr: stderr})}
}

// EventListener wraps a Go function as an event listener.
func EventListener(fn func(event string, args ...any)) *js.Function {
	return events.Listener("", func(event string, args []any) error {
		fn(event, args...)
		return nil
	})
}

// listener converts fn to the emitter's listener type.
func listener(fn any) *js.Function {
	switch f := fn.(type) {
	case *js.Function:
		return f
	case func(string, ...any):
		return EventListener(f)
	}
	panic(js.NewTypeError("listener must be a function"))
}

// On adds a listener for event, which may contain "*" and "**" segments.
func (e *Event) On(event string, fn any) *Event {
	must(e.e.On(event, listener(fn)))
	return e
}

// Once adds a listener that runs at most once.
func (e *Event) Once(event string, fn any) *Event {
	must(e.e.Once(event, listener(fn)))
	return e
}

// Many adds a listener that runs at most n times.
func (e *Event) Many(event string, n int, fn any) *Event {
	must(e.e.Many(event, n, listener(fn)))
	return e
}

// Off removes a listener added with On, Once or Many.
func (e *Event) Off(event string, fn any) *Event {
	must(e.e.Off(event, listener(fn)))
	return e
}

// OnAny adds a listener for every event.
func (e *Event) OnAny(fn any) *Event {
	must(e.e.OnAny(listener(fn)))
	return e
}

// OffAny removes a listener added with OnAny; a nil fn removes them all.
func (e *Event) OffAny(fn any) *Event {
	if fn == nil {
		e.e.OffAny(nil)
	} else {
		e.e.OffAny(listener(fn))
	}
	return e
}

// Emit calls the listeners of event with args and reports whether there were any.
// A listener's error, or an "error" event nobody listens to, panics with the error
// the original throws.
func (e *Event) Emit(event string, args ...any) bool {
	for i, a := range args {
		args[i] = js.Normalize(a)
	}
	handled, err := e.e.Emit(event, args...)
	must(err)
	return handled
}

// RemoveAllListeners removes the listeners of event, or of every event.
func (e *Event) RemoveAllListeners(event ...string) *Event {
	e.e.RemoveAllListeners(event...)
	return e
}

// Listeners returns how many listeners event would call
// (grunt.event.listeners(event).length).
func (e *Event) Listeners(event string) int { return len(e.e.Listeners(event)) }

// SetMaxListeners sets the number of listeners per event above which a possible
// leak is reported on stderr (10 by default, 0 for no limit).
func (e *Event) SetMaxListeners(n int) { e.e.SetMaxListeners(n) }
