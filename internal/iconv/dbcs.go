package iconv

import (
	"embed"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// tableFS holds iconv-lite's double-byte tables (encodings/tables), copied
// verbatim by go generate.
//
//go:embed tables/*.json
var tableFS embed.FS

// Trie and encode-table markers of encodings/dbcs-codec.js.
const (
	unassigned  = -1    // UNASSIGNED: unknown or unassigned sequence
	gb18030Code = -2    // GB18030_CODE: end of a GB18030 four-byte sequence
	seqMark     = -10   // SEQ_START: values <= seqMark index a sequence
	nodeMark    = -1000 // NODE_START: values <= nodeMark index a trie node
	defChar     = -1    // DEF_CHAR: key of a sequence's single-char fallback
)

// dbcsDef is the option object of a double-byte entry in dbcs-data.js.
type dbcsDef struct {
	// tables are the files of tableFS concatenated into the mapping table.
	tables []string
	// gb18030 enables the four-byte GB18030 ranges.
	gb18030 bool
	// encodeAdd maps extra characters to codes (applied after the decode
	// tables, only where no mapping exists).
	encodeAdd map[int64]int64
	// encodeSkip lists codes that decode but are not used for encoding.
	encodeSkip []codeRange
}

// codeRange is an inclusive range of multi-byte codes.
type codeRange struct{ from, to int64 }

// seqNode is an encodeTableSeq object: keys are code points, or defChar for
// the code of the sequence prefix on its own.
type seqNode map[int64]seqValue

// seqValue is a seqNode value: a code, or a deeper node when node != nil.
type seqValue struct {
	code int64
	node seqNode
}

// gb18030Ranges is tables/gb18030-ranges.json.
type gb18030Ranges struct {
	UChars  []int64 `json:"uChars"`
	GBChars []int64 `json:"gbChars"`
}

// dbcsCodec is DBCSCodec (encodings/dbcs-codec.js).
type dbcsCodec struct {
	name string

	// decodeTables is the decoding trie; node 0 is the root. Values: >= 0 a
	// code point, unassigned, gb18030Code, <= seqMark an index into
	// decodeTableSeq, <= nodeMark a child node.
	decodeTables   []*[256]int64
	decodeTableSeq [][]uint16
	gb18030        *gb18030Ranges

	// encodeTable maps code points to codes in buckets of 256 (nil when
	// empty); values <= seqMark index encodeTableSeq.
	encodeTable    []*[256]int64
	encodeTableSeq []seqNode
	defCharSB      int64
}

func newDBCSCodec(opts codecOptions) (codec, error) {
	def := opts.dbcs
	if def == nil || len(def.tables) == 0 {
		return nil, js.NewError("Encoding '" + opts.encodingName + "' has no data.")
	}
	c := &dbcsCodec{name: opts.encodingName, encodeTable: make([]*[256]int64, 0x110000>>8)}
	c.decodeTables = []*[256]int64{newNode()}

	for _, file := range def.tables {
		var chunks [][]any
		if err := loadTable(file, &chunks); err != nil {
			return nil, err
		}
		for _, chunk := range chunks {
			if err := c.addDecodeChunk(chunk); err != nil {
				return nil, err
			}
		}
	}

	if def.gb18030 {
		c.gb18030 = new(gb18030Ranges)
		if err := loadTable("gb18030-ranges.json", c.gb18030); err != nil {
			return nil, err
		}
		if err := c.addGB18030Nodes(); err != nil {
			return nil, err
		}
	}

	skip := map[int64]bool{}
	for _, r := range def.encodeSkip {
		for v := r.from; v <= r.to; v++ {
			skip[v] = true
		}
	}
	c.fillEncodeTable(0, 0, skip)
	for uChar, code := range def.encodeAdd {
		c.setEncodeChar(uChar, code)
	}

	// defCharSB: the code of '?'. iconv-lite's second fallback reads
	// encodeTable[0]['?'] (always undefined) and its third is '?' itself;
	// every table maps '?', so only the first lookup matters.
	c.defCharSB = defaultCharSingleByte
	if b := c.encodeTable[0]; b != nil && b[defaultCharSingleByte] != unassigned {
		c.defCharSB = b[defaultCharSingleByte]
	}
	return c, nil
}

// loadTable decodes an embedded JSON table.
func loadTable(file string, v any) error {
	data, err := tableFS.ReadFile("tables/" + file)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// newNode returns a trie node (or encode bucket) with every entry unassigned.
func newNode() *[256]int64 {
	n := new([256]int64)
	for i := range n {
		n[i] = unassigned
	}
	return n
}

func (*dbcsCodec) bomAware() bool { return false }

// getDecodeTrieNode is _getDecodeTrieNode: it returns the node holding the
// last byte of addr, creating intermediate nodes.
func (c *dbcsCodec) getDecodeTrieNode(addr int64) (*[256]int64, error) {
	var bytes []int64
	for a := addr; a > 0; a >>= 8 {
		bytes = append(bytes, a&0xFF)
	}
	if len(bytes) == 0 {
		bytes = append(bytes, 0)
	}
	node := c.decodeTables[0]
	for i := len(bytes) - 1; i > 0; i-- {
		switch val := node[bytes[i]]; {
		case val == unassigned:
			node[bytes[i]] = nodeMark - int64(len(c.decodeTables))
			node = newNode()
			c.decodeTables = append(c.decodeTables, node)
		case val <= nodeMark:
			node = c.decodeTables[nodeMark-val]
		default:
			return nil, js.NewError("Overwrite byte in " + c.name + ", addr: " + strconv.FormatInt(addr, 16))
		}
	}
	return node, nil
}

// addDecodeChunk is _addDecodeChunk. A chunk is [hexAddress, part...]
// where a string part lists characters for consecutive codes (surrogate
// pairs combine; U+0FF1..U+0FFF introduce a multi-character sequence of
// 0xFFF-code+2 units) and a number part continues the previous character
// with that many consecutive code points.
func (c *dbcsCodec) addDecodeChunk(chunk []any) error {
	if len(chunk) == 0 {
		return js.NewError("Empty chunk in " + c.name)
	}
	head, _ := chunk[0].(string)
	addr, err := strconv.ParseInt(head, 16, 64)
	if err != nil {
		return js.NewError(fmt.Sprintf("Incorrect chunk address %v in %s", chunk[0], c.name))
	}
	table, err := c.getDecodeTrieNode(addr)
	if err != nil {
		return err
	}
	tooLong := js.NewError("Incorrect chunk in " + c.name + " at addr " + head + ": too long")
	cur := addr & 0xFF
	write := func(v int64) error {
		if cur > 0xFF { // The original writes past the node and throws afterwards.
			return tooLong
		}
		table[cur] = v
		cur++
		return nil
	}
	for _, part := range chunk[1:] {
		switch p := part.(type) {
		case string:
			u := jsstr.ToUTF16(p)
			for l := 0; l < len(u); {
				code := u[l]
				l++
				var v int64
				switch {
				case 0xD800 <= code && code < 0xDC00: // Surrogate pair.
					if l >= len(u) || u[l] < 0xDC00 || u[l] >= 0xE000 {
						return js.NewError("Incorrect surrogate pair in " + c.name + " at chunk " + head)
					}
					v = 0x10000 + int64(code-0xD800)*0x400 + int64(u[l]-0xDC00)
					l++
				case 0x0FF0 < code && code <= 0x0FFF: // Character sequence.
					n := int(0xFFF-code) + 2
					if l+n > len(u) {
						return js.NewError("Incorrect sequence in " + c.name + " at chunk " + head)
					}
					v = seqMark - int64(len(c.decodeTableSeq))
					c.decodeTableSeq = append(c.decodeTableSeq, append([]uint16(nil), u[l:l+n]...))
					l += n
				default:
					v = int64(code)
				}
				if err := write(v); err != nil {
					return err
				}
			}
		case float64:
			if cur == 0 {
				return js.NewError("Incorrect chunk in " + c.name + " at addr " + head + ": count without a character")
			}
			if cur > 0xFF {
				return tooLong
			}
			next := table[cur-1] + 1
			for n := 0; n < int(p); n++ {
				if err := write(next); err != nil {
					return err
				}
				next++
			}
		default:
			return js.NewError(fmt.Sprintf("Incorrect type '%T' given in %s at chunk %s", part, c.name, head))
		}
	}
	// As in the original, a chunk may not run up to 0xFF itself.
	if cur > 0xFF {
		return tooLong
	}
	return nil
}

// addGB18030Nodes adds the shared third- and fourth-byte nodes through
// which every 0x81-0xFE 0x30-0x39 0x81-0xFE 0x30-0x39 sequence reaches
// gb18030Code.
func (c *dbcsCodec) addGB18030Nodes() error {
	third := int64(len(c.decodeTables))
	c.decodeTables = append(c.decodeTables, newNode())
	fourth := int64(len(c.decodeTables))
	c.decodeTables = append(c.decodeTables, newNode())

	first := c.decodeTables[0]
	for i := 0x81; i <= 0xFE; i++ {
		if first[i] > nodeMark {
			return js.NewError("gb18030 decode tables conflict at byte 1")
		}
		second := c.decodeTables[nodeMark-first[i]]
		for j := 0x30; j <= 0x39; j++ {
			if second[j] == unassigned {
				second[j] = nodeMark - third
			} else if second[j] > nodeMark {
				return js.NewError("gb18030 decode tables conflict at byte 2")
			}
			thirdNode := c.decodeTables[nodeMark-second[j]]
			for k := 0x81; k <= 0xFE; k++ {
				switch {
				case thirdNode[k] == unassigned:
					thirdNode[k] = nodeMark - fourth
				case thirdNode[k] == nodeMark-fourth:
					continue
				case thirdNode[k] > nodeMark:
					return js.NewError("gb18030 decode tables conflict at byte 3")
				}
				fourthNode := c.decodeTables[nodeMark-thirdNode[k]]
				for l := 0x30; l <= 0x39; l++ {
					if fourthNode[l] == unassigned {
						fourthNode[l] = gb18030Code
					}
				}
			}
		}
	}
	return nil
}

// encodeBucket is _getEncodeBucket.
func (c *dbcsCodec) encodeBucket(uCode int64) *[256]int64 {
	high := uCode >> 8
	if c.encodeTable[high] == nil {
		c.encodeTable[high] = newNode()
	}
	return c.encodeTable[high]
}

// setEncodeChar is _setEncodeChar: the first mapping found wins, except that
// a character starting a sequence gets its single-char code replaced.
func (c *dbcsCodec) setEncodeChar(uCode, dbcsCode int64) {
	bucket := c.encodeBucket(uCode)
	low := uCode & 0xFF
	if bucket[low] <= seqMark {
		c.encodeTableSeq[seqMark-bucket[low]][defChar] = seqValue{code: dbcsCode}
	} else if bucket[low] == unassigned {
		bucket[low] = dbcsCode
	}
}

// setEncodeSequence is _setEncodeSequence.
func (c *dbcsCodec) setEncodeSequence(seq []uint16, dbcsCode int64) {
	uCode := int64(seq[0])
	bucket := c.encodeBucket(uCode)
	low := uCode & 0xFF

	var node seqNode
	if bucket[low] <= seqMark {
		node = c.encodeTableSeq[seqMark-bucket[low]]
	} else {
		node = seqNode{}
		if bucket[low] != unassigned {
			node[defChar] = seqValue{code: bucket[low]}
		}
		bucket[low] = seqMark - int64(len(c.encodeTableSeq))
		c.encodeTableSeq = append(c.encodeTableSeq, node)
	}

	// iconv-lite keys the intermediate nodes by uCode (the first character)
	// rather than seq[j]; kept as is. Only sequences of three or more
	// characters reach this loop, and the 0.6.3 tables have none.
	for j := 1; j < len(seq)-1; j++ {
		old, ok := node[uCode]
		if ok && old.node != nil {
			node = old.node
			continue
		}
		next := seqNode{}
		if ok {
			next[defChar] = old
		}
		node[uCode] = seqValue{node: next}
		node = next
	}
	node[int64(seq[len(seq)-1])] = seqValue{code: dbcsCode}
}

// fillEncodeTable is _fillEncodeTable: a depth-first walk of the decoding
// trie in byte order. It reports whether the subtree holds any character.
func (c *dbcsCodec) fillEncodeTable(nodeIdx, prefix int64, skip map[int64]bool) bool {
	node := c.decodeTables[nodeIdx]
	hasValues := false
	subNodeEmpty := map[int64]bool{} // Skips empty subtrees, large in gb18030.
	for i := int64(0); i < 0x100; i++ {
		uCode := node[i]
		mbCode := prefix + i
		if skip[mbCode] {
			continue
		}
		switch {
		case uCode >= 0:
			c.setEncodeChar(uCode, mbCode)
			hasValues = true
		case uCode <= nodeMark:
			sub := nodeMark - uCode
			if !subNodeEmpty[sub] {
				if c.fillEncodeTable(sub, (mbCode<<8)&0xFFFFFFFF, skip) {
					hasValues = true
				} else {
					subNodeEmpty[sub] = true
				}
			}
		case uCode <= seqMark:
			c.setEncodeSequence(c.decodeTableSeq[seqMark-uCode], mbCode)
			hasValues = true
		}
	}
	return hasValues
}

// encode is DBCSEncoder write() followed by end().
func (c *dbcsCodec) encode(u []uint16) []byte {
	var out []byte
	writeCode := func(code int64) {
		switch {
		case code < 0x100:
			out = append(out, byte(code))
		case code < 0x10000:
			out = append(out, byte(code>>8), byte(code))
		case code < 0x1000000:
			out = append(out, byte(code>>16), byte(code>>8), byte(code))
		default:
			out = append(out, byte(code>>24), byte(code>>16), byte(code>>8), byte(code))
		}
	}

	lead := int64(-1)
	var seqObj seqNode
	nextChar := int64(-1)
	for i := 0; ; {
		// 0. Get next character.
		var uCode int64
		if nextChar == -1 {
			if i == len(u) {
				break
			}
			uCode = int64(u[i])
			i++
		} else {
			uCode, nextChar = nextChar, -1
		}

		// 1. Handle surrogates.
		if 0xD800 <= uCode && uCode < 0xE000 {
			if uCode < 0xDC00 { // Lead surrogate.
				if lead == -1 {
					lead = uCode
					continue
				}
				lead, uCode = uCode, unassigned // Double lead surrogate.
			} else if lead != -1 { // Trail surrogate completing a pair.
				uCode = 0x10000 + (lead-0xD800)*0x400 + (uCode - 0xDC00)
				lead = -1
			} else { // Trail surrogate alone.
				uCode = unassigned
			}
		} else if lead != -1 {
			// Lead surrogate alone: write an error, then the current char.
			nextChar, uCode, lead = uCode, unassigned, -1
		}

		// 2. Convert uCode.
		dbcsCode := int64(unassigned)
		if seqObj != nil && uCode != unassigned { // Inside a sequence.
			res, ok := seqObj[uCode]
			if ok && res.node != nil { // The sequence continues.
				seqObj = res.node
				continue
			}
			if ok { // The sequence is complete.
				dbcsCode = res.code
			} else if d, ok := seqObj[defChar]; ok {
				// Not part of the sequence: write its prefix, then retry
				// the current character.
				dbcsCode = d.code
				nextChar = uCode
			}
			// Without a prefix code iconv-lite drops the sequence (an
			// acknowledged gap upstream); no 0.6.3 table has such a sequence.
			seqObj = nil
		} else if uCode >= 0 {
			if bucket := c.encodeTable[uCode>>8]; bucket != nil {
				dbcsCode = bucket[uCode&0xFF]
			}
			if dbcsCode <= seqMark { // Sequence start.
				seqObj = c.encodeTableSeq[seqMark-dbcsCode]
				continue
			}
			if dbcsCode == unassigned && c.gb18030 != nil {
				if idx := findIdx(c.gb18030.UChars, uCode); idx != -1 {
					code := c.gb18030.GBChars[idx] + (uCode - c.gb18030.UChars[idx])
					out = append(out,
						byte(0x81+code/12600), byte(0x30+code%12600/1260),
						byte(0x81+code%1260/10), byte(0x30+code%10))
					continue
				}
			}
		}

		// 3. Write dbcsCode.
		if dbcsCode == unassigned {
			dbcsCode = c.defCharSB
		}
		writeCode(dbcsCode)
	}

	// write() allocates str.length*3 bytes (*4 for gb18030) and Buffer drops
	// writes past the end. This is observable: GBK maps U+E7C7 to the
	// four-byte code 81 35 F4 37, and encoding it alone yields 81 35 F4.
	perUnit := 3
	if c.gb18030 != nil {
		perUnit = 4
	}
	if limit := len(u) * perUnit; len(out) > limit {
		out = out[:limit]
	}

	// DBCSEncoder.prototype.end: flush a pending sequence prefix (at most two
	// bytes) and an unpaired lead surrogate.
	if seqObj != nil {
		if d, ok := seqObj[defChar]; ok {
			if d.code < 0x100 {
				out = append(out, byte(d.code))
			} else {
				out = append(out, byte(d.code>>8), byte(d.code))
			}
		}
	}
	if lead != -1 {
		out = append(out, byte(c.defCharSB))
	}
	return out
}

// decode is DBCSDecoder write() followed by end(): end() replaces the first
// byte of an unfinished sequence with U+FFFD and re-parses the rest.
func (c *dbcsCodec) decode(b []byte, _ bool) (written, ended []uint16) {
	written, rest := c.decodeChunk(b)
	for len(rest) > 0 {
		ended = append(ended, defaultCharUnicode)
		var u []uint16
		u, rest = c.decodeChunk(rest[1:])
		ended = append(ended, u...)
	}
	return written, ended
}

// decodeChunk is one DBCSDecoder.prototype.write call on a fresh decoder
// state. It returns the decoded units and the bytes of an unfinished
// sequence (the new prevBytes).
//
// An unassigned sequence yields U+FFFD and parsing restarts at its second
// byte.
func (c *dbcsCodec) decodeChunk(buf []byte) (u []uint16, rest []byte) {
	nodeIdx, start := int64(0), 0
	for i := 0; i < len(buf); i++ {
		uCode := c.decodeTables[nodeIdx][buf[i]]
		switch {
		case uCode >= 0: // Normal character.
		case uCode == unassigned:
			uCode = defaultCharUnicode
			i = start // Skip one byte and parse again.
		case uCode == gb18030Code:
			ptr := (int64(buf[i-3])-0x81)*12600 + (int64(buf[i-2])-0x30)*1260 +
				(int64(buf[i-1])-0x81)*10 + (int64(buf[i]) - 0x30)
			idx := findIdx(c.gb18030.GBChars, ptr)
			uCode = c.gb18030.UChars[idx] + ptr - c.gb18030.GBChars[idx]
		case uCode <= nodeMark: // Go to the next trie node.
			nodeIdx = nodeMark - uCode
			continue
		default: // uCode <= seqMark: output a sequence of chars.
			seq := c.decodeTableSeq[seqMark-uCode]
			u = append(u, seq[:len(seq)-1]...)
			uCode = int64(seq[len(seq)-1])
		}
		// Write the character, using a surrogate pair above the BMP. The
		// pointer arithmetic of out-of-range GB18030 sequences can pass
		// U+10FFFF; like the Uint8 writes of the original, the units are
		// truncated to 16 bits (and come out as lone surrogates).
		if uCode >= 0x10000 {
			uCode -= 0x10000
			u = append(u, uint16(0xD800|uCode>>10))
			uCode = 0xDC00 | uCode&0x3FF
		}
		u = append(u, uint16(uCode))
		nodeIdx, start = 0, i+1
	}
	// write() allocates buf.length*2 output bytes and Buffer drops writes
	// past the end, so the result never exceeds len(buf) code units.
	if len(u) > len(buf) {
		u = u[:len(buf)]
	}
	return u, buf[start:]
}

// findIdx returns the largest i with table[i] <= val, or -1.
func findIdx(table []int64, val int64) int {
	if table[0] > val {
		return -1
	}
	l, r := 0, len(table)
	for l < r-1 { // Always table[l] <= val < table[r].
		mid := l + (r-l+1)>>1
		if table[mid] <= val {
			l = mid
		} else {
			r = mid
		}
	}
	return l
}
