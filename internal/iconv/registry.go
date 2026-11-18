package iconv

// codecDefs holds the entries of iconv-lite's merged encoding table
// (encodings/index.js) that are neither aliases nor single-byte tables; those
// two kinds are generated into aliasTable and sbcsTable. The key set must
// equal generatedCodecNames, which the tests check.
//
// It is assigned in init because some constructors resolve other codecs
// through resolveCodec, which reads codecDefs: a package-level initializer
// would form an initialization cycle.
var codecDefs map[string]codecDef

func init() {
	codecDefs = map[string]codecDef{
		// encodings/internal.js
		"utf8":      {typ: "_internal", opts: codecOptions{bomAware: true}},
		"cesu8":     {typ: "_internal", opts: codecOptions{bomAware: true}},
		"ucs2":      {typ: "_internal", opts: codecOptions{bomAware: true}},
		"binary":    {typ: "_internal"},
		"base64":    {typ: "_internal"},
		"hex":       {typ: "_internal"},
		"_internal": {build: newInternalCodec},

		// encodings/utf32.js
		"_utf32":  {build: newUTF32Codec},
		"utf32le": {typ: "_utf32", opts: codecOptions{isLE: true}},
		"utf32be": {typ: "_utf32"},
		"utf32":   {build: newUTF32AutoCodec},

		// encodings/utf16.js
		"utf16be": {build: newUTF16BECodec},
		"utf16":   {build: newUTF16AutoCodec},

		// encodings/utf7.js
		"utf7":     {build: newUTF7Codec},
		"utf7imap": {build: newUTF7IMAPCodec},

		// encodings/sbcs-codec.js
		"_sbcs": {build: newSBCSCodec},

		// encodings/dbcs-codec.js and dbcs-data.js
		"_dbcs":     {build: newDBCSCodec},
		"shiftjis":  {typ: "_dbcs", opts: codecOptions{dbcs: shiftJISDef}},
		"eucjp":     {typ: "_dbcs", opts: codecOptions{dbcs: eucJPDef}},
		"cp936":     {typ: "_dbcs", opts: codecOptions{dbcs: cp936Def}},
		"gbk":       {typ: "_dbcs", opts: codecOptions{dbcs: gbkDef}},
		"gb18030":   {typ: "_dbcs", opts: codecOptions{dbcs: gb18030Def}},
		"cp949":     {typ: "_dbcs", opts: codecOptions{dbcs: cp949Def}},
		"cp950":     {typ: "_dbcs", opts: codecOptions{dbcs: cp950Def}},
		"big5hkscs": {typ: "_dbcs", opts: codecOptions{dbcs: big5HKSCSDef}},
	}
}
