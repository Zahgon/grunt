// record.js records the differential fixtures of package iconv from
// iconv-lite 0.6.3 running under Node.js 24:
//
//   cases.json       explicit decode/encode/name cases with their results
//   exhaustive.json  SHA-256 digests over exhaustive per-codec sweeps
//
// Usage: node record.js <iconv-lite package dir> <output dir>
//
// The PRNG is seeded, so re-recording on the same Node.js version yields the
// same files. Decoded strings are stored after String.prototype.toWellFormed
// (lone surrogates -> U+FFFD), which is how a Go string represents them.
"use strict";

var path = require("path");
var fs = require("fs");
var crypto = require("crypto");

var pkgDir = path.resolve(process.argv[2]);
var outDir = path.resolve(process.argv[3]);
var iconv = require(pkgDir);
var encodings = require(path.join(pkgDir, "encodings"));

// mulberry32
var seed = 0x1c0de;
function rand() {
    seed = (seed + 0x6D2B79F5) | 0;
    var t = seed;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
}
function randInt(lo, hi) { return lo + Math.floor(rand() * (hi - lo + 1)); }
function pick(arr) { return arr[Math.floor(rand() * arr.length)]; }

function randBytes(n, lo, hi) {
    var b = Buffer.alloc(n);
    for (var i = 0; i < n; i++) b[i] = randInt(lo === undefined ? 0 : lo, hi === undefined ? 255 : hi);
    return b;
}

var classes = [
    [0x20, 0x7E], [0x00, 0x1F], [0xA0, 0xFF], [0x0100, 0x017F], [0x0370, 0x03FF],
    [0x0400, 0x04FF], [0x0590, 0x05FF], [0x0600, 0x06FF], [0x0E00, 0x0E7F],
    [0x2000, 0x206F], [0x2500, 0x257F], [0x3040, 0x30FF], [0x4E00, 0x9FFF],
    [0xAC00, 0xD7A3], [0xFF00, 0xFFEF], [0x1F300, 0x1F64F], [0x20000, 0x2A6DF], [0x0300, 0x036F],
];
var specials = [0xFFFD, 0xFEFF, 0x00, 0x20AC, 0xA5, 0x203E, 0x2B, 0x2D, 0x26, 0x3D, 0x2F, 0x2C, 0x7E, 0x5C];

function randString(n, repertoire) {
    var s = "";
    for (var i = 0; i < n; i++) {
        var r = rand();
        if (repertoire && repertoire.length > 0 && r < 0.5) s += pick(repertoire);
        else if (r < 0.6) s += String.fromCodePoint(pick(specials));
        else { var c = pick(classes); s += String.fromCodePoint(randInt(c[0], c[1])); }
    }
    return s;
}

// charsetString draws n characters (code points, never lone surrogates)
// from chars, a string or an array.
function charsetString(chars, n) {
    var cps = Array.from(chars), s = "";
    for (var i = 0; i < n; i++) s += cps[randInt(0, cps.length - 1)];
    return s;
}

function wellFormed(s) { return s.toWellFormed(); }

// --- Codec groups -----------------------------------------------------------

var names = Object.keys(encodings).filter(function (n) { return n[0] !== "_"; }).sort();
var groups = new Map(); // codec -> representative name
names.forEach(function (n) {
    var codec = iconv.getCodec(n);
    var isDef = typeof encodings[n] !== "string";
    if (!groups.has(codec) || (isDef && typeof encodings[groups.get(codec)] === "string"))
        groups.set(codec, n);
});
var reps = Array.from(groups.values()).sort();

function kindOf(name) {
    var def = encodings[name];
    if (def && def.type === "_sbcs") return "sbcs";
    if (def && def.type === "_dbcs") return "dbcs";
    return name;
}

function repertoire(name) {
    var kind = kindOf(name), set = new Set();
    var add = function (s) {
        Array.from(s).forEach(function (ch) { if (ch !== "�" && ch.isWellFormed()) set.add(ch); });
    };
    if (kind === "sbcs") {
        var all = Buffer.alloc(256);
        for (var i = 0; i < 256; i++) all[i] = i;
        add(iconv.decode(all, name));
    } else if (kind === "dbcs") {
        for (var k = 0; k < 3000; k++) add(iconv.decode(Buffer.from([randInt(0x81, 0xFE), randInt(0x40, 0xFE)]), name));
    }
    return Array.from(set);
}

