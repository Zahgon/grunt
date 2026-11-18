package jsrt

import (
	"errors"
	"testing"

	"github.com/gruntjs/grunt-go/js"
)

func TestExecuteAPI(t *testing.T) {
	realm := NewRealm(Options{Now: func() float64 { return 86400000 }})
	s := Delimiters("<%", "%>")
	s.Realm = realm
	s.Imports = testImports(realm)
	config := js.ObjectOf("pkg", js.ObjectOf("name", "grunt", "version", "1.6.3"))
	data := WithProto(js.ObjectOf("grunt", js.ObjectOf("version", "1.6.3")), config)

	tmpl, err := CompileTemplate("<%= pkg.name %>@<%- grunt.version %> <%= new Date().toISOString() %>", s)
	if err != nil {
		t.Fatal(err)
	}
	got, err := tmpl.Execute(data)
	if err != nil || got != "grunt@1.6.3 1970-01-02T00:00:00.000Z" {
		t.Fatalf("Execute = %q, %v", got, err)
	}

	// Globals created by one template are seen by the next in the same realm.
	first, err := CompileTemplate("<% counter = 41 %>", s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Execute(data); err != nil {
		t.Fatal(err)
	}
	second, err := CompileTemplate("<%= ++counter %>", s)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := second.Execute(data); err != nil || got != "42" {
		t.Fatalf("shared global = %q, %v", got, err)
	}
}

func TestErrors(t *testing.T) {
	s := Delimiters("<%", "%>")
	if _, err := CompileTemplate("<%= a b %>", s); err == nil || err.Error() != "Unexpected identifier 'b'" {
		t.Errorf("syntax error = %v", err)
	}
	tmpl, err := CompileTemplate("<% throw 'boom' %>", s)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tmpl.Execute(nil)
	var thrown *Thrown
	if !errors.As(err, &thrown) || thrown.Value != "boom" {
		t.Errorf("throw 'boom' gave %#v", err)
	}
	tmpl, err = CompileTemplate("<%= missing %>", s)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tmpl.Execute(nil)
	var jsErr *js.Error
	if !errors.As(err, &jsErr) || jsErr.Name != "ReferenceError" || jsErr.Message != "missing is not defined" {
		t.Errorf("missing gave %#v", err)
	}
	// Without a clock, the current time is an error rather than the wall clock.
	tmpl, err = CompileTemplate("<%= Date.now() %>", s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmpl.Execute(nil); err == nil {
		t.Error("Date.now() without a clock succeeded")
	}
}

func TestJSONHelpers(t *testing.T) {
	o := js.ObjectOf("a", js.NewArray(1.0, "x"), "d", js.Date{Ms: 0})
	got, ok, err := JSONStringify(o, nil, 2.0)
	want := "{\n  \"a\": [\n    1,\n    \"x\"\n  ],\n  \"d\": \"1970-01-01T00:00:00.000Z\"\n}"
	if err != nil || !ok || got != want {
		t.Errorf("JSONStringify = %q, %v, %v", got, ok, err)
	}
	if _, err := JSONParse("{x"); err == nil || err.Error() != "Expected property name or '}' in JSON at position 1 (line 1 column 2)" {
		t.Errorf("JSONParse error = %v", err)
	}
}
