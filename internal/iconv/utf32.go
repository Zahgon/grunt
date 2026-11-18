package iconv

import (
	"encoding/binary"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
)

// utf32Codec is Utf32Codec (encodings/utf32.js), registered as utf32le and
// utf32be.
type utf32Codec struct {
	order byteOrder
}

// byteOrder is implemented by binary.LittleEndian and binary.BigEndian.
type byteOrder interface {
	binary.ByteOrder
	binary.AppendByteOrder
}

func newUTF32Codec(opts codecOptions) (codec, error) {
	if opts.isLE {
		return utf32Codec{order: binary.LittleEndian}, nil
	}
	return utf32Codec{order: binary.BigEndian}, nil
}

func (utf32Codec) bomAware() bool { return true }

// decode is Utf32Decoder: values above U+10FFFF (including those that are
// negative as int32) become U+FFFD; surrogate code points are passed through
// as code units, so a high/low pair of UTF-32 units recombines. 1-3
// trailing bytes stay in the overflow buffer and are discarded by end().
func (c utf32Codec) decode(b []byte, _ bool) (written, ended []uint16) {
	u := make([]uint16, 0, len(b)/4)
	for i := 0; i+4 <= len(b); i += 4 {
		cp := c.order.Uint32(b[i:])
		if cp > 0x10FFFF {
			cp = defaultCharUnicode
		}
		u = jsstr.AppendCodePoint(u, rune(cp))
	}
	return u, nil
}

// encode is Utf32Encoder: surrogate pairs are combined; an unpaired
// surrogate is written as a stand-alone value.
func (c utf32Codec) encode(u []uint16) []byte {
	out := make([]byte, 0, 4*len(u))
	put := func(v uint32) { out = c.order.AppendUint32(out, v) }
	high := uint16(0)
	for _, code := range u {
		isHigh := 0xD800 <= code && code < 0xDC00
		isLow := 0xDC00 <= code && code < 0xE000
		if high != 0 {
			if isHigh || !isLow {
				put(uint32(high))
			} else {
				put((uint32(high-0xD800)<<10 | uint32(code-0xDC00)) + 0x10000)
				high = 0
				continue
			}
		}
		if isHigh {
			high = code
		} else {
			put(uint32(code))
			high = 0
		}
	}
	if high != 0 { // Utf32Encoder.prototype.end
		put(uint32(high))
	}
	return out
}

// utf32AutoCodec is Utf32AutoCodec (encodings/utf32.js), registered as utf32.
type utf32AutoCodec struct {
	le, be codec
}

func newUTF32AutoCodec(codecOptions) (codec, error) {
	le, err := resolveCodec("utf-32le")
	if err != nil {
		return nil, err
	}
	be, err := resolveCodec("utf-32be")
	if err != nil {
		return nil, err
	}
	return &utf32AutoCodec{le: le, be: be}, nil
}

func (*utf32AutoCodec) bomAware() bool { return false }

// decode is Utf32AutoDecoder; like utf16, the chosen decoder sees the whole
// input in one write whether or not the 32-byte threshold was reached.
func (c *utf32AutoCodec) decode(b []byte, stripBOM bool) (written, ended []uint16) {
	inner := c.le
	if detectUTF32BE(b) {
		inner = c.be
	}
	return decodeUnits(inner, b, stripBOM), nil
}

// encode is Utf32AutoEncoder: UTF-32LE (the defaultEncoding) with a BOM,
// since an absent addBOM option defaults to true.
func (c *utf32AutoCodec) encode(u []uint16) []byte {
	return encodeUnits(c.le, u, true)
}

// detectUTF32BE is detectEncoding in encodings/utf32.js: a BOM decides,
// otherwise the first 100 units are scored (BMP-looking values minus
// impossible values) under both byte orders; ties default to UTF-32LE.
func detectUTF32BE(b []byte) bool {
	if len(b) >= 4 {
		if b[0] == 0xFF && b[1] == 0xFE && b[2] == 0 && b[3] == 0 {
			return false
		}
		if b[0] == 0 && b[1] == 0 && b[2] == 0xFE && b[3] == 0xFF {
			return true
		}
	}
	invalidLE, invalidBE, bmpLE, bmpBE := 0, 0, 0, 0
	for i := 0; i+3 < len(b) && i < 400; i += 4 {
		q := b[i : i+4]
		if q[0] != 0 || q[1] > 0x10 {
			invalidBE++
		}
		if q[3] != 0 || q[2] > 0x10 {
			invalidLE++
		}
		if q[0] == 0 && q[1] == 0 && (q[2] != 0 || q[3] != 0) {
			bmpBE++
		}
		if (q[0] != 0 || q[1] != 0) && q[2] == 0 && q[3] == 0 {
			bmpLE++
		}
	}
	return bmpBE-invalidBE > bmpLE-invalidLE
}
