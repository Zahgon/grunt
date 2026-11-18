package jsrt

import (
	"sync"

	"github.com/gruntjs/grunt-go/js"
)

// intrinsics is the realm whose builtin constructors and prototypes the values of
// the model inherit from outside a template: lodash's property reads, getobject's
// and dateformat's lookups. Those lookups only read it.
var intrinsics = sync.OnceValue(func() *Realm { return NewRealm(Options{}) })

// Inherited returns the value v inherits for key from the builtin prototypes of its
// type (Object.prototype, Array.prototype, String.prototype, Function.prototype,
// the error prototypes and so on), and whether one has the key: constructor is
// v's builtin constructor, and the methods are the realm's builtin functions. v
// must not be null or undefined.
//
// __proto__, Object.prototype's accessor, reads as v's prototype as the model
// sees it: the prototype of a Derived object, and for other values the
// enumerable data of their builtin prototype, which is none, so an empty array
// for an array (Array.prototype is itself an array) and an empty object for
// anything else. The builtin functions belong to a realm of their own, so they are
// not the same values as the globals of a template's realm.
func Inherited(v any, key string) (any, bool) {
	if key == "__proto__" {
		switch x := v.(type) {
		case *Derived:
			return x.Proto, true
		case *js.Array:
			return js.NewArray(), true
		}
		return js.NewObject(), true
	}
	r := intrinsics()
	return r.lookup(r.protoOf(v), key)
}

// GetProperty is v[key] outside a template: v's own property (js.GetOwn, or the
// own properties of a Derived object), else what it inherits (Inherited), else
// undefined. Null and undefined have no properties.
func GetProperty(v any, key string) any {
	if js.IsNullish(v) {
		return js.Undefined
	}
	if d, ok := v.(*Derived); ok && d.Own.Has(key) {
		return d.Own.Get(key)
	}
	if val, ok := js.GetOwn(v, key); ok {
		return val
	}
	val, _ := Inherited(v, key)
	return val
}

// HasProperty is `key in Object(v)` outside a template, with the properties
// GetProperty reads; null and undefined have none.
func HasProperty(v any, key string) bool {
	if js.IsNullish(v) {
		return false
	}
	if d, ok := v.(*Derived); ok && d.Own.Has(key) {
		return true
	}
	if js.HasOwn(v, key) {
		return true
	}
	_, ok := Inherited(v, key)
	return ok
}
