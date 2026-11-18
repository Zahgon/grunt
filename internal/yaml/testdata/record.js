// Records js-yaml 3.15.2's behaviour on a generated corpus, for loader_test.go.
//
//   docker run --rm -v <grunt checkout>:/orig -v $PWD:/w -w /w node:24-bookworm \
//     node record.js /orig/node_modules/js-yaml > fixtures.jsonl
//
// Two optional arguments, a scale factor for the random sections and a seed, produce
// larger or different corpora for one-off fuzzing runs.
//
// Each output line is {"name", "schema", "src", "want"}: schema is "safe" (safeLoad)
// or "full" (load), and want is the canonical encoding of the result (see encode) or
// {"error": {"name", "message"}}. The corpus is deterministic: the random generators
// use a fixed seed.
'use strict';

const yaml = require(process.argv[2] || 'js-yaml');

// encode maps a loaded value to JSON that keeps the distinctions JSON.stringify loses.
// Arrays and objects are numbered in visiting order; a repeat visit (an alias, or a
// cycle) is {"ref": n}.
function encode(value) {
  const seen = new Map();
  function enc(v) {
    if (v === undefined) return { u: 1 };
    if (v === null || typeof v === 'boolean') return v;
    if (typeof v === 'number') return { n: Object.is(v, -0) ? '-0' : String(v) };
    if (typeof v === 'string') return v.toWellFormed();
    if (typeof v === 'function') return { fn: 1 };
    if (Buffer.isBuffer(v)) return { b: v.toString('hex') };
    if (v instanceof Date) return { d: String(v.getTime()) };
    if (v instanceof RegExp) return { r: v.source.toWellFormed(), f: v.flags };
    if (seen.has(v)) return { ref: seen.get(v) };
    seen.set(v, seen.size);
    if (Array.isArray(v)) return { a: v.map(enc) };
    return { o: Object.keys(v).map((k) => [k.toWellFormed(), enc(v[k])]) };
  }
  return enc(value);
}

function run(src, schema) {
  try {
    return encode(schema === 'full' ? yaml.load(src) : yaml.safeLoad(src));
  } catch (e) {
    return { error: { name: e.name, message: String(e.message).toWellFormed() } };
  }
}

// --- deterministic randomness -------------------------------------------------------

let seed = Number(process.argv[4] || 0x5eed1234);
const scale = Number(process.argv[3] || 1);
function rnd() {
  // mulberry32
  seed = (seed + 0x6d2b79f5) | 0;
  let t = seed;
  t = Math.imul(t ^ (t >>> 15), t | 1);
  t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
  return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
}
const int = (n) => Math.floor(rnd() * n);
const pick = (xs) => xs[int(xs.length)];
const chance = (p) => rnd() < p;

// --- hand-written corpus ------------------------------------------------------------

