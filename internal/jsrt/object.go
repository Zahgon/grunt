package jsrt

import (
	"github.com/gruntjs/grunt-go/js"
)

// Derived is an object with an explicit prototype, as Object.create(proto) builds
// it. Own holds its own properties; Proto is the next object on the prototype
// chain: a *js.Object, another *Derived, or nil for Object.create(null).
//
// grunt renders templates with Object.create(configData) plus a grunt property, so
// a caller passes WithProto(own, configData) to Template.Execute.
type Derived struct {
	Own   *js.Object
	Proto any
}

// WithProto returns an object whose own properties are own (a new empty object
// when nil) and whose prototype is proto.
func WithProto(own *js.Object, proto any) *Derived {
	if own == nil {
		own = js.NewObject()
	}
	return &Derived{Own: own, Proto: proto}
}

// arguments is a function's arguments object: array-like but not an array.
type arguments struct {
	items  []any
	callee *js.Function
}

// iterator is a built-in iterator (Array Iterator, String Iterator, RegExp String
// Iterator) over values computed when it was created.
type iterator struct {
	items []any
	pos   int
	// tag is the class Object.prototype.toString reports, e.g. "Array Iterator".
	tag string
}

// protoOf returns the [[Prototype]] of v: a prototype object, or nil at the end of
// the chain. Primitives report the prototype their wrapper object would have.
func (r *Realm) protoOf(v any) any {
	switch x := v.(type) {
	case *js.Object:
		if x == r.objectProto {
			return nil
		}
		if p, ok := r.protoLinks[x]; ok {
			return p
		}
		return r.objectProto
	case *Derived:
		return x.Proto
	case *js.Array:
		return r.arrayProto
	case string:
		return r.stringProto
	case float64:
		return r.numberProto
	case bool:
		return r.booleanProto
	case *js.Function:
		return r.functionProto
	case js.Date:
		return r.dateProto
	case *js.RegExp:
		return r.regexpProto
	case *js.Error:
		if p, ok := r.errorProtos[x.Name]; ok {
			return p
		}
		return r.errorProtos["Error"]
	case *arguments, []byte:
		return r.objectProto
	case *iterator:
		return r.iteratorProto
	}
	return nil
}

// lookup implements [[Get]] without accessors: the value of key on v or its
// prototype chain, and whether it was found. v must not be null or undefined.
func (r *Realm) lookup(v any, key string) (any, bool) {
	for v != nil {
		if val, ok := r.getOwn(v, key); ok {
			return val, true
		}
		v = r.protoOf(v)
	}
	return js.Undefined, false
}

// getOwn returns the own property key of v: the data the value model holds
// (js.GetOwn), plus the properties this realm keeps for objects whose Go type has
// no room for them.
func (r *Realm) getOwn(v any, key string) (any, bool) {
	switch x := v.(type) {
	case *Derived:
		if x.Own.Has(key) {
			return x.Own.Get(key), true
		}
		return nil, false
	case *js.Array:
		if val, ok := js.GetOwn(x, key); ok {
			return val, true
		}
	case *js.Object, string, float64, bool, js.Date, *js.Error, []byte:
		return js.GetOwn(x, key)
	case *js.Function:
		if p := r.extra[x]; p != nil && p.Has(key) {
			return p.Get(key), true
		}
		if key == "prototype" {
			if proto := r.functionPrototype(x); proto != nil {
				return proto, true
			}
			return nil, false
		}
		return js.GetOwn(x, key)
	case *js.RegExp:
		if key == "lastIndex" {
			return r.regexpState(x).lastIndex, true
		}
		// The flag and source accessors live on RegExp.prototype in JavaScript;
		// they are read as own values here.
		if v, ok := r.regexpProp(x, key); ok {
			return v, true
		}
	case *arguments:
		if i, ok := js.ArrayIndex(key); ok && i < len(x.items) {
			return x.items[i], true
		}
		switch key {
		case "length":
			return float64(len(x.items)), true
		case "callee":
			return x.callee, true
		}
		return nil, false
	}
	if p := r.extra[v]; p != nil && p.Has(key) {
		return p.Get(key), true
	}
	return nil, false
}

