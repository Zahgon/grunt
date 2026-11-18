// Records underscore.string 3.3.6 (and sprintf-js 1.1.3) calls through
// grunt.util._ for internal/lodash/testdata/ustring.json.gz.
const fs = require('fs');
const { record, rnd, pick } = require('./fixlib');

const strs = ['', ' ', 'a', 'hello world', '  Hello World  ', 'foo_bar-baz qux', 'FooBarBaz', 'fooBAR', 'élan déjà vu Ærø ß',
  'ǅungla', 'İstanbul ΑΣ ΣΑΣ', '😀 emoji 😀x', 'a\nb\r\nc\rd', 'tab\there\u00a0nbsp', '<p>Hello <b>you</b></p><br/>',
  '&lt;&amp;&#39;&#x41;&#65;&nope;&constructor;&__proto__;&toString;&#xFFFF;&#1234567;&apos;&nbsp;&#x1F600;&toolongentityname;',
  '123', '1,234.5', 'abc123def45', 'some_id', 'user_id_id', 'Hello, world! How are you?', '-dash-', '__under__', 'x-y_z w',
  'ÀÉÎÕÜ çñ łđ ŞĞ', 'ﬁ ŉ ǰ', '"quoted"', "'single'", 'aaa', 'abcabcabc', '\\t\\tbackslash', '  two\n    four\n  two', '\t\ttab\n\tone',
  'The quick brown fox jumps', 'Supercalifragilistic', 'a1b2c3', 'file10.txt', 'file9.txt', 'v1.10', 'v1.9', 'x.05', 'x.5'];
const odd = [null, undefined, 0, 1, -1, 12.5, NaN, true, false, [1, 2], ['a'], {}, { a: 1 }, new Date(0)];
const nums = [undefined, null, 0, 1, 2, 3, 5, 10, 20, -1, -3, 1.5, 2.7, -2.5, NaN, Infinity, '4', 'x', true];
const calls = [];
const add = (fn, ...args) => calls.push([fn, args]);