const hand = [
  // documents and streams
  '', '\n', ' \n', '# only a comment', '---', '--- \n...', '...', '---\n---', 'a\n---\nb',
  '--- a\n... \n', '%YAML 1.2\n---\na', '%YAML 1.1\n%YAML 1.1\n---', '%YAML 2.0\n---',
  '%YAML 1\n---', '%YAML 1.2 x\n---', '%TAG ! tag:example.com,2000:\n--- !foo bar',
  '%TAG !e! tag:e.com:\n---\n!e!x 1', '%TAG !e! tag:e.com:\n%TAG !e! tag:f.com:\n---',
  '%TAG !e!\n---', '%TAG e tag:x\n---', '%TAG !e! {x}\n---', '%FOO bar\n---\nx', '%\n---',
  '%YAML 1.2', '%YAML 1.2\nfoo', '\ufeffa: 1', '\ufeff', 'a: 1\r\nb: 2\r\n', 'a: 1\rb: 2',
  'a\0b', 'x: "\0"', '  a', '  - a\n  - b', ' # c\n a: 1',
  // plain scalars and implicit types
  'null', 'Null', 'NULL', '~', 'nULL', 'true', 'True', 'TRUE', 'tRUE', 'false', 'yes', 'no',
  'on', 'off', 'y', '0', '-0', '+0', '00', '-00', '0_0', '007', '08', '0777', '0o17', '0b101',
  '-0b101', '0b', '0b2', '0x1F', '0X1F', '0xg', '0x_1', '0x1_', '+0x10', '1_000', '_1', '1_',
  '1__0', '190:20:30', '-1:30', '1:60', '1:5:9', '1_0:30', '1_:30', '0:30', '1:', ':30',
  '9007199254740993', '18446744073709551616', '0x20000000000001', '0b' + '1'.repeat(70),
  '1.5', '-1.5', '+1.5', '1.', '.5', '-.5', '+.5', '._5', '.__1', '1e5', '1E5', '1e+5',
  '1.5e-3', '1.e5', '1e', '1e5.5', '1_000.5_5', '1.5_', '0.0', '-0.0', '+0.0', '00.5',
  '01.5', '1.7976931348623157e308', '1e400', '-1e400', '1e-400', '4.9e-324', '2.5e-324',
  '190:20:30.15', '1:30.', '-1:30.5', '1:30.5e3', '.inf', '.Inf', '.INF', '-.inf', '+.inf',
  '.iNf', '.nan', '.NaN', '.NAN', '-.nan', 'inf', 'nan', 'Infinity', 'NaN', '0.1', '0.30000000000000004',
  '123456789012345678901234567890', '1e21', '1e-7', '123e-20', '5e-324',
  '2001-12-14', '2001-12-14t21:59:43.10-05:00', '2001-12-14 21:59:43.10 -5',
  '2001-12-15T02:59:43.1Z', '2001-12-15 2:59:43.10', '2002-12-14', '2001-1-1', '2001-01-01T1:00:00',
  '2001-1-1 1:00:00', '2001-02-30', '2001-13-01', '2001-00-01', '2001-01-00', '0050-06-15',
  '0099-12-31T23:59:59Z', '0100-01-01', '9999-12-31T23:59:59.999+14:00', '2001-12-14 21:59:43.1234567',
  '2001-12-14 21:59:43.', '2001-12-14T21:59:43+01', '2001-12-14T21:59:43 +01:30',
  '2001-12-14T21:59:43 Z', '2001-12-14T21:59:43z', '2001-12-14  21:59:43', '2001-12-14\t21:59:43',
  '2001-12-14T99:99:99', '2001-12-14T21:59', '<<', '=', 'a b', 'a  b ', 'a #b', 'a#b',
  'a:b', 'a: b: c', 'http://x.y/z?a=b#c', '-a', '?a', ':a', '- a', '? a', ': a', '-', '?', ':',
  'a\n b\n  c', 'a\n\n b', 'a\n\n\n b', 'a\n#c\nb', '---a', 'a ---', '[a]b', '{a}b', 'a,b',
  '@a', '`a', '%a', 'a\tb', 'a \t', '\ta', 'a\u0085b', 'a\u2028b', 'a\x7fb', 'a\x01b', 'a\ufffeb',
  'é', '😀', 'a😀: b', '\u00a0a',
  // quoted scalars
  "'a'", "'a''b'", "''", "''''", "'a\n b'", "'a\n\n b'", "'a\n  \n b'", "'a\nb'", "'a", "'a\n---\n'",
  "'\ta'", "'a\x01'", "'a\x7f'", '"a"', '""', '"a\\"b"', '"\\0\\a\\b\\t\\\t\\n\\v\\f\\r\\e\\ \\"\\/\\\\\\N\\_\\L\\P"',
  '"\\x41\\x4a\\x4A"', '"\\u263A"', '"\\U0001F600"', '"\\uD83D\\uDE00"', '"\\uD83D"', '"\\uDE00x"',
  '"\\UFFFFFFFF"', '"\\U80000000"', '"\\U7FFFFFFF"', '"\\U00110000"', '"\\x4"', '"\\xGG"', '"\\u12"',
  '"\\q"', '"\\', '"a', '"a\n---\n"', '"a\\\n  b"', '"a\\\n\n  b"', '"a\n b"', '"a \n  b"', '"a\n\n b"',
  '"a\\\r\n b"', '"\x01"', '"\t"', '"\x7f"', '"\u0085"', '"a" b', '"a"b', '"a" # c', "'a' # c",
  '"a":1', '"a": 1', "'a': 1", '"a\nb": 1', '"\\x00": 1',
  // block scalars
  '|\n a\n b', '|\n a\n b\n', '|\n a\n\n', '|-\n a\n\n', '|+\n a\n\n', '|+\n\n', '|-\n', '|\n',
  '|', '>', '>\n a\n b\n\n c\n', '>\n a\n  b\n c\n', '>\n  a\n b', '>-\n a\n b', '>+\n a\n\n\n',
  '|2\n   a\n  b', '|1\n  a', '|0\n a', '|10\n a', '|+-\n a', '|++\n a', '|22\n a', '|2+\n  a', '|+2\n  a',
  '| # c\n a', '|#c\n a', '| x\n a', '|\n\ta', '|\n  a\n\tb', '>\n\n a', '>\n \n a', '>\n a\n\n  b\n\n c',
  '>\n  a\n   b\n  c\n\n  d', '- |\n  a\n- >\n  b', 'k: |\n  a\n k2: v', 'k: |\n  a\nk2: v',
  'k: >-\n  a\n  b\n\nk2: v', '|\n a\x01', '|\n a\r\n b\r\n', '>\n a\r\n\r\n b', '|\n  \n  a',
  '|\n    \n  a', '|1\n \n  a', '>\n  a\n \n  b', '|\n a\n...\n', '|\n a\n---\n',
  // block collections
  'a: 1\nb: 2', 'a: 1\na: 2', 'a:\n  b: 1\n  c: 2', 'a:\n- 1\n- 2', 'a:\n  - 1\n  - 2', '- a\n- b',
  '- - a\n  - b\n- c', '- a: 1\n  b: 2\n- c', '-\n  a', '-\n- b', '- \n-', 'a:', 'a:\nb:', 'a: \n  b',
  'a:\n b\n c: 1', 'a: 1\n b: 2', 'a: 1\n  b: 2', ' a: 1\nb: 2', 'a: 1\n- b', '- a\nb: 1',
  '- a\n - b', '- a\n  - b', 'a:\n  - b\n  c: 1', 'a: b\nc', 'a: b\n c', 'a:b: c', 'a :b',
  'a : b', 'a\t: b', 'a:\tb', 'a:\t\tb', '? a\n: b', '? a\n? b\n: c', '? a', '?\n: b', '? - a\n  - b\n: c',
  '? |\n  a\n: b', ': b', '? a\n: b\n: c', 'a: 1\n? b\n: 2', '? a\n  b\n: c', '?a: b', 'a: 1\n\tb: 2',
  'a:\n\t- b', '\ta: 1', 'a: 1\n  # c\nb: 2', 'a: 1 # c\nb: 2 #d', 'a: |\n  x\nb: >\n  y',
  '"a\n b": 1', 'a\n b: 1', 'a: b\n  c: d', 'k:\n  - a\n  -b', '- - - a', '- ? a\n  : b',
  '[a]: 1', '{a: 1}: 2', '[a, [b]]: 1', '[a, {b: 1}]: 2', '[1, null, true, ~]: x', '{a: 1}: 1\n{b: 2}: 2',
  '1: a\n1.0: b', '1: a\n"1": b', 'null: a\n~: b', 'true: a\nTrue: b', '.nan: a\n.NaN: b',
  '0x10: a\n16: b', '- 1\n- 2\n- 3\n- 4\n- 5\n- 6\n- 7\n- 8\n- 9\n- 10\n- 11',
  'b: 1\n2: 2\na: 3\n1: 4\n01: 5\n-1: 6\n4294967295: 7\n4294967294: 8', '__proto__: 1',
  '__proto__: {a: 1}', '__proto__: 1\n__proto__: 2', '{__proto__: 1, a: 2}', 'constructor: 1\ntoString: 2\nhasOwnProperty: 3',
  'a: 1\nb: 2\nc: 3\nd: 4\ne: 5\nf: 6\ng: 7\nh: 8\ni: 9\nj: 10',
  // flow collections
  '[]', '[a]', '[a,]', '[a, b]', '[ a , b ]', '[a,,b]', '[,a]', '[a b]', '[a\nb]', '[a\n, b]',
  '[a', '[a,', '[[a]', '[a]]', ']', '}', '{}', '{a}', '{a: b}', '{a: b,}', '{a:b}', '{a:1, b:2}',
  '{"a":1}', "{'a':1}", '{a: b c}', '{a, b: c}', '{a: [b, c]}', '{a: {b: c}}', '{a: b', '{a: b}}',
  '{a: 1, a: 2}', '{? a: b}', '{? a}', '{?a: b}', '{: b}', '[a: b]', '[a: b, c: d]', '[? a: b]',
  '[? a]', '[a: b, a: c]', '[: b]', '[a:b]', '[a:]', '[a:,b]', '["a":b]', '[a]#c', '[a] #c', '[a # c\n]',
  '[a, #c\n b]', '{a: b\n , c: d}', '{a:\n b}', '[\na\n]', '[a\n]', '- [a,\nb]', 'k: [a,\nb]', 'k: [a,\n  b]',
  'k: {a: 1,\n b: 2}', '[a, - b]', '[-a]', '[- a]', '[a-b]', '[a, ?b]', '{a: -}', '[*]', '[&]',
  '[a, &x b, *x]', '[&x a, *x]', '{&x a: *x}', '[!!str 1, !!int "2"]', '[a]: [b]', '[ [a, b], [c] ]',
  '[{a: 1}, {b: 2}]', '{a: 1}\n', '[a]\n- b', '{a: 1} x', '[a] b',
  // anchors, aliases, merges
  'a: &x 1\nb: *x', 'a: &x [1, 2]\nb: *x', '*x', 'a: *x', 'a: &x\nb: *x', '&x', '&', '*', '& a',
  'a: &x &y 1', 'a: &x !!str 1\nb: *x', 'a: !!str &x 1\nb: *x', 'a: &x 1\nb: !!str *x', 'a: &x 1\nb: &y *x',
  '- &x\n  foo\n- *x', '- &x !!str\n  foo\n- *x', '&a [*a]', '&a {b: *a}', '&a\n- *a', '&a\nb: *a',
  'a: &x {b: 1}\nc: *x\nd: *x', 'a: &x b\n*x : c', 'a: &x b\n? *x\n: c', '&x a: b', 'a: &x b\nc: &x d\ne: *x',
  'a: &__proto__ 1\nb: *__proto__', 'a: &x\n  - 1\nb: *x', '- &x a\n- *x\n- *x',
  'a: &x {k: 1}\nb:\n  <<: *x\n  j: 2', 'a: &x {k: 1}\nb:\n  <<: *x\n  k: 2', 'a: &x {k: 1}\nb:\n  k: 2\n  <<: *x',
  'a: &x {k: 1}\nb: &y {k: 2, j: 3}\nc:\n  <<: [*x, *y]', 'a: &x {k: 1}\nc:\n  <<: [*x, *x]\n  k: 3\n  k: 4',
  '<<: {a: 1}', '<<: [{a: 1}, {a: 2, b: 3}]', '<<: 1', '<<: [1]', '<<: "a"', '<<: ~', '<<:', '<<: [[1, 2]]',
  '<<: [1, 2]', '<<: [[a, b], {c: d}]', '<<: !!binary aGVsbG8=', '<<: 2001-01-01', '<<: {a: 1}\n<<: {b: 2}',
  '<<: {a: 1}\na: 2', 'a: 2\n<<: {a: 1}', '"<<": {a: 1}', '!!merge <<: {a: 1}', '? <<\n: {a: 1}',
  '{<<: {a: 1}, b: 2}', '[<<: {a: 1}]', '<<: {__proto__: 1}\n__proto__: 2', '<<: {a: 1}\n<<: {a: 2}',
  '<<: !!set {a, b}', '<<: [!!omap [a: 1]]', '<<: {a: 1, b: 2}\nb: 3\nc: 4',
  // tags
  '!!str 1', '!!str', '!!str ~', '!!str true', '!!int 1', '!!int "1"', '!!int 1.5', '!!int', '!!int ~',
  '!!float 1', '!!float "1.5"', '!!float', '!!bool true', '!!bool yes', '!!bool', '!!null ~', '!!null',
  '!!null a', '!!timestamp 2001-01-01', '!!timestamp a', '!!seq [a]', '!!seq', '!!seq a', '!!map {a: 1}',
  '!!map', '!!map [a]', '!!set {a, b}', '!!set {a: 1}', '!!set\n? a\n? b', '!!set', '!!omap [a: 1, b: 2]',
  '!!omap [a: 1, a: 2]', '!!omap [a, b]', '!!omap [{a: 1, b: 2}]', '!!omap', '!!omap\n- a: 1\n- b: 2',
  '!!pairs [a: 1, a: 2]', '!!pairs [a]', '!!pairs', '!!pairs\n- a: 1\n- b: [2]', '!!binary aGVsbG8=',
  '!!binary aGVsbG8', '!!binary aGVsbG8==', '!!binary "aGVs\nbG8="', '!!binary |\n  aGVs\n  bG8=',
  '!!binary a', '!!binary ""', '!!binary Q===', '!!binary "QQ=="', '!!binary a b', '!!binary', '!!binary ~',
  '!!binary\n  1234', '!!merge <<', '!!merge a', '!!merge', '!!foo a', '!foo a', '!foo', '! a', '! 1', '!',
  '!<tag:yaml.org,2002:str> 1', '!<!foo> a', '!<> a', '!<>x> a', '!<?> 1', '!<?> "1"', '!<?> [1]',
  '!<?>', '!<tag:yaml.org,2002:str', '!<a b> c', '!e!x a', '!a!b!c d', '!!a!b c', '!a{b} c', '!a,b c',
  '!a%20b c', '!a%2 c', '!a%zz c', '!é a', '!😀 a', '!<😀> a', '!!str !!str a', '!!str &x !!int 1',
  '- !!str\n  123', '- !!int\n  123', '- !!float\n  123', '- !!binary\n  123', '- !!str\n  ~',
  '- !!js/regexp\n  12', '- !!timestamp\n  2001-01-01', '- !!null\n  ~', '- !!bool\n  true',
  '- !<?>\n  12', '- !!str\n  *x', '- !!seq\n  [a]', '- !!map\n  {a: 1}', '- !!set\n  {a: 1}',
  '- !!str\n  2001-01-01', '!!str\n2001-01-01', '!!map\na: 1', '!!seq\n- a', '!!str\na: 1', '!!str [a]',
  '!!str {a: 1}', '!!int [1]', '!!seq {a: 1}', 'a: !!str', 'a: !!str\nb: 1', '- !!str\n- b', '!!str\n',
  '!!str a: b', '!!str a: !!int 1', '? !!str a\n: !!int 1', '[!!str]', '{!!str : a}', '{!!str a: b}',
  '!!str\n  a\n  b', '&x !!str\n  a', '!!js/undefined', '!!js/undefined a', '!!js/undefined [a]',
  '!!js/regexp /a/g', '!!js/regexp a', '!!js/function "function(){}"',
  // keys built from non-strings
  '? !!binary aGVsbG8=\n: x', '? [!!binary 4oI=]\n: x', '2001-01-01: x', '2001-01-01T12:30:00.5Z: x',
  '[2001-01-01, 1]: x', '? !!set {a}\n: x', '? !!pairs [a: 1]\n: x', '? !!omap [a: 1]\n: x',
  '? [[a]]\n: x', '? [a, [b]]\n: x', '? [{a: 1}, b]\n: x', '? [~, b, ~]\n: x', '? []\n: x',
  '0.1: a\n1e21: b\n-0.0: c\n.inf: d\n-.inf: e', '? !!binary ""\n: x', '? !!binary /w==\n: x',
  '? [!!binary 4pyT, !!binary 7aCA]\n: x',
  // indentation and structure errors
  'a:\n  b: 1\n c: 2', 'a:\n  - 1\n - 2', '- a\n  b: 1', 'a: [\nb]', 'a: {\nb: 1}', '- a\n -b',
  'a: b: c', 'a: "b\nc', "a: 'b", 'a: [b', 'a: {b', '[a: [b]]', 'a: >\n b\nc: d\n e', 'a: b\n c: d',
  'a:\n  b\n  c: d', 'a: 1\nb', 'a\nb: 1', 'a: 1\n\n\nb', '- a\nb', '"a"\n"b"', '[a]\n[b]', 'a: 1\n  - b',
  'x: - a', 'x: ? a', '? a\n: b\n:c', 'a:\n\n  b', 'a:\n\t\tb: c', '---\na\n--- b\n...\nc', 'a\n...\nb',
  'a\n... x', '--- [a]\n--- [b]', '--- !!str\n---', 'a: |\n  b\n c', 'a: -1\nb: - 1',
];

