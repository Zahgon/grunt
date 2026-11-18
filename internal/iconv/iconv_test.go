package iconv

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"os"
	"slices"
	"sort"
	"testing"

	"github.com/gruntjs/grunt-go/js"
)

// The fixtures in testdata were recorded from iconv-lite 0.6.3 under Node.js
// 24 by testdata/record.js.

type fixtureMeta struct {
	Node      string `json:"node"`
	IconvLite string `json:"iconvLite"`
}

type decodeCase struct {
	Enc   string `json:"enc"`
	Hex   string `json:"hex"`
	Strip bool   `json:"strip"`
	Out   string `json:"out"`
}

type encodeCase struct {
	Enc string `json:"enc"`
	In  string `json:"in"`
	BOM bool   `json:"bom"`
	Hex string `json:"hex"`
}

type nameCase struct {
	Name      string `json:"name"`
	Canonical string `json:"canonical"`
	Exists    bool   `json:"exists"`
	Decoded   string `json:"decoded"`
	Encoded   string `json:"encoded"`
	Error     string `json:"error"`
}

type sweep struct {
	Enc    string `json:"enc"`
	Sweep  string `json:"sweep"`
	Count  int    `json:"count"`
	SHA256 string `json:"sha256"`
}

func loadFixture(t *testing.T, file string, v any) {
	t.Helper()
	data, err := os.ReadFile("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%s: %v", file, err)
	}
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCodecDefsMatchGeneratedTable(t *testing.T) {
	var keys []string
	for k := range codecDefs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if !slices.Equal(keys, generatedCodecNames) {
		t.Fatalf("codecDefs keys %v\nwant generatedCodecNames %v", keys, generatedCodecNames)
	}
	for name := range aliasTable {
		if _, dup := sbcsTable[name]; dup {
			t.Errorf("%q is both an alias and an SBCS table", name)
		}
	}
}

func TestFixtureCases(t *testing.T) {
	var fx struct {
		Meta   fixtureMeta  `json:"meta"`
		Decode []decodeCase `json:"decode"`
		Encode []encodeCase `json:"encode"`
		Names  []nameCase   `json:"names"`
	}
	loadFixture(t, "cases.json", &fx)
	t.Logf("fixtures from iconv-lite %s on Node.js %s", fx.Meta.IconvLite, fx.Meta.Node)

	t.Run("decode", func(t *testing.T) {
		for _, c := range fx.Decode {
			got, err := Decode(mustHex(t, c.Hex), c.Enc, c.Strip)
			if err != nil || got != c.Out {
				t.Errorf("Decode(%s, %q, %v) = %+q, %v; want %+q", c.Hex, c.Enc, c.Strip, got, err, c.Out)
			}
		}
		t.Logf("%d cases", len(fx.Decode))
	})
	t.Run("encode", func(t *testing.T) {
		for _, c := range fx.Encode {
			got, err := Encode(c.In, c.Enc, c.BOM)
			if err != nil || hex.EncodeToString(got) != c.Hex {
				t.Errorf("Encode(%+q, %q, %v) = %x, %v; want %s", c.In, c.Enc, c.BOM, got, err, c.Hex)
			}
		}
		t.Logf("%d cases", len(fx.Encode))
	})
	t.Run("names", func(t *testing.T) {
		probe := mustHex(t, "41e980fffe00a4a130a1a24f2b2d")
		for _, c := range fx.Names {
			if got := canonicalize(c.Name); got != c.Canonical {
				t.Errorf("canonicalize(%q) = %q; want %q", c.Name, got, c.Canonical)
			}
			if got := EncodingExists(c.Name); got != c.Exists {
				t.Errorf("EncodingExists(%q) = %v; want %v", c.Name, got, c.Exists)
			}
			decoded, err := Decode(probe, c.Name, true)
			if c.Error != "" {
				var jsErr *js.Error
				if !errors.As(err, &jsErr) || jsErr.Name != "Error" || jsErr.Message != c.Error {
					t.Errorf("Decode(_, %q) error = %#v; want Error %q", c.Name, err, c.Error)
				}
				if _, err := Encode("", c.Name, false); err == nil || err.Error() != c.Error {
					t.Errorf("Encode(_, %q) error = %v; want %q", c.Name, err, c.Error)
				}
				continue
			}
			if err != nil || decoded != c.Decoded {
				t.Errorf("Decode(probe, %q) = %+q, %v; want %+q", c.Name, decoded, err, c.Decoded)
			}
			encoded, err := Encode("Aé€日\U0001F600+-&", c.Name, false)
			if err != nil || hex.EncodeToString(encoded) != c.Encoded {
				t.Errorf("Encode(probe, %q) = %x, %v; want %s", c.Name, encoded, err, c.Encoded)
			}
		}
		t.Logf("%d names", len(fx.Names))
	})
}