for (const s of [...strs, ...odd]) {
  for (const fn of ['isBlank', 'stripTags', 'decapitalize', 'clean', 'cleanDiacritics', 'chars', 'swapCase', 'escapeHTML',
    'unescapeHTML', 'lines', 'pred', 'succ', 'titleize', 'underscored', 'dasherize', 'classify', 'humanize', 'slugify',
    'escapeRegExp', 'trim', 'ltrim', 'rtrim', 'strip', 'lstrip', 'rstrip', 'words', 'camelize', 'camelcase', 'capitalize', 'dedent']) add(fn, s);
  add('capitalize', s, true);
  add('camelize', s, true);
  add('chop', s, pick([2, 3, 0, -1, '2', 1.9, undefined]));
}
const trimChars = [undefined, null, '', 0, '_', '-', ' ', 'ab', 'a-z', '.*', '[]', '\\', 'o', 1, ['a', 'b'], /[ol]/, /l+/g, /\s/, { source: 'o' }];
for (const c of trimChars) for (const s of ['__hello__', '  hi  ', 'ooloollo', 'abcba', '-a-b-', 'x.*x.*', '1a1', 'a,b', '\\x\\', '[x]']) {
  add('trim', s, c); add('ltrim', s, c); add('rtrim', s, c);
}
for (const s of ['  a  b  c ', 'a,b,,c', 'one  two\tthree', '', '   ', 'a-b_c', 'x1y1z', null]) for (const d of [undefined, ',', ' ', '-', /[-_]/, /,/g, 1, '', /(,)/]) add('words', s, d);
for (const [s, n] of [['', 3], ['ab', 5], ['abc', 2], ['abc', 0], ['😀', 4], [5, 3], [null, 2], ['x', -3], ['x', 4.9], ['x', '4'], ['x', NaN], ['ab', 2147483649]]) {
  for (const p of [undefined, '', '0', 'ab', '-', 5, true, 1.5, ['x'], {}, null, '😀']) for (const t of [undefined, 'left', 'right', 'both', 'x']) add('pad', s, n, p, t);
  add('lpad', s, n, '*'); add('rpad', s, n, '*'); add('lrpad', s, n, '*'); add('center', s, n); add('rjust', s, n, 0); add('ljust', s, n, '.');
}
add('pad', 'x', 4, ['a', 'b']);
for (const s of ['Hello world', 'abc', '', null, 12345, '😀😀😀', 'Hello, cruel world!', 'Hello cruel world', 'a b', 'Supercalifragilistic', 'Hello.World']) {
  for (const n of [undefined, 0, 1, 3, 5, 7, 10, -2, -10, 2.5, '4', 100]) {
    add('truncate', s, n); add('truncate', s, n, '…'); add('truncate', s, n, 0); add('truncate', s, n, 7);
    add('prune', s, n); add('prune', s, n, ' (more)'); add('prune', s, n, null); add('prune', s, n, 0); add('prune', s, n, '');
  }
}
for (const s of ['hello', 'hello world', '', null, 'aaaa', '😀x']) for (const i of [0, 1, 3, -1, -10, 10, 1.7, '2', undefined]) {
  add('splice', s, i, pick([0, 1, 2, -1, undefined, 10]), pick(['X', '', undefined, null, 5, '😀']));
  add('insert', s, i, pick(['X', 'ZZ', undefined, 0]));
}
for (const [s, f] of [['a.b.c', '.'], ['a.b.c', '\\.'], ['Hello hello HELLO', 'hello'], ['xax', 'a'], ['abc', ''], ['abc', undefined], ['abc', null],
  ['a+b', '+'], ['a1b22c333', '\\d+'], ['a1b22c333', /\d/], ['aXbxc', /x/i], ['abc', '(']]) {
  for (const r of ['-', '[$&]', '$1', '', undefined, 'wrapUpper']) for (const ic of [undefined, true, 'yes'])
    add('replaceAll', s, f, r === 'wrapUpper' ? require('./fixlib').callbacks.wrapUpper : r, ic);
}
for (const s of ['abcabc', 'aaaa', 'hello', '', null, 1212, '😀😀']) for (const sub of ['a', 'aa', 'bc', '', null, 1, '😀', 'x']) add('count', s, sub);
for (const s of ['hello world', 'hello', '', null, 12345, '😀a😀']) for (const t of ['hello', 'world', 'o', '', undefined, null, 123, 'h']) {
  for (const p of [undefined, null, 0, 1, 2, 6, -1, 100, 1.5, '1', NaN, Infinity]) { add('startsWith', s, t, p); add('endsWith', s, t, p); }
}
for (const s of ['  a\n    b\n  c', '    x\n  y', 'no indent', '\t\ta\n\tb', '  \n  x', 'ta\ntb', '\\a\n\\b', '', null, '  x\r  y', '   a\n   b']) {
  for (const p of [undefined, '  ', ' ', '\\s+', 5, '(']) add('dedent', s, p);
}
for (const s of ['hello world', 'a/b/c', 'a::b::c', '', null, 'x', 1234, 'aXbXc']) for (const sep of ['/', '::', 'X', '', null, undefined, 'zz', 3, 'o']) {
  add('strRight', s, sep); add('strRightBack', s, sep); add('strLeft', s, sep); add('strLeftBack', s, sep);
}
for (const a of [[], ['a'], ['a', 'b'], ['a', 'b', 'c'], ['a', null, undefined, 'd'], [1, 2, 3, 4], null, undefined, 'abc', 5, {}, [[1, 2], 'x']]) {
  for (const [sep, last] of [[undefined, undefined], ['; ', ' or '], ['', ''], [', ', undefined], [' , ', ' & '], [0, 1]]) {
    add('toSentence', a, sep, last); add('toSentence', a, sep, last, true); add('toSentenceSerial', a, sep, last);
  }
}
for (const s of ['abc', '', null, 5, '"x"']) for (const w of ['"', "'", '*', '', null, undefined, 0, 7, '<>']) { add('surround', s, w); add('quote', s, w); add('q', s, w); }
for (const s of ['"abc"', "'abc'", '"abc', '"', '""', '', 'abc', 5, ['"', 'x', '"'], ['"'], ['a'], { 0: '"', length: 1 }, null, undefined]) for (const q of [undefined, "'", '"', 'a', 5]) add('unquote', s, q);
for (const s of ['ab', '', null, 5, '😀']) for (const n of [0, 1, 3, -1, 2.9, '2', undefined, NaN]) for (const sep of [undefined, null, ',', '', 0, '-+-']) add('repeat', s, n, sep);
const cmp = ['a', 'b', 'A', 'a1', 'a2', 'a10', 'a01', 'file9.txt', 'file10.txt', 'v1.9', 'v1.10', 'x.05', 'x.5', '', null, undefined, 0, 1, '1', 10, '10', 'abc', 'abc1', ' 1', '  1', '1 ', 'é', 'z', '😀', true, [1], {}];
for (let i = 0; i < 400; i++) add('naturalCmp', pick(cmp), pick(cmp));
for (const a of cmp) add('naturalCmp', a, a);
for (let i = 0; i < 200; i++) add('levenshtein', pick([...strs.slice(0, 12), 'kitten', 'sitting', 'saturday', 'sunday', null, 123]), pick(['kitten', 'sitting', '', 'hello world', 'Hello World', null, 1234, '😀 emoji']));
for (const s of ['true', 'false', ' TRUE ', '1', '0', 'yes', 'no', 'Y', '', 'on', 1, 0, 2, null, undefined, {}, [], true, false, NaN, 'True']) {
  add('toBoolean', s);
  add('toBoolean', s, ['yes', 'on', 'y'], ['no', 'off', 'n']);
  add('toBoolean', s, 'yes', 'no');
  add('toBoolean', s, /^y/i, /^n/i);
  add('toBoolean', s, [/^o/, 'true'], [5]);
  add('toBool', s, null, ['', 'off']);
}
for (const n of [0, 1, 1.5, 2.5, -2.5, 1234.5678, -0.0001, 1e21, 1e-7, NaN, Infinity, null, undefined, '12.345', 'x', true, [1.25], 0.125, 0.5, -0.5, 1.005, 123.456, 3.14159, 1e15]) {
  for (const p of [undefined, null, 0, 1, 2, 3, -1, -2, 1.5, '2', 'x', NaN, Infinity, 23, -23, 400]) add('toNumber', n, p);
}
for (const n of [0, 1, 1234, 1234.5678, -1234.5678, 1234567.891, 1e21, 0.5, 1.005, 2.5, -0.0001, 999.9999, NaN, Infinity, -Infinity, null, undefined, '1234', true, [5], 123456789012345680000]) {
  for (const [d, ds, ts] of [[undefined, undefined, undefined], [2, undefined, undefined], [0, ',', '.'], [3, ',', ' '], [1, undefined, ''], [2, 5, 7], [-1, '.', ','], [2.9, '', "'"], [2, '.', '$&'], [101, '.', ','], [20, '.', ',']]) add('numberFormat', n, d, ds, ts);
}
for (const s of ['abc', '', null, 'éx', 'Hello World']) for (const cb of ['upper', 'argstr', 'len', 5, undefined]) add('mapChars', s, typeof cb === 'string' ? require('./fixlib').callbacks[cb] : cb);
// sprintf / vsprintf
const fmts = ['%s', '%d', '%i', '%f', '%e', '%b', '%o', '%x', '%X', '%c', '%j', '%u', '%g', '%t', '%T', '%v', '%%', '%5s', '%-5s|', '%05d', "%'*8s",
  "%'#-8d|", '%+d', '%+05d', '% d', '%.2f', '%8.3f', '%-8.3f|', '%+.1e', '%.0e', '%.3e', '%.2g', '%.0g', '%.10g', '%.1s', '%.0s', '%10.4f', '%+i',
  '%2$s %1$s', '%1$s %1$s', '%(name)s', '%(a.b)s', '%(a.b[1])s', '%(arr[0])s', '%(name)s and %(x)s', '%(1x)s', '%(a.)s', '%(name)s %s', '%s %(name)s',
  '%3j', '%12j', '%2j', 'plain', '', 'a%sb%sc', '%z', '%', 'x%', '%5.1f%%', '%+f', '%+.3f', '%.15f', '%.20f', '%.100f', '%.101f', '%e', '%.100e', '%.101g', '%30s',
  '%-030d', '%0-5d|', "%'05d", "%'$5d", '%1$s', '%01$s', '%0$s', '%.2t', '%.3T', '%.1v', '%5t', '%5T', '%5v', '%x %x', '%.f', '%5.s'];
