package grunt

import (
	"errors"

	"github.com/gruntjs/grunt-go/internal/jsrt"
	"github.com/gruntjs/grunt-go/js"
)

// This file maps JavaScript exceptions onto Go: a throw is a panic with the thrown
// value (an error for Error objects), and the places where grunt catches
// exceptions recover it.

// catch runs fn and returns the value it panicked with, if any.
func catch(fn func()) (thrown any, ok bool) {
	defer func() {
		if v := recover(); v != nil {
			if _, exit := v.(exitSignal); exit {
				panic(v)
			}
			thrown, ok = v, true
		}
	}()
	fn()
	return nil, false
}

// throws runs fn and returns what it threw (panicked with) as an error: errors as
// they are, other values as the *jsrt.Thrown that carries them.
func throws(fn func()) error {
	thrown, ok := catch(fn)
	if !ok {
		return nil
	}
	if err, isErr := thrown.(error); isErr {
		return err
	}
	return &jsrt.Thrown{Value: thrown}
}

// exitSignal is the panic value of a process.exit that does not end the program
// (the tests' process): it unwinds the run to the caller, so every recover that
// stands for a JavaScript catch passes it on (see catch).
type exitSignal struct{ code int }

// asJSError returns err as a *js.Error, wrapping errors that are not one.
func asJSError(err error) *js.Error {
	var e *js.Error
	if errors.As(err, &e) {
		return e
	}
	return &js.Error{Name: "Error", Message: err.Error(), Orig: err}
}

// errorStack is err.stack without the frames.
func errorStack(err error) string {
	if je, ok := err.(*js.Error); ok {
		return js.ToString(je)
	}
	return "Error: " + err.Error()
}

// must panics with err: the emitter's errors are the exceptions EventEmitter2
// throws (a failing listener, an unhandled "error" event).
func must(err error) {
	if err != nil {
		panic(err)
	}
}
