// Records lodash collection/array/object/lang/math calls for internal/lodash.
const fs = require('fs');
const { record, rnd, pick, callbacks: cb } = require('./fixlib');
const calls = [];
const add = (fn, ...args) => calls.push([fn, args]);

const prims = [undefined, null, 0, -0, 1, -1, 2, 3, 1.5, NaN, Infinity, -Infinity, '', 'a', 'abc', '2', '10', ' 3 ', '0x1f', '0b11', '-0x1', true, false];
const arrays = [[], [1, 2, 3], [3, 1, 2, 1, 3], [1, [2, [3, [4]]], 5], [0, -0, NaN, NaN, null, undefined, '', false], ['b', 'a', 'c', 'a'],
  [{ a: 1 }, { a: 2 }], [[1, 2], [3]], [1, '1', true, 'true'], [10, 9, 100, '9', 'z', null, undefined, NaN], ['😀x', 'é']];
const objects = [{}, { a: 1, b: 2 }, { b: 2, a: 1, 2: 'two', 1: 'one' }, { a: { b: { c: 3 } }, x: [1, { y: 2 }] }, { length: 2, 0: 'x', 1: 'y' },
  { 'a.b': 1, a: { b: 2 } }, { u: undefined, n: null }];
const values = [...prims, ...arrays, ...objects, new Date(0), /ab+c/gi];

// Array functions over many inputs.
for (const v of values) {
  for (const fn of ['compact', 'flatten', 'flattenDeep', 'head', 'first', 'initial', 'last', 'tail', 'uniq', 'size', 'keys', 'values', 'isEmpty',
    'isArray', 'isBoolean', 'isNil', 'isNull', 'isNumber', 'isPlainObject', 'isString', 'isUndefined', 'toArray', 'clone', 'cloneDeep',
    'max', 'min', 'sum', 'mean', 'identity', 'noop', 'join', 'reverse', 'concat', 'sortBy', 'orderBy']) add(fn, v);
}
for (const v of [...arrays, 'abc', null, { length: 3, 0: 1, 2: 3 }]) {
  for (const n of [undefined, 0, 1, 2, -1, 1.7, 10, '2', NaN, Infinity]) { add('take', v, n); add('drop', v, n); add('chunk', v, n); }
  add('take', v, 5, 'guard'); add('drop', v, 2, true); add('chunk', v, 2, { 0: v });
  for (const x of [1, 'a', NaN, -0, null, undefined, '1']) { add('indexOf', v, x); add('indexOf', v, x, -2); add('indexOf', v, x, 2); add('includes', v, x); add('includes', v, x, -1); }
  for (const sep of [undefined, '-', '', null, 0, ', ']) add('join', v, sep);
}
add('includes', 'abcabc', 'c', 3); add('includes', 'abc', 'bc', -2); add('includes', 'abc', '', 9); add('includes', { a: 1, b: NaN }, NaN);
add('includes', { a: 'x' }, 'x', 1); add('includes', 'a1', 1); add('includes', [1, 2], 2, 1, true);
for (const [a, b, c] of [[[1, 2, 3], [2, 3, 4]], [[1, NaN, -0], [NaN, 0]], ['abc', ['a']], [[1, 2], 'ab', [2]], [{ length: 2, 0: 1, 1: 2 }, [1]], [[1, [2]], [[2]]], [null, [1]], [[3, 2, 1], [1], [2]]]) {
  add('difference', a, b, c); add('union', a, b, c); add('intersection', a, b, c); add('without', a, ...(Array.isArray(b) ? b : [b]));
  add('concat', a, b, c); add('union', b); add('intersection', a); add('difference', a);
}
add('concat'); add('concat', 1, [2, [3]], [[4]]); add('union'); add('intersection', [1, 1, 2], [1, 2], [2, 1]);
add('reverse', [1, 2, 3]); add('reverse', 'ab'); add('reverse', 'a');

// Collections with iteratees.
const colls = [[1, 2, 3, 4], ['a', 'bb', 'ccc'], { a: 1, b: 2, c: 3 }, 'abc', [{ n: 'x', v: 1, t: { k: 1 } }, { n: 'y', v: 2, t: { k: 2 } }, { n: 'x', v: 3 }],
  [[1, 'a'], [2, 'b']], null, 5, [], { length: 2, 0: 'p', 1: 'q' }, [3.2, 1.7, 1.2, 3.9]];
