package js

import (
	"strconv"
)

// Object is a plain JavaScript object. Keys enumerate as JavaScript orders them:
// array-index keys ascending, then the other string keys in insertion order.
type Object struct {
	keys   []string
	values map[string]any
}

// NewObject returns an empty object.
func NewObject() *Object {
	return &Object{values: map[string]any{}}
}

// ObjectOf builds an object from alternating keys and values.
func ObjectOf(kv ...any) *Object {
	o := NewObject()
	for i := 0; i+1 < len(kv); i += 2 {
		k, _ := kv[i].(string)
		o.Set(k, Normalize(kv[i+1]))
	}
	return o
}

// ArrayIndex reports whether the property key k is an array index, the canonical
// decimal form of an integer from 0 to 2^32-2, and returns that integer. Array-index
// keys are the ones objects list first, in ascending order.
func ArrayIndex(k string) (int, bool) {
	if k == "" || len(k) > 10 || (len(k) > 1 && k[0] == '0') {
		return 0, false
	}
	n, err := strconv.ParseUint(k, 10, 64)
	if err != nil || n > 4294967294 {
		return 0, false
	}
	return int(n), true
}

// IndexKeys returns the array-index keys "0" to n-1: what Object.keys lists for
// an array, a string or a Buffer of length n.
func IndexKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = strconv.Itoa(i)
	}
	return keys
}

// Has reports whether k is an own key.
func (o *Object) Has(k string) bool {
	_, ok := o.values[k]
	return ok
}

// Get returns the value at k, or Undefined.
func (o *Object) Get(k string) any {
	if v, ok := o.values[k]; ok {
		return v
	}
	return Undefined
}

// Set stores v at k, keeping k's position if it already exists.
func (o *Object) Set(k string, v any) {
	if _, ok := o.values[k]; !ok {
		o.insertKey(k)
	}
	o.values[k] = v
}

func (o *Object) insertKey(k string) {
	n, isIndex := ArrayIndex(k)
	if !isIndex {
		o.keys = append(o.keys, k)
		return
	}
	// Indexes sit before every string key, in ascending order.
	pos := 0
	for pos < len(o.keys) {
		m, ok := ArrayIndex(o.keys[pos])
		if !ok || m > n {
			break
		}
		pos++
	}
	o.keys = append(o.keys, "")
	copy(o.keys[pos+1:], o.keys[pos:])
	o.keys[pos] = k
}

// Delete removes k.
func (o *Object) Delete(k string) {
	if _, ok := o.values[k]; !ok {
		return
	}
	delete(o.values, k)
	for i, key := range o.keys {
		if key == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// Keys returns the own enumerable keys in JavaScript order.
func (o *Object) Keys() []string {
	return append([]string(nil), o.keys...)
}

// Len returns the number of keys.
func (o *Object) Len() int { return len(o.keys) }

// Clone returns a shallow copy.
func (o *Object) Clone() *Object {
	c := NewObject()
	for _, k := range o.keys {
		c.Set(k, o.values[k])
	}
	return c
}
