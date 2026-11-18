// Package iconv is a port of iconv-lite 0.6.3, the character-encoding library
// grunt uses in grunt.file.read (iconv.decode) and grunt.file.write
// (iconv.encode).
//
// Every codec of iconv-lite is implemented: Node's internal encodings (utf8,
// cesu8, ucs2/utf16le, binary, base64, hex), utf16be, utf16, utf32le/be,
// utf32, utf7, utf7imap, all single-byte tables and all double-byte tables
// (Shift_JIS, EUC-JP, CP936/GBK/GB18030, CP949, CP950/Big5-HKSCS), together
// with the full alias table and iconv-lite's name canonicalization.
//
// Codecs work on UTF-16 code units, as JavaScript strings do, so that
// surrogate handling, unmappable-character substitution and output lengths
// follow the original byte for byte. Two conversions sit at the boundary:
//
//   - Encode converts its Go string to UTF-16 first; each invalid UTF-8 byte
//     becomes U+FFFD. (A JavaScript string can hold lone surrogates, a valid
//     Go string cannot.)
//   - Decode converts the decoded UTF-16 back to a Go string; each lone
//     surrogate that iconv-lite would have produced becomes U+FFFD.
//
// Where iconv-lite delegates to Node's Buffer and string_decoder (the
// "internal" codecs and the base64/hex helpers used by utf7), the behaviour
// recorded from Node.js 24 is reproduced: WHATWG replacement for malformed
// UTF-8, odd trailing bytes dropped by UCS-2, Node's lenient base64 and hex
// parsers.
package iconv

//go:generate go run ./gen -iconv-lite $ICONV_LITE_DIR

