package iconv

import (
	"encoding/base64"
	"strings"
)

// utf7Codec implements Utf7Codec (RFC 2152) and, with imap set,
// Utf7IMAPCodec (RFC 3501 modified UTF-7) from encodings/utf7.js.
type utf7Codec struct {
	imap bool
	// ascii and utf16be are the codecs iconv-lite's UTF-7 decoder calls
	// through iconv.decode(..., "ascii") and iconv.decode(..., "utf16-be").
	ascii, utf16be codec
}

func newUTF7Codec(codecOptions) (codec, error)     { return newUTF7(false) }
func newUTF7IMAPCodec(codecOptions) (codec, error) { return newUTF7(true) }

func newUTF7(imap bool) (codec, error) {
	ascii, err := resolveCodec("ascii")
	if err != nil {
		return nil, err
	}
	be, err := resolveCodec("utf16-be")
	if err != nil {
		return nil, err
	}
	return &utf7Codec{imap: imap, ascii: ascii, utf16be: be}, nil
}

func (*utf7Codec) bomAware() bool { return true }

// shift returns the character that opens a base64 section.
func (c *utf7Codec) shift() byte {
	if c.imap {
		return '&'
	}
	return '+'
}

// isBase64 reports whether b may appear inside a base64 section
// (base64Chars / base64IMAPChars in encodings/utf7.js).
func (c *utf7Codec) isBase64(b byte) bool {
	return 'A' <= b && b <= 'Z' || 'a' <= b && b <= 'z' || '0' <= b && b <= '9' ||
		b == '+' || b == '/' || (c.imap && b == ',')
}

// direct decodes a run of direct characters with the ascii codec.
func (c *utf7Codec) direct(b []byte) []uint16 {
	return decodeUnits(c.ascii, b, true)
}

// section returns the base64 text of a run of base64 characters; IMAP
// writes '/' as ','.
func (c *utf7Codec) section(b []byte) []uint16 {
	u := c.direct(b)
	if c.imap {
		for i, x := range u {
			if x == ',' {
				u[i] = '/'
			}
		}
	}
	return u
}

// unbase64 decodes base64 text to UTF-16BE through Node's base64 parser and
// iconv.decode(buf, "utf16-be"). That call passes no options, so StripBOM is
// active: a U+FEFF at the start of every base64 section is dropped,
// whatever the caller's stripBOM.
func (c *utf7Codec) unbase64(b64 []uint16) []uint16 {
	return decodeUnits(c.utf16be, nodeBase64Decode(b64), true)
}

// decode is Utf7Decoder / Utf7IMAPDecoder write() followed by end().
func (c *utf7Codec) decode(b []byte, _ bool) (written, ended []uint16) {
	var res, accum []uint16
	lastI, inBase64 := 0, false
	for i := 0; i < len(b); i++ {
		if !inBase64 {
			if b[i] == c.shift() {
				res = append(res, c.direct(b[lastI:i])...)
				lastI = i + 1
				inBase64 = true
			}
			continue
		}
		if c.isBase64(b[i]) {
			continue
		}
		// Base64 ended.
		if i == lastI && b[i] == '-' { // "+-" -> "+", "&-" -> "&"
			res = append(res, uint16(c.shift()))
		} else {
			res = append(res, c.unbase64(append(accum, c.section(b[lastI:i])...))...)
		}
		if b[i] != '-' { // The minus is absorbed; anything else is re-read as direct.
			i--
		}
		lastI = i + 1
		inBase64 = false
		accum = nil
	}
	if !inBase64 {
		res = append(res, c.direct(b[lastI:])...)
		return res, nil
	}
	// Still in base64 at the end of write(): decode whole 8-character
	// blocks now and keep the rest for end().
	b64 := append(accum, c.section(b[lastI:])...)
	whole := len(b64) - len(b64)%8
	res = append(res, c.unbase64(b64[:whole])...)
	if rest := b64[whole:]; len(rest) > 0 {
		ended = c.unbase64(rest)
	}
	return res, ended
}

// encode dispatches to the RFC 2152 or RFC 3501 encoder.
func (c *utf7Codec) encode(u []uint16) []byte {
	if c.imap {
		return encodeUTF7IMAP(u)
	}
	return encodeUTF7(u)
}

// isUTF7Direct reports whether a code unit is outside nonDirectChars, the
// pattern /[^A-Za-z0-9'\(\),-\.\/:\? \n\r\t]+/g; its ",-\." is a range that
// also covers '-'.
func isUTF7Direct(x uint16) bool {
	return 'A' <= x && x <= 'Z' || 'a' <= x && x <= 'z' || '0' <= x && x <= '9' ||
		strings.ContainsRune("'(),-./:? \n\r\t", rune(x))
}

// encodeUTF7 is Utf7Encoder: each maximal run of non-direct code units
// becomes "+" + unpadded base64 of its UTF-16BE bytes + "-"; a lone "+" run
// becomes "+-".
func encodeUTF7(u []uint16) []byte {
	var out []byte
	for i := 0; i < len(u); {
		if isUTF7Direct(u[i]) {
			out = append(out, byte(u[i]))
			i++
			continue
		}
		j := i
		for j < len(u) && !isUTF7Direct(u[j]) {
			j++
		}
		out = append(out, '+')
		if !(j-i == 1 && u[i] == '+') {
			out = append(out, base64.RawStdEncoding.EncodeToString(utf16BECodec{}.encode(u[i:j]))...)
		}
		out = append(out, '-')
		i = j
	}
	return out
}

// imapBase64 is base64 with ',' for '/' and no padding.
func imapBase64(b []byte) string {
	return strings.ReplaceAll(base64.RawStdEncoding.EncodeToString(b), "/", ",")
}

// encodeUTF7IMAP is Utf7IMAPEncoder write() + end(): printable ASCII is
// direct ('&' as "&-"); other code units are accumulated as UTF-16BE and
// flushed in base64 every 6 bytes, on return to direct mode, and at the end,
// which also closes an open section with '-'.
func encodeUTF7IMAP(u []uint16) []byte {
	var out, accum []byte
	inBase64 := false
	flush := func() {
		if len(accum) > 0 {
			out = append(out, imapBase64(accum)...)
			accum = accum[:0]
		}
	}
	for _, x := range u {
		if 0x20 <= x && x <= 0x7E {
			if inBase64 {
				flush()
				out = append(out, '-')
				inBase64 = false
			}
			out = append(out, byte(x))
			if x == '&' {
				out = append(out, '-')
			}
			continue
		}
		if !inBase64 {
			out = append(out, '&')
			inBase64 = true
		}
		accum = append(accum, byte(x>>8), byte(x))
		if len(accum) == 6 {
			flush()
		}
	}
	if inBase64 {
		flush()
		out = append(out, '-')
	}
	return out
}
