package events

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gruntjs/grunt-go/js"
)

// testdata/cases.json is recorded from EventEmitter2 0.4.14 under Node 24 by
// testdata/gen.js.

type eventCase struct {
	Conf struct {
		Delimiter    string `json:"delimiter"`
		MaxListeners int    `json:"maxListeners"`
		NewListener  bool   `json:"newListener"`
	} `json:"conf"`
	Ops []struct {
		Op     string   `json:"op"`
		Event  *string  `json:"event"`
		Fn     string   `json:"fn"`
		TTL    int      `json:"ttl"`
		N      int      `json:"n"`
		Args   []any    `json:"args"`
		Err    string   `json:"err"`
		Ret    any      `json:"ret"`
		Throws string   `json:"throws"`
		Calls  []string `json:"calls"`
	} `json:"ops"`
	Stderr string `json:"stderr"`
}

var listenerNames = []string{"L0", "L1", "L2", "L3", "L4", "T", "A"}

// showArgs renders listener arguments as gen.js does: JSON with functions as
// "fn:name" and errors as {}.
func showArgs(t *testing.T, args []any) string {
	shown := make([]any, len(args))
	for i, a := range args {
		switch x := a.(type) {
		case *js.Function:
			shown[i] = "fn:" + x.Name
		case *js.Error:
			// JSON.stringify of an Error: no enumerable properties.
			shown[i] = struct{}{}
		default:
			shown[i] = a
		}
	}
	b, err := json.Marshal(shown)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestEmitterFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []eventCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for ci, c := range cases {
		var stderr bytes.Buffer
		e := New(Options{Delimiter: c.Conf.Delimiter, MaxListeners: c.Conf.MaxListeners, NewListener: c.Conf.NewListener, Stderr: &stderr})
		var calls []string
		fns := map[string]*js.Function{}
		for _, name := range listenerNames {
			fns[name] = &js.Function{Name: name, Call: func(this any, args []any) (any, error) {
				em := this.(*Emitter)
				calls = append(calls, name+"|"+em.Event()+"|"+showArgs(t, args))
				switch name {
				case "T":
					return nil, js.NewError("thrown by T on " + em.Event())
				case "A":
					em.OffAny(nil)
				}
				return js.Undefined, nil
			}}
		}
		for oi, op := range c.Ops {
			event := ""
			if op.Event != nil {
				event = *op.Event
			}
			var err error
			var ret any
			switch op.Op {
			case "on":
				err = e.On(event, fns[op.Fn])
			case "once":
				err = e.Once(event, fns[op.Fn])
			case "many":
				err = e.Many(event, op.TTL, fns[op.Fn])
			case "emit":
				args := op.Args
				if op.Err != "" {
					args = append([]any{js.NewError(op.Err)}, args...)
				}
				ret, err = e.Emit(event, args...)
			case "off":
				err = e.Off(event, fns[op.Fn])
			case "onAny":
				err = e.OnAny(fns[op.Fn])
			case "offAny":
				e.OffAny(fns[op.Fn])
			case "removeAllListeners":
				if op.Event != nil {
					e.RemoveAllListeners(event)
				} else {
					e.RemoveAllListeners()
				}
			case "listeners":
				names := []any{}
				for _, f := range e.Listeners(event) {
					names = append(names, f.Name)
				}
				ret = names
			case "setMaxListeners":
				e.SetMaxListeners(op.N)
			}
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != op.Throws {
				t.Errorf("case %d op %d %s(%q): error %q, want %q", ci, oi, op.Op, event, got, op.Throws)
			}
			if op.Throws == "" && (op.Op == "emit" || op.Op == "listeners") && !reflect.DeepEqual(ret, op.Ret) {
				t.Errorf("case %d op %d %s(%q): returned %v, want %v", ci, oi, op.Op, event, ret, op.Ret)
			}
			if strings.Join(calls, "\n") != strings.Join(op.Calls, "\n") {
				t.Errorf("case %d op %d %s(%q): calls\n%q\nwant\n%q", ci, oi, op.Op, event, calls, op.Calls)
			}
			calls = nil
		}
		if stderr.String() != c.Stderr {
			t.Errorf("case %d stderr %q, want %q", ci, stderr.String(), c.Stderr)
		}
	}
}

func TestListenerReceivesEvent(t *testing.T) {
	e := New(Options{})
	var got []string
	l := Listener("watch", func(event string, args []any) error {
		got = append(got, event)
		return nil
	})
	if err := e.On("watch.*", l); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Emit("watch.changed", "file.js"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"watch.changed"}) {
		t.Errorf("events = %v", got)
	}
}

func TestUnhandledErrorEvent(t *testing.T) {
	e := New(Options{})
	want := js.NewError("boom")
	if _, err := e.Emit("error", want); !errors.Is(err, want) {
		t.Errorf("Emit(error) = %v, want %v", err, want)
	}
}