const its = [cb.double, cb.isEven, cb.gt2, cb.argstr, cb.keyOf, cb.isString, cb.truthy, cb.floor, cb.len, undefined, null, 'n', 'v', 't.k', ['n', 'x'], ['v', 2], { n: 'x' },
  { t: { k: 1 } }, 'length', 0, ['t.k', 1], { v: 3, n: 'x' }];
for (const c of colls) for (const it of its) {
  for (const fn of ['map', 'filter', 'reject', 'find', 'every', 'some', 'groupBy', 'countBy', 'keyBy', 'sortBy']) add(fn, c, it);
}
for (const c of colls) {
  add('reduce', c, cb.add); add('reduce', c, cb.add, 10); add('reduce', c, cb.argstr, ''); add('reduce', c, cb.add, undefined);
  add('find', c, cb.truthy, 1); add('find', c, cb.truthy, -1); add('find', c, cb.isEven, -2);
  add('every', c, cb.truthy, { a: 1 }); add('some', c);
  add('orderBy', c, ['v', 'n'], ['desc', 'asc']); add('orderBy', c, 'v', 'desc'); add('orderBy', c, undefined, 'desc');
  add('sortBy', c, ['n', 'v']); add('sortBy', c, 'n', 'v'); add('sortBy', c, [['t', 'k']]); add('orderBy', c, [['t', 'k']], ['desc']);
}
add('map', [1, 2], cb.throws); add('filter', { a: 1 }, cb.throws); add('find', [1], cb.throws);
// Sorting edge cases: mixed types, NaN, null, undefined, stability.
const mixed = [3, 'b', NaN, null, undefined, 1, 'a', 2, NaN, '10', 9, null, -0, 0, '', true, false];
add('sortBy', mixed); add('orderBy', mixed, undefined, 'desc');
for (let i = 0; i < 40; i++) {
  const n = 1 + Math.floor(rnd() * 80);
  const arr = [];
  for (let j = 0; j < n; j++) arr.push(rnd() < 0.7 ? Math.floor(rnd() * 20) : pick(['x', 'y', null, undefined, NaN]));
  const objs = arr.map((v, j) => ({ k: v, j }));
  add('sortBy', i % 2 ? arr.filter(x => typeof x === 'number' && x === x) : arr.slice(0, 63)); add('sortBy', objs.slice(0, 50), 'k');
  add('orderBy', objs.filter(o => typeof o.k === 'number'), ['k'], ['desc']);
}
// Math.
for (const v of [...prims, [1, 2], [3], {}, new Date(5)]) {
  for (const p of [undefined, 0, 1, 2, -1, -2, 1.9, 300, '1', null]) { add('round', v, p); add('floor', v, p); add('ceil', v, p); }
}
for (const x of [4.006, 4.005, 1.005, 2.5, -2.5, 0.5, -0.5, 1e21, 1.23456e-7, 123.456, -123.456, 5e-324, 1.7976931348623157e308, 0.1 + 0.2]) {
  for (const p of [0, 1, 2, 3, -1, -2, 10, 292]) { add('round', x, p); add('floor', x, p); add('ceil', x, p); }
}
for (const a of [-10, -5, 0, 5, 10, NaN, '7', null, undefined, Infinity]) for (const b of [undefined, -5, 0, 5, NaN, '3', null]) {
  add('clamp', a, b); add('clamp', a, b, 5); add('clamp', a, -5, b); add('inRange', a, b); add('inRange', a, b, 7); add('inRange', a, 2, b);
}
add('inRange', -0, 0); add('inRange', 0, -0, 1); add('inRange', 3, 4, 2);
for (const a of [undefined, 0, 1, 4, -4, 2.5, '3', NaN, Infinity]) for (const b of [undefined, 0, 1, 10, -3, '5', 0.5]) for (const c of [undefined, 0, 1, -1, 2, 0.25, '2']) add('range', a, b, c);
add('range', 1, 2, { 1: 2 });
for (const n of [undefined, 0, 1, 3, -1, 2.9, '4', NaN]) for (const it of [undefined, cb.double, cb.argstr, 'n', null]) add('times', n, it);
add('times', 2, cb.throws);
// Objects.
const deep = () => ({ a: { b: [1, { c: 2 }], 'x.y': 3 }, 'a.b': 'flat', arr: [{ k: 'v' }], n: null, u: undefined, s: 'str' });
const paths = ['a', 'a.b', 'a.b[1].c', 'a.b.1.c', ['a', 'b', '1', 'c'], 'a.b[0]', 'a["x.y"]', "a['x.y']", 'a.b', 'arr[0].k', 'n.x', 'u', 's.length', 's[1]', 'missing', '', '.a', 'a..b',
  'a[b]', ['a.b'], [], 0, 'arr.0.k', 'a.b.length', 'constructor', 'toString', '__proto__', 'a.b[-1]', 'a.b[1.5]', 'a\\.b'];
