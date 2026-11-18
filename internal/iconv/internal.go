package iconv

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// internalCodec is InternalCodec (encodings/internal.js): Node's built-in
// Buffer encodings, decoded through string_decoder.StringDecoder.
type internalCodec struct {
	enc   string // encodingName: utf8, cesu8, ucs2, binary, base64 or hex
	isBOM bool
}

func newInternalCodec(opts codecOptions) (codec, error) {
	return &internalCodec{enc: opts.encodingName, isBOM: opts.bomAware}, nil
}

func (c *internalCodec) bomAware() bool { return c.isBOM }

// decode mirrors StringDecoder(enc).write(b) + end(). For utf8 and ucs2 the
// decoder holds back an incomplete trailing character until end(); that
// tail can only produce U+FFFD (utf8) or a lone high surrogate (ucs2), never
// a BOM, so returning everything as written leaves StripBOM unaffected.
func (c *internalCodec) decode(b []byte, _ bool) (written, ended []uint16) {
	switch c.enc {
	case "utf8":
		// Buffer#toString('utf8') decodes as the WHATWG decoder does.
		return jsstr.ToUTF16(js.DecodeUTF8(b)), nil
	case "cesu8":
		// Node >= 4 cannot decode CESU-8 (the probe in InternalCodec finds
		// Buffer.from('eda0bdedb2a9', 'hex').toString() !== '💩'), so
		// iconv-lite installs InternalDecoderCesu8.
		return decodeCESU8(b)
	case "ucs2":
		return decodeUCS2(b), nil
	case "binary":
		return latin1Units(b), nil
	case "base64":
		// The base64 StringDecoder emits whole 3-byte groups on write and the
		// padded remainder on end: standard padded base64 overall.
		return latin1Units([]byte(base64.StdEncoding.EncodeToString(b))), nil
	default: // hex
		return latin1Units([]byte(hex.EncodeToString(b))), nil
	}
}

// encode mirrors Buffer.from(str, enc), with the base64 and cesu8 encoders
// of encodings/internal.js.
func (c *internalCodec) encode(u []uint16) []byte {
	switch c.enc {
	case "utf8":
		// Buffer.from(str, 'utf8') writes lone surrogates as EF BF BD,
		// exactly what U+FFFD encodes to.
		return []byte(jsstr.FromUTF16WellFormed(u))
	case "cesu8":
		return encodeCESU8(u)
	case "ucs2":
		return encodeUCS2(u)
	case "binary":
		out := make([]byte, len(u))
		for i, x := range u {
			out[i] = byte(x) // Node's latin1 writer keeps the low byte.
		}
		return out
	case "base64":
		// InternalEncoderBase64 decodes the complete quads on write() and
		// the 0-3 leftover code units on end(), separately.
		complete := len(u) - len(u)%4
		return append(nodeBase64Decode(u[:complete]), nodeBase64Decode(u[complete:])...)
	default: // hex
		return nodeHexDecode(u)
	}
}

// latin1Units widens bytes to code units (Buffer#toString('latin1')).
func latin1Units(b []byte) []uint16 {
	u := make([]uint16, len(b))
	for i, x := range b {
		u[i] = uint16(x)
	}
	return u
}

// decodeUCS2 is Buffer#toString('ucs2'): little-endian code units, with an
// odd trailing byte ignored (StringDecoder::FlushData drops it).
func decodeUCS2(b []byte) []uint16 {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return u
}

// encodeUCS2 is Buffer.from(str, 'ucs2').
func encodeUCS2(u []uint16) []byte {
	out := make([]byte, 2*len(u))
	for i, x := range u {
		binary.LittleEndian.PutUint16(out[2*i:], x)
	}
	return out
}

// decodeCESU8 is InternalDecoderCesu8.write + end. It works per code unit,
// so surrogate halves encoded separately recombine in the result.
func decodeCESU8(b []byte) (written, ended []uint16) {
	var acc uint32
	contBytes, accBytes := 0, 0
	for _, cur := range b {
		if cur&0xC0 != 0x80 { // Leading byte.
			if contBytes > 0 { // Previous code is invalid.
				written = append(written, defaultCharUnicode)
				contBytes = 0
			}
			switch {
			case cur < 0x80:
				written = append(written, uint16(cur))
			case cur < 0xE0:
				acc, contBytes, accBytes = uint32(cur&0x1F), 1, 1
			case cur < 0xF0:
				acc, contBytes, accBytes = uint32(cur&0x0F), 2, 1
			default: // Four or more bytes are not supported by CESU-8.
				written = append(written, defaultCharUnicode)
			}
			continue
		}
		if contBytes == 0 { // Unexpected continuation byte.
			written = append(written, defaultCharUnicode)
			continue
		}
		acc = acc<<6 | uint32(cur&0x3F)
		contBytes--
		accBytes++
		if contBytes > 0 {
			continue
		}
		// Overlong encodings are rejected, but Modified UTF-8's C0 80 for
		// NUL is accepted.
		if (accBytes == 2 && acc < 0x80 && acc > 0) || (accBytes == 3 && acc < 0x800) {
			written = append(written, defaultCharUnicode)
		} else {
			written = append(written, uint16(acc)) // String.fromCharCode truncates to 16 bits.
		}
	}
	if contBytes > 0 {
		// InternalDecoderCesu8.prototype.end starts from `var res = 0` and
		// appends the default char, so an unfinished sequence yields the
		// string "0�", which iconv.decode appends.
		ended = []uint16{'0', defaultCharUnicode}
	}
	return written, ended
}

