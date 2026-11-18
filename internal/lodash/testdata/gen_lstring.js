// Records lodash 4.18.1 string functions (as seen on grunt.util._) for
// internal/lodash/testdata/lstring.json.gz.
const fs = require('fs');
const { record, rnd, pick } = require('./fixlib');
const calls = [];
const odd = [undefined, null, '', 0, -0, NaN, Infinity, -Infinity, 1e21, 123.456, -42, true, false,
  [], [1, [2, null], undefined], ['a', 'b'], {}, { a: 1 }, new Date(0), /x/g];
const strs = ['', ' ', 'a', 'abc', 'fooBar', 'foo bar', 'Foo Bar', '--foo-bar--', '__FOO_BAR__', 'XMLHttpRequest',
  'foo2bar', 'foo2Bar', '2foo', 'ÀÉÎõü', 'déjà vu', 'Crème Brûlée', "don't stop", 'DON’T STOP', "it's John's", 'hello world!',
  'camelCaseString', 'snake_case_string', 'kebab-case-string', 'Title Case String', 'aBC', 'ABCdef', 'ABC',
  '1st 2nd 3rd 4th 11TH', '12th of 21st', 'été', 'ﬀ ß straße', 'İstanbul', 'ΣΑΣ', 'ǅungla',
  '😀 smile 😀', 'a😀b', '👍🏽 ok', '👨‍👩‍👧 family', '🇺🇸 flag', 'x‍y', 'a️b', '✂ cut', ' line ',
  '  \t padded \n ', ' ﻿nbsp　', 'Ⅻ roman', '日本語テキスト', 'Привет Мир', 'ŉ ĳ œ ſ', 'Æsir Þór ðað',
  'a.b.c', 'a1b2c3', 'A1B2', 'v1.2.3-beta', 'HTML5Parser', 'getHTTPResponseCode', 'foo_bar-baz qux', '×÷±¬',
  '<a href="x">Tom & \'Jerry\'</a>', '&amp;&lt;&gt;&quot;&#39;&#96;&amp;lt;', '&AMP; &#x27;'];
const caseFns = ['camelCase', 'kebabCase', 'snakeCase', 'startCase', 'lowerCase', 'upperCase', 'upperFirst', 'lowerFirst',
  'deburr', 'escape', 'unescape', 'toUpper', 'toLower', 'toString', 'toInteger', 'trimStart', 'trimEnd'];
for (const fn of caseFns) {
  for (const s of strs) calls.push([fn, [s]]);
  for (const v of odd) calls.push([fn, [v]]);
  calls.push([fn, []]);
}
// Random compounds from a mixed alphabet.
const pool = ['a', 'B', 'cd', 'EF', 'Gh', '1', '23', 'th', 'ST', ' ', '-', '_', '.', "'", '’', 'é', 'Ö', 'ß', 'ﬀ', '\u0301',
  '😀', '🏽', '\u200d', '🇫🇷', '✂', '\ufe0f', '×', '日', 'Я', 'ǅ', '\u2000', 'D', 'LL', 'RE', 's', 've'];
for (let i = 0; i < 400; i++) {
  let w = '';
  const n = 1 + Math.floor(rnd() * 7);
  for (let j = 0; j < n; j++) w += pick(pool);
  calls.push([pick(caseFns.slice(0, 9)), [w]]);
  calls.push([pick(['padStart', 'padEnd', 'trimStart', 'trimEnd', 'split']), [w, pick([5, 9, '', '😀', '-', 'é'])]]);
}
for (const v of ['3.7', '-3.7', '0x1f', '-0x1f', '0b101', '0o17', ' 12 ', '1e3', 'abc', '', 1.9, -1.9, 2 ** 60, -0, NaN,
  Infinity, -Infinity, 1e308 * 10, [7], [1, 2], '4294967297', true, null]) calls.push(['toInteger', [v]]);