// --- Explicit cases -----------------------------------------------------------

var decodeCases = [], encodeCases = [], nameCases = [];

function addDecode(enc, buf, strip) {
    var out = iconv.decode(Buffer.from(buf), enc, { stripBOM: strip });
    decodeCases.push({ enc: enc, hex: Buffer.from(buf).toString("hex"), strip: strip, out: wellFormed(out) });
}
function addEncode(enc, str, bom) {
    var out = iconv.encode(str, enc, bom ? { addBOM: true } : undefined);
    encodeCases.push({ enc: enc, in: str, bom: bom, hex: out.toString("hex") });
}

var fixedDecode = [
    "", "00", "41", "efbbbf41", "efbbbf", "efbb", "fffe4100", "fffe", "feff0041", "feff",
    "fffe000041000000", "0000feff00000041", "fffe0000", "0000feff",
    "41e282", "e28241", "eda080", "c080", "f4908080", "ff41", "e08041", "f09f98", "eda0bdedb2a9",
    Buffer.from("+/v8-A").toString("hex"), Buffer.from("&/v8-A").toString("hex"),
    Buffer.from("+/v7+/w-").toString("hex"), Buffer.from("+/v7+/w").toString("hex"),
    Buffer.from("+/v8").toString("hex"),
];
var fixedEncode = ["", "A", "﻿A", "a+b-c&d=e/f,g~h\\i", "Hi Mom -☺-!", "日本語",
    "😀", "\u0000", "�", "¥‾€", "é̈", "Ê̄Ê̌ê̄"];

var utf7Samples = ["Hi Mom -+Jjo--!", "+AGEAYgBj", "+AGE-", "+-", "a+b", "+ZeVnLIqe-", "&AGE-&-", "&ZeVnLIqe-x",
    "+AGEAYgBjAGQ", "+AGEAYgBjAGQAZQ", "+AGEAYgBjAGQAZQBm-", "+2D3eAA-", "+2D0-", "&2D3eAA-", "+AGE,AGI-", "&AGE,AGI-",
    "+AGE+AGI-", "+AGé", "+/v8A", "a&b+c-d", "+AAA-", "+A-", "+AA-", "+AAAA", "&-&-&"];
var base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
var base64Extra = base64Chars + "==== \n\t\r-_.!ŁĽé€";