// hasProperty is the HasProperty operation behind "in" and with-scope lookup.
func (r *Realm) hasProperty(v any, key string) bool {
	_, ok := r.lookup(v, key)
	return ok
}

// getV reads key from any value, as GetV does: primitives read through their
// prototype; null and undefined throw V8's TypeError.
func (r *Realm) getV(v any, key string) (any, error) {
	if js.IsNullish(v) {
		return nil, js.NewTypeError("Cannot read properties of " + js.ToString(v) + " (reading '" + key + "')")
	}
	val, _ := r.lookup(v, key)
	return val, nil
}

// extraProps returns the side table holding v's own properties beyond the ones its
// Go representation stores, creating it if asked.
func (r *Realm) extraProps(v any, create bool) *js.Object {
	p := r.extra[v]
	if p == nil && create {
		p = js.NewObject()
		r.extra[v] = p
	}
	return p
}

// put implements [[Set]] for a data property in sloppy mode: writes to primitives
// are silently dropped.
func (r *Realm) put(v any, key string, val any) error {
	if js.IsNullish(v) {
		return js.NewTypeError("Cannot set properties of " + js.ToString(v) + " (setting '" + key + "')")
	}
	switch x := v.(type) {
	case *js.Object:
		if r.frozen[x] || r.readOnly[readOnlyKey{x, key}] {
			return nil
		}
		x.Set(key, val)
	case *Derived:
		x.Own.Set(key, val)
	case *js.Array:
		if i, ok := js.ArrayIndex(key); ok {
			x.SetIndex(i, val)
			return nil
		}
		if key == "length" {
			return r.setArrayLength(x, val)
		}
		r.extraProps(x, true).Set(key, val)
	case *js.Function:
		if key == "name" || key == "length" {
			// Function name and length are read-only.
			return nil
		}
		p := r.extraProps(x, true)
		if r.readOnly[readOnlyKey{p, key}] {
			return nil
		}
		p.Set(key, val)
	case *js.RegExp:
		if key == "lastIndex" {
			r.regexpState(x).lastIndex = val
			return nil
		}
		r.extraProps(x, true).Set(key, val)
	case *js.Error:
		if key == "message" || key == "name" {
			// The model stores these as strings; convert with this realm's ToString.
			s, err := r.toString(val)
			if err != nil {
				return err
			}
			val = s
		}
		js.SetProperty(x, key, val)
	case *arguments:
		if i, ok := js.ArrayIndex(key); ok && i < len(x.items) {
			x.items[i] = val
		}
	case []byte:
		if i, ok := js.ArrayIndex(key); ok && i < len(x) {
			n, err := r.toNumber(val)
			if err != nil {
				return err
			}
			x[i] = byte(js.ToUint32(n))
		}
	}
	return nil
}

// setArrayLength implements ArraySetLength: truncation or extension.
func (r *Realm) setArrayLength(a *js.Array, val any) error {
	n, err := r.toNumber(val)
	if err != nil {
		return err
	}
	l := js.ToUint32(n)
	if float64(l) != n {
		return js.InvalidArrayLength()
	}
	a.SetLength(int(l))
	return nil
}