import (
	"regexp"
	"strings"
	"sync"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

const (
	// bomChar is the byte order mark, BOMChar in lib/bom-handling.js.
	bomChar = 0xFEFF
	// defaultCharUnicode is iconv.defaultCharUnicode, emitted for undecodable input.
	defaultCharUnicode = 0xFFFD
	// defaultCharSingleByte is iconv.defaultCharSingleByte, emitted for
	// unencodable characters.
	defaultCharSingleByte = '?'
)

// codec is an instantiated encoding, the object iconv.getCodec returns.
type codec interface {
	// decode runs the codec's decoder over b as iconv.decode does: a single
	// write(b) followed by end(). The two results are returned separately
	// because StripBOM (lib/bom-handling.js) only inspects the first one.
	// stripBOM is the caller's option, forwarded by the codecs that delegate
	// to another decoder (utf16, utf32).
	decode(b []byte, stripBOM bool) (written, ended []uint16)
	// encode runs the encoder over u: write(u) followed by end().
	encode(u []uint16) []byte
	// bomAware reports whether iconv.getDecoder/getEncoder wrap this codec in
	// StripBOM/PrependBOM.
	bomAware() bool
}

// Decode converts b from the named encoding to a string, as
// iconv.decode(b, encoding, {stripBOM: stripBOM}) does. With stripBOM, a
// leading U+FEFF is removed for the BOM-aware codecs (utf8, cesu8, ucs2,
// utf16be, utf32le/be, utf7, utf7imap; utf16 and utf32 forward the option to
// the endianness they detect).
func Decode(b []byte, encoding string, stripBOM bool) (string, error) {
	c, err := getCodec(encoding)
	if err != nil {
		return "", err
	}
	return jsstr.FromUTF16WellFormed(decodeUnits(c, b, stripBOM)), nil
}

// Encode converts s to the named encoding. addBOM=false is the call grunt
// makes, iconv.encode(s, encoding) without options; addBOM=true is
// iconv.encode(s, encoding, {addBOM: true}), which prepends U+FEFF for the
// BOM-aware codecs. utf16 and utf32 emit a BOM in both cases, because they
// default an absent addBOM option to true (encodings/utf16.js, utf32.js).
func Encode(s string, encoding string, addBOM bool) ([]byte, error) {
	c, err := getCodec(encoding)
	if err != nil {
		return nil, err
	}
	return encodeUnits(c, jsstr.ToUTF16(s), addBOM), nil
}

// EncodingExists reports whether name resolves to a codec, as
// iconv.encodingExists does.
func EncodingExists(name string) bool {
	_, err := getCodec(name)
	return err == nil
}

// decodeUnits is iconv.getDecoder plus a single write/end pass.
func decodeUnits(c codec, b []byte, stripBOM bool) []uint16 {
	written, ended := c.decode(b, stripBOM)
	// StripBOMWrapper.prototype.write: only a non-empty first write result
	// is inspected.
	if c.bomAware() && stripBOM && len(written) > 0 && written[0] == bomChar {
		written = written[1:]
	}
	return append(written, ended...)
}

// encodeUnits is iconv.getEncoder plus a single write/end pass.
func encodeUnits(c codec, u []uint16, addBOM bool) []byte {
	if c.bomAware() && addBOM {
		u = append([]uint16{bomChar}, u...)
	}
	return c.encode(u)
}

// codecOptions accumulates the options of the object entries met while
// resolving a name (the codecOptions object of iconv.getCodec).
type codecOptions struct {
	encodingName string
	bomAware     bool
	isLE         bool
	chars        string
	dbcs         *dbcsDef
}

// codecDef is a non-alias entry of the encoding table: either an option
// object ({type: ..., ...}) or a codec constructor.
type codecDef struct {
	// typ is the name an option object forwards to; empty for constructors.
	typ string
	// opts are the fields an option object sets.
	opts codecOptions
	// build is the codec constructor.
	build func(opts codecOptions) (codec, error)
}

var (
	cacheMu    sync.Mutex
	codecCache = map[string]codec{} // iconv._codecDataCache
)

// getCodec resolves an encoding name to its codec, as iconv.getCodec does.
func getCodec(encoding string) (codec, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	return resolveCodec(encoding)
}

// resolveCodec is getCodec with cacheMu already held. Codec constructors that
// look up another codec (utf16, utf32, utf7) call it directly.
func resolveCodec(encoding string) (codec, error) {
	enc := canonicalize(encoding)
	var opts codecOptions
	for {
		if c, ok := codecCache[enc]; ok {
			return c, nil
		}
		if target, ok := aliasTable[enc]; ok {
			enc = target
			continue
		}
		if chars, ok := sbcsTable[enc]; ok {
			opts.chars = chars
			if opts.encodingName == "" {
				opts.encodingName = enc
			}
			enc = "_sbcs"
			continue
		}
		def, ok := codecDefs[enc]
		if !ok {
			return nil, js.NewError("Encoding not recognized: '" + encoding + "' (searched as: '" + enc + "')")
		}
		if opts.encodingName == "" {
			opts.encodingName = enc
		}
		if def.build == nil {
			opts.merge(def.opts)
			enc = def.typ
			continue
		}
		c, err := def.build(opts)
		if err != nil {
			return nil, err
		}
		codecCache[opts.encodingName] = c
		return c, nil
	}
}

// merge copies the fields an option object sets, like the
// `for (var key in codecDef)` loop of iconv.getCodec.
func (o *codecOptions) merge(from codecOptions) {
	if from.bomAware {
		o.bomAware = true
	}
	if from.isLE {
		o.isLE = true
	}
	if from.chars != "" {
		o.chars = from.chars
	}
	if from.dbcs != nil {
		o.dbcs = from.dbcs
	}
}

// yearSuffix is the `:\d{4}$` half of the _canonicalizeEncoding pattern.
var yearSuffix = regexp.MustCompile(`:[0-9]{4}$`)

// canonicalize is iconv._canonicalizeEncoding: lowercase, drop a trailing
// ":NNNN" year, then drop everything but [0-9a-z]. Lowercasing before the
// filter means U+0130 and U+212A (KELVIN SIGN) survive as "i" and "k", as
// with String.prototype.toLowerCase.
func canonicalize(encoding string) string {
	s := yearSuffix.ReplaceAllString(strings.ToLower(encoding), "")
	return strings.Map(func(r rune) rune {
		if ('0' <= r && r <= '9') || ('a' <= r && r <= 'z') {
			return r
		}
		return -1
	}, s)
}