// TestExhaustiveSweeps replays the sweeps of record.js: decoding every 1-
// and 2-byte buffer (plus selected 3-byte and all GB18030 4-byte buffers)
// and encoding every scalar value, each digested with SHA-256.
func TestExhaustiveSweeps(t *testing.T) {
	if testing.Short() {
		t.Skip("exhaustive sweeps skipped in -short mode")
	}
	var fx struct {
		Sweeps []sweep `json:"sweeps"`
	}
	loadFixture(t, "exhaustive.json", &fx)
	for _, s := range fx.Sweeps {
		t.Run(s.Enc+"/"+s.Sweep, func(t *testing.T) {
			t.Parallel()
			h := sha256.New()
			var count int
			var err error
			if s.Sweep == "encode" {
				count, err = sweepEncode(h, s.Enc, isDBCS(t, s.Enc))
			} else {
				count, err = sweepDecode(h, s.Enc, sweepBuffers(s.Enc, s.Sweep))
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := hex.EncodeToString(h.Sum(nil)); count != s.Count || got != s.SHA256 {
				t.Errorf("%d buffers, sha256 %s; want %d, %s", count, got, s.Count, s.SHA256)
			}
		})
	}
}

func isDBCS(t *testing.T, enc string) bool {
	c, err := getCodec(enc)
	if err != nil {
		t.Fatal(err)
	}
	_, ok := c.(*dbcsCodec)
	return ok
}

// sweepBuffers returns the buffer generator of a record.js decode sweep.
func sweepBuffers(enc, name string) func(emit func([]byte)) {
	triples := func(firsts ...byte) func(emit func([]byte)) {
		return func(emit func([]byte)) {
			for _, a := range firsts {
				for b := 0; b < 256; b++ {
					for c := 0; c < 256; c++ {
						emit([]byte{a, byte(b), byte(c)})
					}
				}
			}
		}
	}
	switch name {
	case "singles":
		return func(emit func([]byte)) {
			for a := 0; a < 256; a++ {
				emit([]byte{byte(a)})
			}
		}
	case "pairs":
		return func(emit func([]byte)) {
			for a := 0; a < 0x10000; a++ {
				emit([]byte{byte(a >> 8), byte(a)})
			}
		}
	case "gbquads":
		return func(emit func([]byte)) {
			for a := byte(0x81); a <= 0xFE; a++ {
				for b := byte(0x30); b <= 0x39; b++ {
					for c := byte(0x81); c <= 0xFE; c++ {
						for d := byte(0x30); d <= 0x39; d++ {
							emit([]byte{a, b, c, d})
						}
					}
				}
			}
		}
	}
	// "triples"
	if enc == "utf8" || enc == "cesu8" {
		return triples(0xE0, 0xED, 0xEF, 0xF0, 0xF4, 0xC2)
	}
	return triples(0x81, 0x8E, 0x8F, 0xA4, 0xC8, 0xFE)
}

func sweepDecode(h hash.Hash, enc string, bufs func(emit func([]byte))) (int, error) {
	var count int
	var firstErr error
	bufs(func(b []byte) {
		s, err := Decode(b, enc, true)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		h.Write([]byte(s))
		h.Write([]byte{0xFF})
		count++
	})
	return count, firstErr
}

func sweepEncode(h hash.Hash, enc string, fullPlanes12 bool) (int, error) {
	fullUpTo := rune(0x10000)
	if fullPlanes12 {
		fullUpTo = 0x30000
	}
	var count int
	for cp := rune(0); cp <= 0x10FFFF; {
		if cp < 0xD800 || cp >= 0xE000 {
			out, err := Encode(string(cp), enc, false)
			if err != nil {
				return count, err
			}
			h.Write(binary.BigEndian.AppendUint32(nil, uint32(len(out))))
			h.Write(out)
			count++
		}
		if cp < fullUpTo {
			cp++
		} else {
			cp += 97
		}
	}
	return count, nil
}