const sargs = [0, 1, -1, 42, -42, 3.14159, -3.14159, 2.5, 0.000001234, 123456789.123, 1e21, -0, NaN, Infinity, -Infinity, 255, 65, 128512, 4294967297,
  '42', '3.7', 'abc', '', ' 12 ', true, false, null, undefined, [1, 2], [], { a: 1 }, { name: 'Bob', a: { b: [1, 'two'] }, arr: ['z'], x: null }, new Date(0), 'ünï😀'];
for (const f of fmts) for (let i = 0; i < 12; i++) add('sprintf', f, pick(sargs), pick(sargs));
for (const f of fmts) add('sprintf', f, { name: 'Bob', a: { b: [1, 'two'] }, arr: ['z'], x: null }, 'second');
for (const f of ['%s', '%d', '%f', '%.2f', '%5.1f', '%x', '%b']) for (const a of sargs) add('sprintf', f, a);
add('sprintf', '%s', require('./fixlib').callbacks.argstr);
add('sprintf', '%T', require('./fixlib').callbacks.argstr);
add('sprintf', '%d', require('./fixlib').callbacks.argstr);
add('sprintf'); add('sprintf', undefined); add('sprintf', 0); add('sprintf', 5); add('sprintf', null, 1);
add('sprintf', '%(a.b.c)s', { a: {} }); add('sprintf', '%(a.b.c)s', { a: null }); add('sprintf', '%(a)s', null); add('sprintf', '%(a)s');
add('sprintf', '%s %s %s', 'only one'); add('sprintf', '%d', 'x'); add('sprintf', '%g', 'x'); add('sprintf', '%.2g', '5'); add('sprintf', '%.2g', null);
add('sprintf', '%.2v', 5); add('sprintf', '%v', null); add('sprintf', '%99999999999999999999999s', 'x'); add('sprintf', '%9999999999s', 'x');
for (const argv of [[1, 2], ['a'], 'str', null, undefined, 0, 5, [], [[1, 2]], { a: 1 }]) { add('vsprintf', '%s|%s', argv); add('vsprintf', '%(a)s', argv); }
// Random numbers through the Number.prototype formatters behind sprintf and numberFormat.
function randNum() {
  const r = rnd();
  if (r < 0.2) return Math.floor(rnd() * 1e6) / Math.pow(10, Math.floor(rnd() * 6));
  if (r < 0.4) return (rnd() - 0.5) * Math.pow(10, Math.floor(rnd() * 40) - 20);
  if (r < 0.5) return Math.floor(rnd() * 2000) / 8 - 125; // exact binary fractions: ties
  if (r < 0.6) return (Math.floor(rnd() * 100000) + 0.5) / Math.pow(10, Math.floor(rnd() * 4));
  if (r < 0.7) return Math.pow(2, Math.floor(rnd() * 120) - 30) * (rnd() < 0.5 ? -1 : 1);
  if (r < 0.8) return Math.floor((rnd() - 0.5) * 1e18);
  return rnd() * 1000;
}
for (let i = 0; i < 2500; i++) {
  const x = randNum(), p = Math.floor(rnd() * 22);
  add('sprintf', pick(['%.' + p + 'f', '%.' + p + 'e', '%.' + (p + 1) + 'g', '%e', '%f', '%g', '%b', '%x', '%o', '%d', '%+.' + p + 'f']), x);
  if (i % 3 === 0) add('numberFormat', x, Math.floor(rnd() * 8), pick([undefined, ',', '.']), pick([undefined, '.', ' ', "'"]));
  if (i % 5 === 0) add('toNumber', x, Math.floor(rnd() * 12) - 3);
}
const alphabet = ['a', 'B', ' ', '-', '_', 'é', 'ß', '1', '0', '.', ',', '\t', '\n', 'Σ', '<', '&', ';', '"', 'x', 'Y', 'ǅ', '😀', '\u00a0'];
const randStr = () => { let s = ''; const n = Math.floor(rnd() * 12); for (let i = 0; i < n; i++) s += pick(alphabet); return s; };
const oneArg = ['isBlank', 'stripTags', 'decapitalize', 'clean', 'cleanDiacritics', 'chars', 'swapCase', 'escapeHTML', 'unescapeHTML', 'lines',
  'titleize', 'underscored', 'dasherize', 'classify', 'humanize', 'slugify', 'escapeRegExp', 'trim', 'words', 'camelize', 'capitalize', 'dedent', 'succ', 'pred'];
for (let i = 0; i < 2500; i++) {
  const s = randStr();
  add(pick(oneArg), s);
  if (i % 2) add(pick(['trim', 'ltrim', 'rtrim', 'words', 'count', 'strLeft', 'strRight', 'strLeftBack', 'strRightBack', 'levenshtein', 'naturalCmp', 'startsWith', 'endsWith']), s, randStr());
  if (i % 3 === 0) add(pick(['truncate', 'prune', 'pad', 'lrpad', 'rpad', 'chop', 'repeat']), s, Math.floor(rnd() * 14) - 2, pick([undefined, '*', '..', randStr()]));
}
fs.writeFileSync(process.argv[2], JSON.stringify(record(calls)));
console.log(calls.length);