// encodeCESU8 is InternalEncoderCesu8: every code unit, surrogates included,
// becomes a 1-3 byte UTF-8-style sequence.
func encodeCESU8(u []uint16) []byte {
	out := make([]byte, 0, 3*len(u))
	for _, c := range u {
		switch {
		case c < 0x80:
			out = append(out, byte(c))
		case c < 0x800:
			out = append(out, byte(0xC0+c>>6), byte(0x80+c&0x3F))
		default:
			out = append(out, byte(0xE0+c>>12), byte(0x80+(c>>6)&0x3F), byte(0x80+c&0x3F))
		}
	}
	return out
}

// unbase64 maps a byte to its base64 value (both the standard and URL-safe
// alphabets, as Node's decoder accepts), or 0xFF.
var unbase64 = func() (t [256]byte) {
	for i := range t {
		t[i] = 0xFF
	}
	const std = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	for i := 0; i < len(std); i++ {
		t[std[i]] = byte(i)
	}
	t['-'], t['_'] = 62, 63
	return t
}()

// nodeBase64Decode is Buffer.from(str, 'base64') in Node.js 24.
//
// Node first tries simdutf's WHATWG forgiving-base64 decoder and falls back
// to nbytes::Base64Decode when it rejects the input. Whenever the forgiving
// decoder accepts, the fallback produces the same bytes, so only the
// fallback is implemented. Its quirks, all observable:
//   - every code unit is truncated to 8 bits before lookup, so U+0141 'Ł'
//     reads as 'A' and U+013D reads as '=';
//   - characters outside the alphabet are skipped, and decoding stops at the
//     first '=' (or at the end) inside a group;
//   - after the main loop only one more group is attempted;
//   - the output is capped at the length lib/buffer.js allocates,
//     base64ByteLength.
func nodeBase64Decode(src []uint16) []byte {
	dstLen := base64ByteLength(src)
	dst := make([]byte, 0, dstLen)
	srcLen := len(src)
	available := min(dstLen, base64DecodedSize(src))
	maxK := available / 3 * 3
	maxI := srcLen / 4 * 4
	i := 0
	for i < maxI && len(dst) < maxK {
		a, b := unbase64[byte(src[i])], unbase64[byte(src[i+1])]
		c, d := unbase64[byte(src[i+2])], unbase64[byte(src[i+3])]
		if (a|b|c|d)&0x80 != 0 { // Whitespace or not valid base64.
			var ok bool
			if dst, ok = base64DecodeGroupSlow(dst, dstLen, src, &i); !ok {
				return dst
			}
			maxI = i + (srcLen-i)/4*4
			continue
		}
		dst = append(dst, a<<2|b>>4, b<<4|c>>2, c<<6|d)
		i += 4
	}
	if i < srcLen && len(dst) < dstLen {
		dst, _ = base64DecodeGroupSlow(dst, dstLen, src, &i)
	}
	return dst
}

// base64DecodeGroupSlow is nbytes::Base64DecodeGroupSlow: it decodes one
// group of four alphabet characters starting at *i, skipping anything else.
// It reports false when decoding must stop.
func base64DecodeGroupSlow(dst []byte, dstLen int, src []uint16, i *int) ([]byte, bool) {
	var hi byte
	for n := 0; n < 4; n++ {
		var lo byte
		for {
			c := byte(src[*i])
			lo = unbase64[c]
			*i++
			if lo < 64 {
				break
			}
			if c == '=' || *i >= len(src) {
				return dst, false
			}
		}
		switch n {
		case 1:
			dst = append(dst, hi<<2|(lo&0x30)>>4)
		case 2:
			dst = append(dst, hi<<4|(lo&0x3C)>>2)
		case 3:
			dst = append(dst, hi<<6|lo)
		}
		if *i >= len(src) || len(dst) >= dstLen {
			return dst, false
		}
		hi = lo
	}
	return dst, true
}

// base64ByteLength is base64ByteLength in lib/buffer.js: the allocation size.
func base64ByteLength(src []uint16) int {
	n := len(src)
	if n > 0 && src[n-1] == '=' {
		n--
	}
	if n > 1 && src[n-1] == '=' {
		n--
	}
	return n * 3 >> 2
}

// base64DecodedSize is nbytes::Base64DecodedSize (compares whole code units).
func base64DecodedSize(src []uint16) int {
	n := len(src)
	if n < 2 {
		return 0
	}
	if src[n-1] == '=' {
		n--
		if src[n-1] == '=' {
			n--
		}
	}
	if n < 2 {
		return 0
	}
	return n/4*3 + (n%4+1)/2
}

// nodeHexDecode is Buffer.from(str, 'hex'): pairs of hex digits up to the
// first invalid pair; an odd last digit is ignored. As in nbytes::HexDecode
// each code unit is truncated to 8 bits first.
func nodeHexDecode(src []uint16) []byte {
	out := make([]byte, 0, len(src)/2)
	for i := 0; i+1 < len(src); i += 2 {
		hi, lo := jsstr.HexDigitValue(rune(byte(src[i]))), jsstr.HexDigitValue(rune(byte(src[i+1])))
		if hi < 0 || lo < 0 {
			break
		}
		out = append(out, byte(hi<<4|lo))
	}
	return out
}
