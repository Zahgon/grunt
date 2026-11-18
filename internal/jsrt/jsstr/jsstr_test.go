package jsstr

import (
	"testing"
)

func TestRoundTrip(t *testing.T) {
	cases := [][]uint16{
		{'a', 'b'},
		{0xD83D, 0xDE00},       // a surrogate pair: 😀
		{0xD800},               // a lone high surrogate
		{'x', 0xDC00, 'y'},     // a lone low surrogate
		{0xDE00, 0xD83D},       // reversed pair: two lone surrogates
		{0xE9, 0x65E5, 0x672C}, // é日本
	}
	for _, u := range cases {
		s := FromUTF16(u)
		back := ToUTF16(s)
		if len(back) != len(u) {
			t.Fatalf("%x: round trip gave %x", u, back)
		}
		for i := range u {
			if back[i] != u[i] {
				t.Fatalf("%x: round trip gave %x", u, back)
			}
		}
		if Length(s) != len(u) {
			t.Errorf("Length(%q) = %d, want %d", s, Length(s), len(u))
		}
	}
	if got := ToWellFormed(FromUTF16([]uint16{'a', 0xD800, 'b'})); got != "a�b" {
		t.Errorf("ToWellFormed = %q", got)
	}
}