// Targeted cases: limits, long lines, keys of every type, regular expressions.
const long = 'x'.repeat(60);
const manyKeys = Array.from({ length: 120 }, (_, i) => 'k' + i + ': ' + i).join(', ');
const targeted = [
  long + ': [' + long, '[' + long + long, 'a: "' + long + '\\q' + long + '"', 'k: ' + long + ' : v',
  'a: 1\n' + 'b'.repeat(20) + ' c: [' + long, '- ' + long.slice(0, 36) + '\n - x', 'x'.repeat(37) + ': [',
  'x'.repeat(36) + ': [', '\t'.repeat(40) + 'a: [', 'é'.repeat(50) + ': [' + '😀'.repeat(30),
  '😀'.repeat(20) + ': [', 'a: [\n' + '😀'.repeat(40) + '\n', 'a: 1\u2028b: [', 'a: 1\u0085b: [',
  'a: &m {' + manyKeys + '}\nb: [' + Array.from({ length: 90 }, () => '{<<: *m}').join(', ') + ']',
  'a: &m {' + manyKeys + '}\n' + Array.from({ length: 82 }, (_, i) => 'b' + i + ': {<<: *m}').join('\n'),
  'e: &e {}\nm: {<<: [' + Array.from({ length: 101 }, () => '*e').join(', ') + ']}',
  'e: &e {}\nm: {<<: [' + Array.from({ length: 100 }, () => '*e').join(', ') + ']}',
  'e: &e [' + Array.from({ length: 30 }, (_, i) => i).join(',') + ']\nm: {<<: [*e]}',
  '? !!js/undefined\n: a', '? !!js/regexp /a\\/b/gim\n: a', '? [!!js/undefined, !!js/regexp x]\n: a',
  'true: a\nfalse: b', '? !!binary gA==\n: a', '? !!binary wqI=\n: a', '? !!binary 4oKs\n: a',
  '? !!binary 8J+YgA==\n: a', '? !!binary 8J+Y\n: a', '? !!binary 4oJB\n: a', '? !!binary 7aCA\n: a',
  '? !!binary 4ICA\n: a', '? !!binary 8ICAgA==\n: a', '? !!binary 9JCAgA==\n: a', '? !!binary wA==\n: a',
  '? !!binary //8=\n: a', '? !!binary 9I+/vw==\n: a', '? !!binary 37/f\n: a', '? !!binary 8JCA\n: a',
  '? 1999-12-31T23:59:59.999-14:00\n: a', '? 0001-01-01\n: a', '? 9999-12-31T23:59:59.999-23:59\n: a',
  '? 2000-99-99T99:99:99\n: a', '? 2000-02-29\n: a', '? 1900-02-29\n: a', '? 1969-12-31T23:59:59.999Z\n: a',
  'a: &x 1\nb:\n  - !!str\n    *x', 'a: &x ~\nb: !!str\n  *x', 'a: &x [1]\nb: !!seq\n  *x',
  'a: &x 1\n- !<?>\n  *x', 'a: &x 1\nb: !<?>\n  *x', '- &x 1\n- !!str\n  *x', '- &x 1\n- !<?>\n  *x',
  '- &x {a: 1}\n- !!map\n  *x', '- &x 1\n- !!int\n  *x', '!<?> [0]', '!<?> {a: 1}', '- !<?>\n  [0]',
  '%YAML 1.2\n%TAG !! tag:example.com,2000:\n---\n!!x 1', '%TAG ! !\n--- !a b', '%TAG !e! %20\n---\n!e!x a',
  '%TAG !e! tag:x,2000:\n---\n!e! a', '%YAML 01.1\n---\na', '%YAML 1.\n---', '%YAML .1\n---',
  '%YAML 1.3\n---\na', '%YAML 1.0\n---\na', '%YAML 1.2 # c\n---\na', '%YAML\n---', '% YAML 1.2\n---',
  ' %YAML 1.2\n---', '%YAML 1.2\n...\n---\na', '%YAML 1.2\n--- a\n...\n%YAML 1.2\n--- b',
  '--- a\n...\n--- b',
  '? a\nb: c', '? a\n? b\nc: d', '? a\nb', '? a\n"b": c', '!!js/regexp "a\\rb"', '!!js/regexp "[\\r]\\r"',
  '!!js/regexp "x{1,a}"', '!!js/regexp "x{1,2a}"', '!!js/regexp "(?<\\\\ud835\\\\u0041>x)"',
  '? 0100-00-01\n: a', '? 0000-01-01\n: a', '0x7fffffffffffffff: a', '0777777777777777777777: a',
  '99999999999999999999:30: a', '- 1_2.3_4e5\n- 123456789.123456789e100\n- 0.1e-5\n- 4_9e-325', 'a\n...\n', 'a\n... # c\n', '---\n...\n---\n...', '--- |\n  a\n--- >\n  b',
  '---\r\na: 1\r\n...\r\n', '--- #c\na', '---#c\na', '--- a: 1', '---\ta', '--- - a\n- b',
];
for (const r of ['(?ii:a)', '(?i-i:a)', '(?-:a)', '(?i', '(?i)', '(?--i:a)', '(?x:a)', '(?is-m:a)', '(?-i:a)',
  '(?i-:a)', '(?m-s:a)', '(?ims-:a)', '(?<a>x)|(?<a>y)', '((?<a>x)|(?<a>y))(?<a>z)', '(?:(?<a>x)|y)|(?<a>z)',
  '(?<a>x)(?:(?<a>y)|z)', '(?<a>x|(?<a>y))', '(?<a>(?<b>x)|(?<b>y))', '\\k', '\\k<a', '\\k<a>', '(?<a>.)\\k',
  '(?<a>.)\\k<', '(?<a>.)\\k<b', '(?<a>.)\\k<b>', '(?<a>.)\\k<a>', '\\k<a>(?<a>.)', '(?<1a>x)',
  '(?<a\\u0062>x)\\k<ab>', '(?<a\\u00>x)', '(?<\\u{61}>x)', '(?<\\u{110000}>x)', '(?<\\u{}>x)',
  '(?<a\\u005c>x)', '(?<\\ud835\\udc9c>x)', '(?<\\ud835>x)', '(?<a\\b>x)', '(?<$_>x)', '(?<é>x)',
  '(?<𝒜>x)', '(?<a\u200d>x)', '(?<a-b>x)', '(?<a1>x)', '(?<_>x)', '(?<a>x)(?<b>y)\\k<c>', '[\\d-z]',
  '[a-\\d]', '[\\x41-\\x40]', '[\\cA-\\c@]', '[\\c1-\\c0]', '[\\c_-\\c0]', '\\c', '[\\c]',
  '[\\c', '[\\101-\\100]', '[\\0-\\1]', '[\\377-\\400]', '[\\u0042-\\u0041]', '[\\u004-a]',
  '[\\b-\\a]', '[\\f-\\n]', '[\\t-\\v]', '[\\r-\\n]', '[z-\\x]', '[\\k-a]', '[\\8-\\7]',
  '[\\08-\\0]', '[a-]', '[-a]', '[a-', '[^]', '[^a-b-c]', '[]]', '[\\]]', 'x{2,1}', '{2,1}', '{2}',
  'x{,2}', 'x{2,}', 'x{2', 'x{2,3', 'x{a}', 'x{99999999999,5}', 'x{2147483647,2147483648}', 'x{1}?',
  'x{1}??', '^*', '$+', '\\b*', '\\B?', '(?=a)*', '(?!a)+', '(?<!a)?', '(?<=a){1}', 'a|*', '()*',
  '(*)', 'a{1}{2}', 'a*?*', '\\8', '\\1(a)', '\\10', '[😀-😁]', '😀+', 'a\\/b', '/', '[/]/',
  '\\', '[\\', '(?<a>', '(?<a>x', '(?<a', '(?<', '(?', '(?=', '(?<=x)+', '(?:a', 'a)', 'a]', 'a}', '(?i:a',
  '(?i-', '(?i-m', '(?i-m:', 'a\nb', 'a\\\nb', '[a\nb/]/', 'a\u2028\u2029', '\\\u2028', '']) {
  targeted.push('!!js/regexp ' + JSON.stringify(r));
}
for (const r of ['/a/', '/a/g', '/a/gim', '/a/mig', '/a/gg', '/a/gimm', '/a/x', '/', '//', '//g', '/a', 'a/g',
  '/a/b/g', '/(/g', '/[/]/', '/\\//']) targeted.push('!!js/regexp ' + r);

