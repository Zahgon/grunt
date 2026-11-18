package iconv

import (
	"encoding/binary"
)

// utf16BECodec is Utf16BECodec (encodings/utf16.js).
type utf16BECodec struct{}

func newUTF16BECodec(codecOptions) (codec, error) { return utf16BECodec{}, nil }

func (utf16BECodec) bomAware() bool { return true }

// decode is Utf16BEDecoder: big-endian code units; an odd trailing byte is
// kept as overflowByte and discarded by end().
func (utf16BECodec) decode(b []byte, _ bool) (written, ended []uint16) {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.BigEndian.Uint16(b[2*i:])
	}
	return u, nil
}

// encode is Utf16BEEncoder: UCS-2 with each byte pair swapped.
func (utf16BECodec) encode(u []uint16) []byte {
	out := make([]byte, 2*len(u))
	for i, x := range u {
		binary.BigEndian.PutUint16(out[2*i:], x)
	}
	return out
}

// utf16AutoCodec is Utf16Codec (encodings/utf16.js): the decoder picks
// UTF-16LE or UTF-16BE from a BOM or a heuristic, the encoder writes
// UTF-16LE with a BOM.
type utf16AutoCodec struct {
	le, be codec
}

func newUTF16AutoCodec(codecOptions) (codec, error) {
	le, err := resolveCodec("utf-16le")
	if err != nil {
		return nil, err
	}
	be, err := resolveCodec("utf-16be")
	if err != nil {
		return nil, err
	}
	return &utf16AutoCodec{le: le, be: be}, nil
}

func (*utf16AutoCodec) bomAware() bool { return false }

// decode is Utf16Decoder. With fewer than 16 bytes the write() buffers
// everything and end() decodes it; either way the chosen decoder sees the
// whole input in one write, wrapped in StripBOM according to stripBOM.
func (c *utf16AutoCodec) decode(b []byte, stripBOM bool) (written, ended []uint16) {
	inner := c.le
	if detectUTF16BE(b) {
		inner = c.be
	}
	return decodeUnits(inner, b, stripBOM), nil
}

// encode is Utf16Encoder: options.addBOM defaults to true, so the result
// always starts with a UTF-16LE BOM.
func (c *utf16AutoCodec) encode(u []uint16) []byte {
	return encodeUnits(c.le, u, true)
}

// detectUTF16BE is detectEncoding in encodings/utf16.js: a BOM decides,
// otherwise the endianness under which more of the first 100 code units look
// like ASCII (a zero high byte) wins; ties default to UTF-16LE.
func detectUTF16BE(b []byte) bool {
	if len(b) >= 2 {
		if b[0] == 0xFF && b[1] == 0xFE {
			return false
		}
		if b[0] == 0xFE && b[1] == 0xFF {
			return true
		}
	}
	asciiLE, asciiBE := 0, 0
	for i := 0; i+1 < len(b) && i < 200; i += 2 {
		if b[i] == 0 && b[i+1] != 0 {
			asciiBE++
		}
		if b[i] != 0 && b[i+1] == 0 {
			asciiLE++
		}
	}
	return asciiBE > asciiLE
}
