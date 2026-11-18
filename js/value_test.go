package js

import "testing"

func TestNumberToString(t *testing.T) {
	a, b := 0.1, 0.2
	cases := map[float64]string{a + b: "0.30000000000000004", 1e21: "1e+21", 123e-20: "1.23e-18", 1e-7: "1e-7", 100: "100", -0.5: "-0.5"}
	for in, want := range cases {
		if got := NumberToString(in); got != want {
			t.Errorf("%v: got %q want %q", in, got, want)
		}
	}
}

func TestObjectKeyOrder(t *testing.T) {
	o := ObjectOf("b", 1, "2", 2, "a", 3, "1", 4, "01", 5)
	got := o.Keys()
	want := []string{"1", "2", "b", "a", "01"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("keys %v want %v", got, want)
		}
	}
}

func TestPropertyOperations(t *testing.T) {
	a := NewArray(1.0)
	SetProperty(a, "3", "x")
	if len(a.Items) != 4 || a.Items[1] != Undefined || a.Items[3] != "x" {
		t.Fatalf("a[3] = 'x' gives %v", a.Items)
	}
	SetProperty(a, "length", 2.0)
	SetProperty(a, "length", -1.0)
	SetProperty(a, "foo", 1.0)
	if n, _ := GetOwn(a, "length"); n != 2.0 {
		t.Fatalf("length = %v", n)
	}
	DeleteProperty(a, "0")
	if a.Items[0] != Undefined || HasOwn(a, "2") || !HasOwn(a, "1") {
		t.Fatalf("after delete a[0]: %v", a.Items)
	}
	if c, ok := GetOwn("a😀b", "3"); !ok || c != "b" {
		t.Errorf("'a😀b'[3] = %q, %v", c, ok)
	}
	e := NewTypeError("boom")
	SetProperty(e, "code", "E1")
	if HasOwn(e, "name") || !HasOwn(e, "message") || len(OwnKeys(e)) != 1 {
		t.Errorf("error own properties: name %v message %v keys %v", HasOwn(e, "name"), HasOwn(e, "message"), OwnKeys(e))
	}
}