// --- random generators --------------------------------------------------------------

const words = ['a', 'b', 'key', 'x y', 'null', '~', 'true', 'False', '1', '-2', '0x1f', '0o7', '0777',
  '1_0', '1:30', '1.5', '.5', '1e3', '.inf', '-.Inf', '.nan', '2001-02-03', '2001-02-03 04:05:06 +07',
  '<<', 'é', '😀', 'a:b', 'a#b', 'http://x', '-x', '?x', '%x', '012', '+1', '0b11', '9'.repeat(25)];
const specials = ['-', '?', ':', ',', '[', ']', '{', '}', '#', '&', '*', '!', '|', '>', "'", '"', '%',
  '@', '`', ' ', '\t', '\n', '\r', '\\', '.', '<', '~', '0', 'a', '\u2028', '\x85', '\ufeff', '\x01'];

function scalar(depth) {
  const w = pick(words);
  switch (int(9)) {
    case 0: return "'" + w.replace(/'/g, "''") + "'";
    case 1: return '"' + w.replace(/["\\]/g, '\\$&') + '"';
    case 2: return '"' + pick(['\\x41', '\\u00e9', '\\U0001F600', '\\n', '\\t', '\\\\', '\\"', '\\N', '\\_', '\\L', '\\P', '\\0', '\\e', '\\ ', '\\/']) + w + '"';
    case 3: return '!!' + pick(['str', 'int', 'float', 'bool', 'null', 'timestamp', 'binary', 'merge']) + ' ' + w;
    case 4: return '&' + pick(['a', 'b', 'c']) + ' ' + w;
    case 5: return '*' + pick(['a', 'b', 'c']);
    default: return w;
  }
}

function flow(depth) {
  if (depth > 2 || chance(0.4)) return scalar(depth);
  const n = int(4);
  const items = [];
  if (chance(0.5)) {
    for (let i = 0; i < n; i++) items.push(chance(0.3) ? scalar(depth) + ': ' + flow(depth + 1) : flow(depth + 1));
    return pick(['', '&a ', '!!seq ', '!!set ', '!!omap ', '!!pairs ']) + '[' + items.join(pick([', ', ',', ' , ', ',\n '])) + ']';
  }
  for (let i = 0; i < n; i++) items.push(scalar(depth) + pick([': ', ':', '']) + (chance(0.8) ? flow(depth + 1) : ''));
  return pick(['', '&b ', '!!map ', '!!set ']) + '{' + items.join(pick([', ', ',', ',\n '])) + '}';
}

function block(indent, depth) {
  const pad = ' '.repeat(indent);
  if (depth > 2 || chance(0.25)) {
    if (chance(0.15)) {
      const lines = [];
      for (let i = int(4); i >= 0; i--) lines.push(pad + '  ' + ' '.repeat(int(2)) + pick(words));
      return pick(['|', '>', '|-', '>+', '|2', '>1-']) + '\n' + lines.join(pick(['\n', '\n\n']));
    }
    return chance(0.3) ? flow(depth) : scalar(depth);
  }
  const lines = [];
  const n = 1 + int(3);
  const inner = indent + 1 + int(3);
  if (chance(0.5)) {
    for (let i = 0; i < n; i++) {
      const v = block(inner, depth + 1);
      lines.push(pad + '-' + (v.includes('\n') && !/^[|>]/.test(v) ? '\n' + ' '.repeat(inner) : ' ') + v);
    }
  } else {
    for (let i = 0; i < n; i++) {
      const k = chance(0.1) ? '? ' + scalar(depth) + '\n' + pad + ':' : (chance(0.1) ? '<<' : scalar(depth)) + ':';
      const v = block(inner, depth + 1);
      lines.push(pad + k + (v.includes('\n') && !/^[|>]/.test(v) ? '\n' + ' '.repeat(inner) : ' ') + v);
    }
  }
  return pick(['', '', '', '&c\n' + pad, '!!map\n' + pad, '!!omap\n' + pad]) + lines.join('\n').replace(/^ +/, '');
}

function mutate(s) {
  const chars = [...s];
  const ops = 1 + int(3);
  for (let i = 0; i < ops; i++) {
    const at = int(chars.length + 1);
    switch (int(4)) {
      case 0: chars.splice(at, 1); break;
      case 1: chars.splice(at, 0, pick(specials)); break;
      case 2: chars.splice(at, 0, ' '.repeat(1 + int(3))); break;
      default: chars[at] = pick(specials);
    }
  }
  return chars.join('');
}

const scalarAlphabet = '0123456789+-._:eExXbBoOtTzZ \tinfaNul~<'.split('');
function randomScalar() {
  let s = '';
  for (let i = 1 + int(12); i > 0; i--) s += pick(scalarAlphabet);
  return s;
}

function randomTimestamp() {
  const d = (n) => String(int(Math.pow(10, n))).padStart(n, '0');
  let s = d(4) + '-' + (chance(0.8) ? d(2) : d(1)) + '-' + (chance(0.8) ? d(2) : d(1));
  if (chance(0.7)) {
    s += pick(['T', 't', ' ', '  ', '\t']) + d(1 + int(2)) + ':' + d(2) + ':' + d(2);
    if (chance(0.5)) s += '.' + d(int(6));
    if (chance(0.6)) s += pick(['', ' ', '\t']) + pick(['Z', '+' + d(1 + int(2)), '-' + d(2) + ':' + d(2), '+' + d(2) + ':' + d(2)]);
  }
  return s;
}

function randomEscapes() {
  let s = '"';
  for (let i = 1 + int(6); i > 0; i--) {
    s += pick(['\\x' + int(256).toString(16).padStart(2, '0'), '\\u' + int(65536).toString(16).padStart(4, '0'),
      '\\U' + (int(0x110000) * (chance(0.1) ? 4096 : 1)).toString(16).padStart(8, '0').slice(-8),
      '\\' + pick('0abtnvfre "/\\N_LPqx'.split('')), 'z', ' ', '\n ', '\\\n ']);
  }
  return s + '"';
}

const reAlphabet = ['a', 'b', '(', ')', '[', ']', '{', '}', '*', '+', '?', '|', '^', '$', '.', '\\', '-',
  '<', '>', '=', '!', ':', ',', '1', '2', 'd', 'k', 'c', 'x', 'u', 'i', 'm', 's', '/', '0', '9', '-', ')', ')'];
function randomRegExp() {
  let s = '';
  for (let i = 1 + int(10); i > 0; i--) s += chance(0.15) ? pick(['(?<a>', '(?<b>', '\\k<a>', '(?:', '(?=', '(?<=', '(?i:', '(?-m:', '[a-z]', '{1,2}', '\\u0041', '\\x4', '(?<a>x)', '(?<a>y)|', '[\\x41-', '[\\c', '\\k<', '(?im-s:']) : pick(reAlphabet);
  return s;
}

// --- assemble -----------------------------------------------------------------------

const cases = [];
const seenCases = new Set();
const add = (name, src, schema) => {
  const key = schema + '\0' + src;
  if (!seenCases.has(key)) {
    seenCases.add(key);
    cases.push({ name, schema, src });
  }
};

hand.forEach((src, i) => { add('hand/' + i, src, 'safe'); add('hand/' + i, src, 'full'); });
targeted.forEach((src, i) => add('targeted/' + i, src, 'full'));
for (let i = 0; i < 1000 * scale; i++) add('block/' + i, block(0, 0), chance(0.2) ? 'full' : 'safe');
for (let i = 0; i < 1500 * scale; i++) add('flow/' + i, flow(0), 'safe');
for (let i = 0; i < 1600 * scale; i++) add('mutated/' + i, mutate(chance(0.5) ? block(0, 0) : flow(0)), 'safe');
for (let i = 0; i < 1500 * scale; i++) {
  const s = randomScalar();
  add('scalar/' + i, chance(0.5) ? s : pick(['k: ', '- ', '[', '{k: ', '? ']) + s + pick(['', ']', '}', ': v']), 'safe');
}
for (let i = 0; i < 600 * scale; i++) add('timestamp/' + i, randomTimestamp(), 'safe');
for (let i = 0; i < 600 * scale; i++) add('escape/' + i, randomEscapes(), 'safe');
for (let i = 0; i < 1200 * scale; i++) {
  let s = '';
  for (let j = 1 + int(12); j > 0; j--) s += pick(specials);
  add('soup/' + i, s, 'safe');
}
for (let i = 0; i < 400 * scale; i++) {
  const docs = [];
  for (let j = int(4); j >= 0; j--) {
    let d = '';
    if (chance(0.3)) d += pick(['%YAML 1.2\n', '%YAML 1.1\n', '%TAG !e! tag:e.com,2000:\n', '%FOO x\n', '%YAML 1.2\n%YAML 1.2\n']);
    if (chance(0.7)) d += pick(['---\n', '--- ', '--- # c\n', '---\r\n']);
    d += pick(['a', 'k: v', '- x', '!e!t x', '"q"', '|\n  lit', '', '# c', '[a, b]']);
    if (chance(0.4)) d += pick(['\n...', '\n... # c', '\n...\n', '\r\n...']);
    docs.push(d);
  }
  add('stream/' + i, docs.join(pick(['\n', '\r\n', '\n\n'])), 'safe');
}
for (let i = 0; i < 300 * scale; i++) {
  const bytes = Array.from({ length: 1 + int(6) }, () => pick([int(256), 0x80 + int(64), 0xc2 + int(30), 0xe0 + int(16), 0xf0 + int(5), 0xed, 0xe0, 0xf4]));
  add('binarykey/' + i, '? !!binary ' + Buffer.from(bytes).toString('base64') + '\n: v', 'safe');
}
for (let i = 0; i < 800 * scale; i++) {
  const re = randomRegExp();
  add('regexp/' + i, '!!js/regexp ' + JSON.stringify(chance(0.5) ? re : '/' + re + '/' + pick(['', 'g', 'gim', 'mg', 'gg', 'x'])), 'full');
}

for (const c of cases) {
  process.stdout.write(JSON.stringify({ name: c.name, schema: c.schema, src: c.src, want: run(c.src, c.schema) }) + '\n');
}
