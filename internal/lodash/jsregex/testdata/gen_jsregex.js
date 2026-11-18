// Records RegExp behaviour (exec, replace, split, errors) for internal/lodash/jsregex.
const fs = require('fs');
const patterns = [
  ['a', ''], ['a', 'g'], ['a', 'gi'], ['^\\s+|\\s+$', 'g'], ['\\s+', 'g'], ['[.*+?^=!:${}()|[\\]\\/\\\\]', 'g'],
  ['(\\d)(?=(?:\\d{3})+$)', 'g'], ['.(?=\\W*\\w*$)', 'g'], ['\\s*\\S+$', ''], ['\\w\\w', ''], ['(?:^|\\s|-)\\S', 'g'],
  ['[-_\\s]+(.)?', 'g'], ['([a-z\\d])([A-Z]+)', 'g'], ['[-\\s]+', 'g'], ['([A-Z])', 'g'], ['[-_\\s]+', 'g'], ['[\\W_]', 'g'],
  ['_id$', ''], ['\\r\\n?|\\n', ''], ['^[\\s\\\\t]*', 'gm'], ['^[ \\t]{2}', 'gm'], ['.{1,3}', 'g'], ['(\\.\\d+|\\d+|\\D+)', 'g'],
  ['<\\/?[^>]+>', 'g'], ['\\&([^;]{1,10});', 'g'], ['^#x([\\da-fA-F]+)$', ''], ['[^\\w\\s-]', 'g'], ['x*', 'g'], ['a*', 'g'],
  ['a*?', 'g'], ['(a|ab)(c|bcd)(d*)', ''], ['(a)|(b)', 'g'], ['(z)((a+)?(b+)?(c))*', ''], ['(a*)*', ''], ['(a*)+', ''], ['(?:a|b)*?c', ''],
  ['\\b\\w+\\b', 'g'], ['\\B.', 'g'], ['(\\w)\\1', 'g'], ['(?<y>\\d{4})-(?<m>\\d\\d)', 'g'], ['\\k<y>', ''], ['(?<=\\$)\\d+', 'g'],
  ['(?<!\\$)\\b\\d+', 'g'], ['(?=a)*b', ''], ['a{2,3}', 'g'], ['a{,3}', 'g'], ['a{2', 'g'], ['{', ''], ['}', ''], [']', ''],
  ['\\1(a)', ''], ['\\2(a)', ''], ['\\10', ''], ['\\08', ''], ['\\8', ''], ['\\c', ''], ['\\cJ', ''], ['[\\cJ]', ''], ['[\\c_]', ''],
  ['\\x4', ''], ['\\x41', ''], ['\\u004', ''], ['\\u0041', ''], ['\\u{41}', 'u'], ['\\u{41}', ''], ['.', 'g'], ['.', 'gs'],
  ['.', 'gu'], ['[^]', 'g'], ['[]', 'g'], ['[\\d-z]', 'g'], ['[a-\\d]', 'g'], ['[z-a]', ''], ['(', ''], [')', ''], ['*', ''],
  ['a**', ''], ['?', ''], ['[a', ''], ['\\', ''], ['(?<a>x)(?<a>y)', ''], ['(?x)', ''], ['a', 'gg'], ['a', 'x'], ['ß', 'i'],
  ['[a-z]+', 'gi'], ['K', 'gi'], ['[k]', 'gi'], ['ſ', 'i'], ['é', 'gi'], ['^', 'gm'], ['$', 'gm'], ['^a$', 'm'], ['\\n', 'g'],
  ['(?:)', 'g'], ['', 'g'], ['\\s', 'g'], ['\\S+', 'g'], ['\\D', 'g'], ['[\\s\\S]', 'g'], ['[^\\s]', 'g'], ['\\bfoo\\b', 'gi'],
  ['(a)?b', 'g'], ['(?:(a)|b)+', ''], ['(a)|b', 'g'], ['[\\b]', 'g'], ['\\0', 'g'], ['\\/', 'g'], ['😀', 'g'], ['^.$', 'u'], ['^.$', ''],
  ['[😀]', 'g'], ['[😀]', 'gu'], ['\\p{L}', 'u'], ['(?<n>a)|(?<m>b)', 'g'], ['\\k', ''], ['\\k<n>(?<n>a)', ''], ['a+?b', 'g'],
  ['(?!a)\\w', 'g'], ['(?=(a+))a*b\\1', ''], ['(.*?)a(?!(a+)b\\2c)\\2(.*)', ''], ['$', 'y'], ['a', 'y'], ['\\d{3}', 'gy'],
];
const inputs = ['', 'a', 'aaa', 'abc', 'baaabaa', '  hello  world  ', 'Hello, world foo', '1234567', '1234567.891', 'fooBarBAZ_qux-quux',
  'a\nb\r\nc\rd', '  two\n    four\n\tTab', '<p>x</p><br/>', '&amp;&#x41;&#65;&nope;&toolongentity;', 'zaacbbbcac', 'abcd', 'aabbcc',
  '2024-01 and 1999-12', '$100 and 200', 'ß SS ss', 'K k K', 'café CAFÉ', '😀a😀', 'x y', 'foo bar_baz', 'ab', 'b', 'ac',
  '\t\v\f 　﻿ end', '\u0000', 'a/b', 'aaab', 'aaaaabaaac'];
const cases = [];
for (const [src, flags] of patterns) {
  let re;
  try { re = new RegExp(src, flags); } catch (e) { cases.push({ src, flags, err: String(e) }); continue; }
  const results = [];
  for (const s of inputs) {
    re.lastIndex = 0;
    const m = new RegExp(src, flags.replace('g', '')).exec(s);
    let rep, split, fnRep;
    rep = s.replace(new RegExp(src, flags), '[$&|$1|$2|$$|$`|$\'|$<y>|$01|$10|$0]');
    fnRep = s.replace(new RegExp(src, flags), (...a) => JSON.stringify(a.map(x => x === undefined ? null : x)));
    split = s.split(new RegExp(src, flags)).map(x => x === undefined ? null : x);
    const split2 = s.split(new RegExp(src, flags), 2).map(x => x === undefined ? null : x);
    const all = flags.includes('g') ? s.match(new RegExp(src, flags)) : null;
    results.push([s, m ? [m.index, ...[...m].map(x => x === undefined ? null : x)] : null, rep, fnRep, split, split2, all]);
  }
  cases.push({ src, flags, results });
}
// String-separator split and string replace.
const strCases = [];
for (const s of inputs) for (const sep of ['', 'a', ' ', 'aa', '😀', 'zz']) strCases.push([s, sep, s.split(sep), s.split(sep, 1), s.replace(sep, '<$&$1$$>')]);
fs.writeFileSync(process.argv[2], JSON.stringify({ cases, strCases }));
console.log(cases.length);