for (const p of paths) {
  add('get', deep(), p); add('get', deep(), p, 'dflt'); add('has', deep(), p); add('pick', deep(), p); add('omit', deep(), p);
  add('map', [deep(), deep()], p); add('get', [[1, 2], 'xy'], p); add('has', 'abc', p);
}
add('get', null, 'a', 1); add('get', undefined, 'a'); add('has', null, 'a'); add('get', { a: 1 }, null); add('get', { null: 7 }, null);
add('pick', deep(), ['a', 's'], 'n'); add('pick', deep(), 'a.b[1].c', 'arr[0]'); add('omit', deep(), ['a.b', 's'], 'arr[0].k'); add('pick', [1, 2, 3], '1', 0);
add('omit', [1, 2, 3], '1'); add('omit', 'abc', 0); add('pick', null, 'a'); add('omit', null, 'a'); add('pick', deep(), 'a.constructor.x');
add('omit', deep(), 'a.__proto__'); add('omit', { __proto__: 1 }, '__proto__');
for (const [a, b] of [[{ a: 1, b: { c: 1 } }, { b: { d: 2 }, e: 3 }], [{ a: [1, 2, 3] }, { a: [4] }], [{ a: [1, { x: 1 }] }, { a: [undefined, { y: 2 }] }],
  [{ a: 1 }, { a: undefined, b: undefined }], [{ a: { x: 1 } }, { a: [9] }], [{ a: [1] }, { a: { 0: 'z', k: 1 } }], [{ a: new Date(1) }, { a: { x: 1 } }],
  [{ a: 'str' }, { a: { x: 1 } }], [{}, { d: new Date(3), r: /x/g, f: cb.double }], [[1, 2], [3]], [{ a: null }, { a: { b: 1 } }],
  [{ 1: 'a', b: 2 }, { 0: 'z', b: 3, 1: undefined }], [{ a: { b: 1 } }, 'ab'], [{ a: 1 }, null], [{}, { constructor: { x: 1 } }], [{}, { constructor: 5 }],
  [{ length: 1, 0: 'a' }, { a: { length: 1, 0: 'q' } }], [{ x: { length: 1, 0: 1 } }, { x: [5, 6] }]]) {
  for (const fn of ['merge', 'assign', 'extend', 'assignIn', 'defaults']) add(fn, JSON.parse(JSON.stringify(a ?? null)) ?? a, b, { z: 1 });
}
add('merge', { a: 1 }, { b: 2 }, { c: 3 }, { a: 4 }); add('merge', null, { a: 1 }); add('merge', 'str', { a: 1 }); add('merge', { a: 1 });
add('defaults', { a: undefined, b: null }, { a: 1, b: 2, c: 3 }, { c: 4, d: 5 }); add('defaults', null, { a: 1 }); add('assign', { a: 1 }, 'xy', 5, null, [9]);
add('merge', {}, { a: 1 }, 'a', [{ a: 1 }]); add('assign', {}, { a: 1 }, 'a', { a: { a: 1 } });
// Equality and cloning.
const eqs = [[1, 1], [NaN, NaN], [0, -0], ['a', 'a'], [[1, [2]], [1, [2]]], [[1, 2], [2, 1]], [{ a: 1, b: 2 }, { b: 2, a: 1 }], [{ a: 1 }, { a: 1, b: undefined }],
  [new Date(1), new Date(1)], [new Date(1), new Date(2)], [/a/g, /a/g], [/a/g, /a/i], [[], {}], [{}, []], [null, undefined], ['1', 1], [{ a: [{ b: NaN }] }, { a: [{ b: NaN }] }],
  [cb.double, cb.double], [cb.double, cb.isEven], [{ f: cb.double }, { f: cb.double }], [[undefined], []], [{ length: 0 }, []]];
for (const [a, b] of eqs) add('isEqual', a, b);
for (const v of [{ a: [1, { b: 2 }], d: new Date(7) }, [[1], [2, [3]]], cb.double, new Error('x')]) { add('clone', v); add('cloneDeep', v); add('cloneDeep', [v]); }
add('constant', 5); add('once', cb.double); add('once', 5);

const cases = record(calls);
fs.writeFileSync(process.argv[2], JSON.stringify(cases));
console.log(cases.length);
