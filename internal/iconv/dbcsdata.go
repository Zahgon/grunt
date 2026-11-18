package iconv

// Double-byte encodings of encodings/dbcs-data.js. Their aliases are in
// aliasTable.
var (
	// jisEncodeAdd maps U+00A5 YEN SIGN and U+203E OVERLINE to the
	// JIS X 0201 bytes 0x5C and 0x7E.
	jisEncodeAdd = map[int64]int64{0x00A5: 0x5C, 0x203E: 0x7E}

	shiftJISDef = &dbcsDef{
		tables:     []string{"shiftjis.json"},
		encodeAdd:  jisEncodeAdd,
		encodeSkip: []codeRange{{0xED40, 0xF940}},
	}
	eucJPDef = &dbcsDef{
		tables:    []string{"eucjp.json"},
		encodeAdd: jisEncodeAdd,
	}

	// cp936Def is Microsoft's GBK approximation, also used for GB2312.
	cp936Def = &dbcsDef{tables: []string{"cp936.json"}}
	gbkDef   = &dbcsDef{tables: []string{"cp936.json", "gbk-added.json"}}
	// gb18030Def is GBK plus the algorithmic four-byte ranges; 0x80 (€ in
	// CP936) decodes but € encodes as A2 E3.
	gb18030Def = &dbcsDef{
		tables:     []string{"cp936.json", "gbk-added.json"},
		gb18030:    true,
		encodeSkip: []codeRange{{0x80, 0x80}},
		encodeAdd:  map[int64]int64{0x20AC: 0xA2E3},
	}

	cp949Def = &dbcsDef{tables: []string{"cp949.json"}}
	cp950Def = &dbcsDef{tables: []string{"cp950.json"}}

	// big5HKSCSDef is the Encoding Standard's Big5 (CP950 plus HKSCS). The
	// skipped codes prefer regular Big5 over HKSCS duplicates, and the last
	// pointer for U+2550, U+255E, U+2561, U+256A, U+5341 and U+5345.
	big5HKSCSDef = &dbcsDef{
		tables:     []string{"cp950.json", "big5-added.json"},
		encodeSkip: codes(big5HKSCSSkip...),
	}
	big5HKSCSSkip = []int64{
		0x8e69, 0x8e6f, 0x8e7e, 0x8eab, 0x8eb4, 0x8ecd, 0x8ed0, 0x8f57, 0x8f69, 0x8f6e, 0x8fcb, 0x8ffe,
		0x906d, 0x907a, 0x90c4, 0x90dc, 0x90f1, 0x91bf, 0x92af, 0x92b0, 0x92b1, 0x92b2, 0x92d1, 0x9447, 0x94ca,
		0x95d9, 0x96fc, 0x9975, 0x9b76, 0x9b78, 0x9b7b, 0x9bc6, 0x9bde, 0x9bec, 0x9bf6, 0x9c42, 0x9c53, 0x9c62,
		0x9c68, 0x9c6b, 0x9c77, 0x9cbc, 0x9cbd, 0x9cd0, 0x9d57, 0x9d5a, 0x9dc4, 0x9def, 0x9dfb, 0x9ea9, 0x9eef,
		0x9efd, 0x9f60, 0x9fcb, 0xa077, 0xa0dc, 0xa0df, 0x8fcc, 0x92c8, 0x9644, 0x96ed,
		0xa2a4, 0xa2a5, 0xa2a7, 0xa2a6, 0xa2cc, 0xa2ce,
	}
)

// codes turns single codes into one-element ranges.
func codes(vals ...int64) []codeRange {
	r := make([]codeRange, len(vals))
	for i, v := range vals {
		r[i] = codeRange{v, v}
	}
	return r
}