reps.forEach(function (enc) {
    var kind = kindOf(enc);
    var rep = repertoire(enc);
    var i;

    fixedDecode.forEach(function (h) { addDecode(enc, Buffer.from(h, "hex"), true); addDecode(enc, Buffer.from(h, "hex"), false); });
    fixedEncode.forEach(function (s) { addEncode(enc, s, false); addEncode(enc, s, true); });

    var nDecode = kind === "sbcs" ? 4 : 24, nEncode = kind === "sbcs" ? 4 : 24;
    for (i = 0; i < nDecode; i++) addDecode(enc, randBytes(randInt(1, 40)), i % 2 === 0);
    for (i = 0; i < nDecode / 4; i++) addDecode(enc, randBytes(randInt(1, 40), 0x80, 0xFF), i % 2 === 0);
    for (i = 0; i < 2; i++) addDecode(enc, randBytes(randInt(100, 260)), i % 2 === 0);
    for (i = 0; i < nEncode; i++) addEncode(enc, randString(randInt(1, 24), rep), i % 2 === 1);
    // Round trips through the encoder exercise valid input of every codec.
    for (i = 0; i < nEncode / 2; i++) addDecode(enc, iconv.encode(randString(randInt(1, 24), rep), enc), i % 2 === 0);

    if (kind === "sbcs") {
        var all = Buffer.alloc(256);
        for (i = 0; i < 256; i++) all[i] = i;
        addDecode(enc, all, true);
        addEncode(enc, rep.join(""), false);
        addEncode(enc, rep.slice().reverse().join(""), false);
    }
    if (kind === "dbcs") {
        for (i = 0; i < 24; i++) addDecode(enc, randBytes(randInt(2, 30), 0x80, 0xFE), i % 2 === 0);
        for (i = 0; i < 8; i++) addEncode(enc, charsetString(rep, randInt(20, 60)), false);
    }
    if (enc === "gb18030") {
        for (i = 0; i < 24; i++) {
            var b = [];
            for (var k = 0; k < randInt(1, 6); k++)
                b.push(randInt(0x81, 0xFE), randInt(0x30, 0x39), randInt(0x81, 0xFE), randInt(0x30, 0x39));
            if (i % 3 === 0) b.pop();
            addDecode(enc, Buffer.from(b), true);
        }
        for (i = 0; i < 12; i++) addEncode(enc, randString(randInt(1, 12)), false);
    }
    if (enc === "utf7" || enc === "utf7imap") {
        utf7Samples.forEach(function (s) { addDecode(enc, Buffer.from(s, "latin1"), true); addDecode(enc, Buffer.from(s, "latin1"), false); });
        for (i = 0; i < 40; i++) addDecode(enc, Buffer.from(charsetString(base64Chars + "+-&,", randInt(1, 30)), "latin1"), i % 2 === 0);
        for (i = 0; i < 20; i++) addEncode(enc, charsetString("abc+-&,~ é日😀﻿", randInt(1, 20)), i % 2 === 0);
    }
    if (enc === "base64") {
        for (i = 0; i < 120; i++) addEncode(enc, charsetString(i < 40 ? base64Chars : base64Extra, randInt(0, 30)), false);
    }
    if (enc === "hex") {
        for (i = 0; i < 60; i++) addEncode(enc, charsetString("0123456789abcdefABCDEFgz Ĵı", randInt(0, 20)), false);
    }
    if (enc === "utf16" || enc === "utf32" || enc === "ucs2" || enc === "utf16be" || enc === "utf32le" || enc === "utf32be") {
        ["utf-16le", "utf-16be", "utf-32le", "utf-32be"].forEach(function (inner) {
            [3, 7, 8, 15, 16, 40, 120].forEach(function (n) {
                var text = charsetString("abc XYZ\néĀ一", n);
                addDecode(enc, iconv.encode(text, inner), true);
                addDecode(enc, iconv.encode(text, inner, { addBOM: true }), n % 2 === 0);
            });
        });
    }
});

encodeCases.forEach(function (c) {
    if (!c.in.isWellFormed()) throw new Error("ill-formed encode input for " + c.enc);
});

// Every name and alias of the table, through one decode and one encode.
var probeBytes = Buffer.from("41e980fffe00a4a130a1a24f2b2d", "hex");
var probeString = "Aé€日😀+-&";
var spellings = ["UTF-8", "utf_8", "Utf8", "Latin1", "LATIN-1", "win1252", "CP1252", "Windows-1252", "windows1251",
    "ISO-8859-15", "ISO_8859-1", "iso-ir-100", "koi8r", "KOI8-R", "UTF16BE", "UTF-16", "utf-16le", "UCS-2", "UCS2",
    "Shift_JIS", "SJIS", "EUC-JP", "GB2312", "GBK", "GB18030", "Big5", "Big5-HKSCS", "EUC-KR", "ks_c_5601-1987",
    "ISO-8859-1:1987", "utf8:2000", "utf8:200", "utf8:20000", "utf8:2000:2000", "US-ASCII", "ASCII", "binary", "Base64", "HEX",
    "UTF-7", "UTF-7-IMAP", "utf32", "UTF-32BE", "UCS-4", "cesu-8", "Koi8-r", "İso-8859-2", "macintosh", "mac",
    "x-mac-cyrillic", "IBM866", "cp866", "csIBM866", "ms_kanji", "windows-31j", "unicode-1-1-utf-8", "unicode-1-1-utf-7",
    "  utf 8  ", "u.t.f.8", "UTF 8", "ütf8"];
var unknown = ["foo", "", "utf-9", "latin", "_sbcs", "_internal", "sbcs", "internal", "dbcs", "utf32x", "日本",
    "iso-8859-1:198", "cp-12345", "1", "undefined", "null", "ucs4x", "win-1252x"];