// Padding.
const padStrs = ['', 'abc', 'a😀b', 'é', '👨‍👩‍👧', null, undefined, 12];
const lens = [undefined, null, 0, -1, 1, 3, 4, 5, 6, 8, 10, 2.9, -2.5, '7', NaN, Infinity, '', [6]];
const chars = [undefined, null, '', ' ', '_', '-=', 'xyz', '😀', 'a😀', 0, [1, 2], 'é́'];
for (const fn of ['padStart', 'padEnd']) {
  for (const s of padStrs) for (const l of lens) calls.push([fn, [s, l]]);
  for (let i = 0; i < 220; i++) calls.push([fn, [pick(padStrs), pick(lens), pick(chars)]]);
  calls.push([fn, ['x', 6e8, '-']]);
  calls.push([fn, ['x', 6e8, '--']]);
}
// Trimming with chars.
const trimStrs = ['  abc  ', '-_-abc-_-', 'nullxnull', 'xxaxx', '😀a😀', 'a😀', 'éé́a', '', '\t\n x 　'];
const trimChars = [undefined, null, '', '-', '_-', 'x', '😀', 'nul', 'é́', 0, ['-', '_'], ' '];
for (const fn of ['trimStart', 'trimEnd']) {
  for (const s of trimStrs) for (const c of trimChars) calls.push([fn, [s, c]]);
  calls.push([fn, ['--a--', '-', 1]]);
  calls.push([fn, ['  a  ', 'a', true]]);
}
// Split.
const splitStrs = ['a,b,c', 'a, b ,c', '', 'abc', 'a😀b😀', '👨‍👩‍👧x', 'a-b_c d', 'nullanullb', 'a1b22c333', 'a,,b,'];
const seps = [undefined, null, '', ',', ', ', /,\s*/, /(,)/, /\d+/, /(\d)(\d)?/g, /x*/, /😀/, '😀', 5, ['b'], {}, /-|_|\s/, /(?:)/];
const limits = [undefined, null, 0, 1, 2, -1, 2.5, '2', NaN, 4294967297];
for (const s of splitStrs) for (const sep of seps) calls.push(['split', [s, sep]]);
for (let i = 0; i < 200; i++) calls.push(['split', [pick(splitStrs), pick(seps), pick(limits)]]);
calls.push(['split', ['a b', 0, ['a b']]]);
calls.push(['split', ['a b', 1, ['x', 'a b']]]);
calls.push(['split', ['a b', 'k', { k: 'a b' }]]);
calls.push(['split', ['a b', 'k', { k: 'other' }]]);
calls.push(['split', [null, ',']]);
calls.push(['split', [12345, 3]]);
// Replace.
const repStrs = ['Hi Fred', 'aaa', 'a-b-c', 'x😀y😀', '', 'foo bar baz', 'nullundefined'];
const pats = ['Fred', 'a', '-', /-/g, /(a)(b)?/, /(a)(b)?/g, /\w+/g, /😀/g, /(?<w>[a-z])/g, '', null, undefined, /$/, /x*/g];
const reps = ['Barney', '[$&]', '<$1|$2>', '$$', "$`|$'", '$<w>!', '', 'wrapUpper', 'argstr', null, 7];
// argstr would stringify the null-prototype groups object, which throws in Node
// and is a property of the test callback, not of replace.
const named = p => p instanceof RegExp && p.source.includes('?<');
for (const s of repStrs) for (const p of pats) calls.push(['replace', [s, p, pick(reps)]]);
for (let i = 0; i < 200; i++) {
  const r = pick(reps), p = pick(pats);
  if (named(p) && r === 'argstr') continue;
  calls.push(['replace', [pick(repStrs), p, r === 'wrapUpper' || r === 'argstr' ? require('./fixlib').callbacks[r] : r]]);
}
calls.push(['replace', ['Hi Fred', 'Fred']]);
calls.push(['replace', ['Hi Fred']]);
calls.push(['replace', []]);
calls.push(['replace', [null, 'null', 'x']]);
calls.push(['replace', ['abc(', '(', 'x']]);
// parseInt.
const ints = ['08', ' 08', '0x1F', '0X1f', '-0x1F', '+12', '-12px', '1e3', '', ' ', '-', '+', '0', '-0', '00', '0x', '0xg',
  '12.9', 'Infinity', 'z', 'Z9', '101', '777', '1010101010101010101010101010101010101010101010101010101', '9007199254740993',
  '123456789012345678901234567890', 'zzzzzzzzzzzzzzzzzz', '0000zzzzzzzz', 'fffffffffffffffffff', ' \t42', '٣',
  '1'.repeat(400), '-' + '9'.repeat(30), 'ffffffffffffff8', '1fffffffffffff1', '20000000000000100000000000000001'];
const radixes = [undefined, null, 0, 2, 8, 10, 16, 36, 37, 1, -1, '16', 16.9, NaN, '', true, 4294967312, 32, 3, 7, 20];
for (const s of ints) for (const r of radixes) calls.push(['parseInt', [s, r]]);
for (const v of odd) calls.push(['parseInt', [v]]);
calls.push(['parseInt', ['10', 2, true]]);
calls.push(['parseInt', ['10', 2, 0]]);
const cases = record(calls);
fs.writeFileSync(process.argv[2], JSON.stringify(cases));
console.log(cases.length);
