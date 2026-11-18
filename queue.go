package grunt

import (
	"slices"
	"sync"
	"time"

	"github.com/gruntjs/grunt-go/js"
)

// This file is the task queue of lib/util/task.js: enqueuing, the run loop and the
// completion of synchronous and asynchronous tasks.

// push inserts items just before the placeholder, or at the end without one.
func (r *TaskRunner) push(items ...*queueItem) {
	if i := slices.Index(r.queue, r.placeholder); i >= 0 {
		r.queue = slices.Insert(r.queue, i, items...)
	} else {
		r.queue = append(r.queue, items...)
	}
}

// Run enqueues tasks: names as arguments or as one array. A missing task fails
// with `Task "x" not found.` (thrown inside a running task, reported as a warning
// otherwise).
func (r *TaskRunner) Run(names ...any) *TaskRunner {
	var items []*queueItem
	for _, n := range parseArgs(names) {
		name, ok := n.(string)
		if !ok {
			panic(js.NewTypeError("str.replace is not a function"))
		}
		items = append(items, r.taskPlusArgs(name))
	}
	for _, it := range items {
		if it.task == nil {
			r.throwIfRunning(js.NewError(`Task "` + it.nameArgs + `" not found.`))
			return r
		}
	}
	r.push(items...)
	return r
}

// Mark adds a marker to the queue, for ClearQueue(true).
func (r *TaskRunner) Mark() *TaskRunner {
	r.push(r.marker)
	return r
}

// ClearQueue empties the queue, or with untilMarker removes everything up to and
// including the first marker.
func (r *TaskRunner) ClearQueue(untilMarker ...bool) *TaskRunner {
	if len(untilMarker) > 0 && untilMarker[0] {
		// splice(0, indexOf(marker) + 1): nothing without a marker.
		r.queue = r.queue[slices.Index(r.queue, r.marker)+1:]
	} else {
		r.queue = nil
	}
	return r
}

// Requires fails with `Required task "x" must be run first.` (or `... failed.`)
// unless every named task (name with arguments, as run) has succeeded.
func (r *TaskRunner) Requires(names ...any) error {
	for _, n := range parseArgs(names) {
		name := js.ToString(n)
		ok, ran := r.success[name]
		if !ok {
			state := "must be run first"
			if ran {
				state = "failed"
			}
			return js.NewError(`Required task "` + name + `" ` + state + ".")
		}
	}
	return nil
}

// start is Task.prototype.start: it runs the queue until it is empty, then calls
// the done handler.
func (r *TaskRunner) start() {
	if r.running {
		return
	}
	r.running = true
	for {
		// Get the next task, skipping any placeholders or markers.
		var thing *queueItem
		for len(r.queue) > 0 {
			thing = r.queue[0]
			r.queue = r.queue[1:]
			if thing != r.placeholder && thing != r.marker {
				break
			}
			thing = nil
		}
		if thing == nil {
			r.running = false
			if r.onDone != nil {
				r.onDone()
			}
			return
		}
		// Add a placeholder to the front of the queue, so that tasks this task runs
		// are inserted before the rest of the queue.
		r.queue = append([]*queueItem{r.placeholder}, r.queue...)
		r.runTaskFn(&Task{
			Name:     thing.task.name,
			NameArgs: thing.nameArgs,
			Args:     append([]string{}, thing.args...),
			Flags:    thing.flags,
			runner:   r,
		}, thing.task.fn)
	}
}

// runTaskFn is Task.prototype.runTaskFn: it runs fn and waits for its completion,
// synchronous or asynchronous.
func (r *TaskRunner) runTaskFn(t *Task, fn func(t *Task) any) {
	async := false
	// done carries the task's asynchronous result. It is the task's own and holds
	// one value, and the done function sends at most once, so the timer never
	// blocks: a result that arrives after the task has already ended (it threw
	// after calling Async) stays in the buffer and is collected with the task.
	done := make(chan any, 1)
	t.async = func() func(any) {
		async = true
		var once sync.Once
		return func(result any) {
			once.Do(func() {
				// The original completes the task in a 1 ms timer, so that
				// this.async()() inside a task still ends it asynchronously.
				time.AfterFunc(time.Millisecond, func() { done <- js.Normalize(result) })
			})
		}
	}
	r.current = t
	var result any
	thrown, threw := catch(func() { result = js.Normalize(fn(t)) })
	if threw {
		result = thrown
	}
	if !async || threw {
		r.complete(t, result)
		return
	}
	r.complete(t, <-done)
}

// complete records a task's success and reports a failure to the error handler.
func (r *TaskRunner) complete(t *Task, result any) {
	var err error
	switch x := result.(type) {
	case bool:
		if !x {
			// Since false was passed, the task failed generically.
			err = js.NewError(`Task "` + t.NameArgs + `" failed.`)
		}
	case error:
		err = x
	}
	r.current = nil
	r.success[t.NameArgs] = err == nil
	if err != nil && r.onError != nil {
		r.onError(err)
	}
}