// deleteProp is the delete operator on a property; it reports false for the
// properties that cannot be deleted.
func (r *Realm) deleteProp(v any, key string) (bool, error) {
	if js.IsNullish(v) {
		return false, js.NewTypeError("Cannot convert undefined or null to object")
	}
	switch x := v.(type) {
	case *js.Object:
		if r.frozen[x] {
			return !x.Has(key), nil
		}
		x.Delete(key)
		if ne := r.nonEnum[x]; ne != nil {
			delete(ne, key)
		}
	case *Derived:
		x.Own.Delete(key)
	case *js.Array:
		if _, ok := js.ArrayIndex(key); ok {
			js.DeleteProperty(x, key)
			return true, nil
		}
		if key == "length" {
			return false, nil
		}
		if p := r.extraProps(x, false); p != nil {
			p.Delete(key)
		}
	case string:
		// A string's length and indexes cannot be deleted.
		if js.HasOwn(x, key) {
			return false, nil
		}
	case *js.Error:
		js.DeleteProperty(x, key)
	default:
		if p := r.extraProps(v, false); p != nil {
			p.Delete(key)
		}
	}
	return true, nil
}

// ownKeys returns v's own enumerable string keys in JavaScript order.
func (r *Realm) ownKeys(v any) []string {
	switch x := v.(type) {
	case *js.Object:
		keys := x.Keys()
		if ne := r.nonEnum[x]; ne != nil {
			out := keys[:0]
			for _, k := range keys {
				if !ne[k] {
					out = append(out, k)
				}
			}
			return out
		}
		return keys
	case *Derived:
		return x.Own.Keys()
	case *js.Array:
		keys := js.OwnKeys(x)
		if p := r.extraProps(x, false); p != nil {
			keys = append(keys, p.Keys()...)
		}
		return keys
	case string, []byte, *js.Error:
		return js.OwnKeys(x)
	case *arguments:
		return js.IndexKeys(len(x.items))
	case *js.Function:
		if p := r.extraProps(x, false); p != nil {
			var keys []string
			for _, k := range p.Keys() {
				if k != "prototype" && !r.nonEnum[p][k] {
					keys = append(keys, k)
				}
			}
			return keys
		}
		return nil
	}
	if p := r.extraProps(v, false); p != nil {
		return p.Keys()
	}
	return nil
}

// isOwnNonEnumerable reports an own key that exists but is not enumerable; it
// shadows the same key further up the chain during for-in.
func (r *Realm) isOwnNonEnumerable(v any, key string) bool {
	switch x := v.(type) {
	case *js.Object:
		return x.Has(key) && r.nonEnum[x][key]
	case *js.Array:
		return key == "length"
	case string:
		return key == "length"
	case *js.Function:
		return key == "name" || key == "length" || key == "prototype"
	case *js.Error:
		return key == "message" || key == "stack"
	case *js.RegExp:
		return key == "lastIndex"
	}
	return false
}

// forInKeys lists the keys for-in visits: own enumerable keys, then those of each
// prototype not already seen, skipping keys shadowed by non-enumerable ones.
func (r *Realm) forInKeys(v any) []string {
	if js.IsNullish(v) {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for v != nil {
		for _, k := range r.ownKeys(v) {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
		if o, ok := v.(*js.Object); ok {
			for k := range r.nonEnum[o] {
				seen[k] = true
			}
		}
		v = r.protoOf(v)
	}
	return out
}

// hasOwn is Object.prototype.hasOwnProperty.
func (r *Realm) hasOwn(v any, key string) bool {
	if _, ok := r.getOwn(v, key); ok {
		return true
	}
	return false
}

// functionPrototype returns the prototype object of a user-defined constructor,
// creating it on first use.
func (r *Realm) functionPrototype(f *js.Function) any {
	c := r.closures[f]
	if c == nil || c.fi.arrow || c.fi.method {
		return nil
	}
	proto := js.NewObject()
	proto.Set("constructor", f)
	r.markNonEnum(proto, "constructor")
	r.extraProps(f, true).Set("prototype", proto)
	return proto
}

func (r *Realm) markNonEnum(o *js.Object, keys ...string) {
	ne := r.nonEnum[o]
	if ne == nil {
		ne = map[string]bool{}
		r.nonEnum[o] = ne
	}
	for _, k := range keys {
		ne[k] = true
	}
}
