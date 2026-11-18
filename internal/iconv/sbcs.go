package iconv

import (
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// sbcsCodec is SBCSCodec (encodings/sbcs-codec.js): a 256-entry byte to
// code unit table and its 64K-entry inverse.
type sbcsCodec struct {
	decodeTable [256]uint16
	encodeTable []byte // indexed by code unit
}

func newSBCSCodec(opts codecOptions) (codec, error) {
	chars := jsstr.ToUTF16(opts.chars)
	if len(chars) != 128 && len(chars) != 256 {
		return nil, js.NewError("Encoding '_sbcs' has incorrect 'chars' (must be of len 128 or 256)")
	}
	c := &sbcsCodec{encodeTable: make([]byte, 0x10000)}
	if len(chars) == 128 { // The lower half is ASCII.
		for i := 0; i < 128; i++ {
			c.decodeTable[i] = uint16(i)
		}
		copy(c.decodeTable[128:], chars)
	} else {
		copy(c.decodeTable[:], chars)
	}
	for i := range c.encodeTable {
		c.encodeTable[i] = defaultCharSingleByte
	}
	// Later bytes win when a character appears twice: "ascii", whose upper
	// half is all U+FFFD, encodes U+FFFD as 0xFF.
	for i, x := range c.decodeTable {
		c.encodeTable[x] = byte(i)
	}
	return c, nil
}

func (*sbcsCodec) bomAware() bool { return false }

func (c *sbcsCodec) decode(b []byte, _ bool) (written, ended []uint16) {
	u := make([]uint16, len(b))
	for i, x := range b {
		u[i] = c.decodeTable[x]
	}
	return u, nil
}

// encode maps every code unit, so a supplementary character becomes two
// default characters.
func (c *sbcsCodec) encode(u []uint16) []byte {
	out := make([]byte, len(u))
	for i, x := range u {
		out[i] = c.encodeTable[x]
	}
	return out
}