names.concat(spellings, unknown).forEach(function (name) {
    var c = { name: name, canonical: iconv._canonicalizeEncoding(name), exists: iconv.encodingExists(name) };
    try {
        c.decoded = wellFormed(iconv.decode(probeBytes, name, { stripBOM: true }));
        c.encoded = iconv.encode(probeString, name).toString("hex");
    } catch (e) {
        c.error = e.message;
    }
    nameCases.push(c);
});

// --- Exhaustive sweeps ----------------------------------------------------------

function uint32(n) { var b = Buffer.alloc(4); b.writeUInt32BE(n); return b; }

// sweepDecode hashes the decoding of every buffer produced by gen.
function sweepDecode(enc, strip, bufs) {
    var h = crypto.createHash("sha256"), count = 0, illFormed = 0;
    bufs(function (buf) {
        var s = iconv.decode(buf, enc, { stripBOM: strip });
        if (!s.isWellFormed()) illFormed++;
        h.update(Buffer.from(wellFormed(s), "utf8"));
        h.update(Buffer.from([0xFF]));
        count++;
    });
    return { count: count, illFormed: illFormed, sha256: h.digest("hex") };
}

function allSingles(emit) { for (var a = 0; a < 256; a++) emit(Buffer.from([a])); }
function allPairs(emit) { for (var a = 0; a < 256; a++) for (var b = 0; b < 256; b++) emit(Buffer.from([a, b])); }
function triples(firsts) {
    return function (emit) {
        firsts.forEach(function (a) { for (var b = 0; b < 256; b++) for (var c = 0; c < 256; c++) emit(Buffer.from([a, b, c])); });
    };
}
function gbQuads(emit) {
    for (var a = 0x81; a <= 0xFE; a++) for (var b = 0x30; b <= 0x39; b++)
        for (var c = 0x81; c <= 0xFE; c++) for (var d = 0x30; d <= 0x39; d++) emit(Buffer.from([a, b, c, d]));
}

// sweepEncode hashes the encoding of scalar values, one per call: every BMP
// value, every value below fullUpTo, and every 97th value above it.
function sweepEncode(enc, fullUpTo) {
    var h = crypto.createHash("sha256"), count = 0;
    for (var cp = 0; cp <= 0x10FFFF; cp += (cp < fullUpTo ? 1 : 97)) {
        if (cp >= 0xD800 && cp < 0xE000) continue;
        var out = iconv.encode(String.fromCodePoint(cp), enc);
        h.update(uint32(out.length));
        h.update(out);
        count++;
    }
    return { count: count, sha256: h.digest("hex") };
}

var exhaustive = [];
reps.forEach(function (enc) {
    var kind = kindOf(enc);
    var add = function (sweep, r) { r.enc = enc; r.sweep = sweep; exhaustive.push(r); };
    add("singles", sweepDecode(enc, true, allSingles));
    add("pairs", sweepDecode(enc, true, allPairs));
    if (kind === "dbcs") add("triples", sweepDecode(enc, true, triples([0x81, 0x8E, 0x8F, 0xA4, 0xC8, 0xFE])));
    if (enc === "gb18030") add("gbquads", sweepDecode(enc, true, gbQuads));
    if (enc === "utf8" || enc === "cesu8") {
        add("triples", sweepDecode(enc, true, triples([0xE0, 0xED, 0xEF, 0xF0, 0xF4, 0xC2])));
    }
    // Supplementary planes are sampled, except planes 1-2 for DBCS tables.
    add("encode", sweepEncode(enc, kind === "dbcs" ? 0x30000 : 0x10000));
});

var meta = { node: process.version, iconvLite: require(path.join(pkgDir, "package.json")).version };
fs.writeFileSync(path.join(outDir, "cases.json"),
    JSON.stringify({ meta: meta, decode: decodeCases, encode: encodeCases, names: nameCases }));
fs.writeFileSync(path.join(outDir, "exhaustive.json"),
    JSON.stringify({ meta: meta, sweeps: exhaustive }, null, 1));
console.log("decode", decodeCases.length, "encode", encodeCases.length, "names", nameCases.length,
    "sweeps", exhaustive.length);
