// Records differential fixtures for internal/log from grunt-legacy-log 3.0.1 and
// Node's util.format. Run with Node 24 and grunt's node_modules mounted at /g/node_modules:
//
//   docker run --rm -e TZ=UTC -v <grunt>/node_modules:/g/node_modules:ro -v $PWD:/w -w /w \
//     node:24-bookworm node gen.js > cases.json
'use strict';

const util = require('util');
const colors = require('/g/node_modules/colors');
const chalk = require('/g/node_modules/chalk');
const {Log} = require('/g/node_modules/grunt-legacy-log');

// mulberry32, so the fixtures are reproducible.
let seed = process.env.SEED ? Number(process.env.SEED) : 0x5eed1e55;
function rand() {
  seed |= 0; seed = seed + 0x6D2B79F5 | 0;
  let t = Math.imul(seed ^ seed >>> 15, 1 | seed);
  t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t;
  return ((t ^ t >>> 14) >>> 0) / 4294967296;
}
const int = (n) => Math.floor(rand() * n);
const pick = (a) => a[int(a.length)];
const chance = (p) => rand() < p;

// ---- value encoding shared with the Go tests -------------------------------------

function encode(v) {
  const ids = new Map();
  let next = 1;
  const seen = new Set();
  // First pass: find objects reached twice on one path (cycles) or shared.
  (function scan(x) {
    if (x === null || typeof x !== 'object' || Buffer.isBuffer(x) || x instanceof Date || x instanceof RegExp) return;
    if (seen.has(x)) { if (!ids.has(x)) ids.set(x, next++); return; }
    seen.add(x);
    if (x instanceof Error) { Object.keys(x).forEach((k) => scan(x[k])); return; }
    if (Array.isArray(x)) x.forEach(scan); else Object.keys(x).forEach((k) => scan(x[k]));
  })(v);
  const emitted = new Set();
  return (function enc(x) {
    if (x === undefined) return {u: 1};
    if (x === null || typeof x === 'boolean' || typeof x === 'string') return x;
    if (typeof x === 'number') return {n: Object.is(x, -0) ? '-0' : String(x)};
    if (typeof x === 'function') return {fn: x.name};
    if (Buffer.isBuffer(x)) return {b: x.toString('hex')};
    if (x instanceof Date) return {d: String(x.getTime())};
    if (x instanceof RegExp) return {r: x.source, f: x.flags};
    if (ids.has(x) && emitted.has(x)) return {ref: ids.get(x)};
    emitted.add(x);
    const id = ids.get(x);
    let out;
    if (x instanceof Error) out = {e: x.name, m: x.message, p: Object.keys(x).map((k) => [k, enc(x[k])])};
    else if (Array.isArray(x)) out = {a: x.map(enc)};
    else out = {o: Object.keys(x).map((k) => [k, enc(x[k])])};
    if (id) out.id = id;
    return out;
  })(v);
}

// ---- random values -----------------------------------------------------------------

const charPool = ['a', 'b', 'Z', '0', '9', ' ', ' ', '_', '*', '-', '.', ',', '!', '?', '%', '$', '{', '}',
  "'", '"', '`', '\\', '\n', '\t', '\r', '\x00', '\x1b', '\x7f', '\x85', ' ', ' ', 'é', '中', '😀',
  '\x1b[31m', '\x1b[39m', '\x1b[4m'];
function randString(max = 12) {
  let s = '';
  const n = int(max + 1);
  for (let i = 0; i < n; i++) s += pick(charPool);
  return s;
}
function wordString(words, spacey) {
  const w = ['lorem', 'ipsum', 'a', 'dolor', 'supercalifragilistic', 'x', 'sit', 'amet,', 'end.', '_u_', '*b*',
    '\x1b[32mgreen\x1b[39m', 'tab\tbed', 'line\nbreak', '😀😀', '中文', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'];
  const parts = [];
  for (let i = 0; i < words; i++) parts.push(pick(w));
  return parts.join(spacey ? pick([' ', '  ', ' ']) : ' ');
}
const numberPool = [0, -0, 1, -1, 1.5, -2.25, 42, 1e21, 1e-7, 123456789, NaN, Infinity, -Infinity, 0.1 + 0.2, 2 ** 53, 7, 100];
const keyPool = ['a', 'b', 'foo', 'bar_baz', '_x', 'A1', '1', '10', '2', 'a-b', 'it\'s', 'with space', '', '__proto__', 'é', 'say "hi"'];

function randValue(depth, opts) {
  const r = rand();
  if (depth <= 0 || r < 0.45) {
    const p = int(9);
    if (p < 3) return randString();
    if (p < 6) return pick(numberPool);
    if (p === 6) return pick([true, false]);
    if (p === 7) return pick([null, undefined]);
    return int(100);
  }
  const k = int(opts.noFunctions ? 6 : 7);
  switch (k) {
    case 0: case 1: {
      const n = chance(0.15) ? 7 + int(30) : int(6);
      const kind = int(3);
      const a = [];
      for (let i = 0; i < n; i++) a.push(kind === 0 ? int(1000) * pick([1, -1, 0.5]) : kind === 1 ? 'x'.repeat(int(8)) + i : randValue(depth - 1, opts));
      return a;
    }
    case 2: case 3: {
      const o = {};
      const n = int(5);
      for (let i = 0; i < n; i++) {
        Object.defineProperty(o, pick(keyPool), {value: randValue(depth - 1, opts), enumerable: true, writable: true, configurable: true});
      }
      return o;
    }
    case 4: return pick([new Date(0), new Date(1700000000123), new Date(NaN), new Date(-60589296000000), new Date(253402300800000), Buffer.from('ab'), Buffer.alloc(0), Buffer.from('x'.repeat(55))]);
    case 5: {
      if (chance(0.5)) return pick([/a+b/g, /x/, /[\]\/]/im]);
      const e = new (pick([Error, TypeError, RangeError]))(pick(['boom', '', 'multi\nline']));
      e.stack = e.message ? e.name + ': ' + e.message : e.name;
      if (chance(0.4)) e.code = randValue(depth - 1, opts);
      return e;
    }
    default: return pick([function named() {}, function () {}, () => {}]);
  }
}

function withCycle(v) {
  if (v && typeof v === 'object' && !Buffer.isBuffer(v) && !(v instanceof Date) && !(v instanceof RegExp) && !(v instanceof Error)) {
    if (Array.isArray(v)) v.push(v); else v.self = {back: v};
  }
  return v;
}

// ---- util.format cases -------------------------------------------------------------

function randFormat() {
  const tokens = ['%s', '%d', '%i', '%f', '%j', '%o', '%O', '%c', '%%', '%x', '%', 'text ', ' ', 'a%', '%5', '😀'];
  let s = '';
  const n = 1 + int(5);
  for (let i = 0; i < n; i++) s += pick(tokens);
  return s;
}

const formatCases = [];
const formatOne = (args) => formatCases.push({args: args.map(encode), out: util.format(...args)});
for (let i = 0; i < 700; i++) {
  const fmt = randFormat();
  const hasO = fmt.includes('%o');
  const opts = {noFunctions: hasO || fmt.includes('%s')};
  const args = [chance(0.9) ? fmt : randValue(2, opts)];
  const n = int(5);
  for (let j = 0; j < n; j++) {
    let v = randValue(chance(0.2) ? 4 : 2, opts);
    if (chance(0.05)) v = withCycle(v);
    args.push(v);
  }
  formatOne(args);
}
// Hand-picked edge cases.
[
  ['%s', -0], ['%d', '-0'], ['%i', '  0x1Fz'], ['%i', '-0'], ['%f', '  .5e-3x'], ['%f', 'Infinityx'], ['%f', '-0x'],
  ['%d', [7]], ['%d', {}], ['%d', Buffer.from('12')], ['%i', 1e21], ['%s %s', 'a'], ['%%'], ['%% %s', 'a'], ['a%', 'b'],
  ['%j', undefined], ['%j', function f() {}], ['%j', withCycle({a: 1})], ['%s', new Date(0)], ['%s', Buffer.from('ab')],
  ['%O', 'x'.repeat(10003)], ['%O', 'abc\n'.repeat(40)], ['%o', []], ['%o', {}], ['%O', [[]]], ['%o', Array.from({length: 30}, (_, i) => i)],
  [Array.from({length: 101}, (_, i) => i)], [Array.from({length: 120}, (_, i) => 'x' + i)], [Array.from({length: 26}, (_, i) => 'abcdefghijklmnopqrstuvwxyz'.slice(0, i))],
  [{a: {b: {c: {d: {}}}}}], [[[[[[]]]]]], [{a: [{b: [{c: [1]}]}]}], [{s: 'a\nb'.repeat(50)}], [['a'.repeat(20), 'b'.repeat(130)]],
  [Buffer.alloc(60)], [new Date(NaN), new Date(-1e14)], ['%O', 'it\'s "x" `y`'], ['%O', 'it\'s "x" ${y}'], ['%O', 'it\'s "x"'],
  [{'a-b': 1, '1': 2, _x: 3, 'it\'s': 4}], [(() => { const a = {x: {}}; a.x.y = a; a.z = [a.x]; return a; })()],
  [['中文字符', '中', 'ab', 'c', 'd', 'e', 'f', 'g']], ['%s', {a: {b: 1}}], ['%s', [1, [2, [3]]]], [1.5, -0, NaN, 'str'],
  ['%c%s', 'x', 'y'], [], [undefined], [null, 'a'], ['%s'],
].forEach(formatOne);

// ---- markup, wraptext, table -------------------------------------------------------

const markupPool = ['_', '*', ' ', 'a', 'b', ',', '.', '!', '?', '\n', '\t', ' ', '😀', '-', 'x'];
const markupCases = [];
for (let i = 0; i < 600; i++) {
  let s = '';
  const n = int(14);
  for (let j = 0; j < n; j++) s += pick(markupPool);
  const color = chance(0.3);
  colors.enabled = color;
  markupCases.push({in: s, color, out: Log.prototype._markup(s)});
}
colors.enabled = false;

const wrapCases = [];
for (let i = 0; i < 400; i++) {
  const width = 1 + int(chance(0.7) ? 30 : 90);
  const text = chance(0.5) ? wordString(int(25), true) : randString(60);
  wrapCases.push({width, text, out: Log.prototype.wraptext(width, text)});
}
const tableCases = [];
for (let i = 0; i < 200; i++) {
  const cols = 1 + int(4);
  const widths = [], texts = [];
  for (let c = 0; c < cols; c++) {
    widths.push(1 + int(25));
    texts.push(chance(0.2) ? '' : wordString(int(10), true));
  }
  tableCases.push({widths, texts, out: Log.prototype.table(widths, texts)});
}

// ---- logger sequences --------------------------------------------------------------

function makeLogger(gruntMode, color, opts) {
  const chunks = [];
  const out = {write: (s) => chunks.push(s)};
  const data = {verbose: opts.verbose, debug: opts.debug, color: opts.color};
  const fail = {errorcount: 0};
  const grunt = {
    option(key) {
      const no = key.match(/^no-(.+)$/);
      return no ? data[no[1]] === false : data[key];
    },
    fail,
  };
  const log = gruntMode
    ? new Log({grunt, outStream: out, maxCols: opts.maxCols})
    : new Log({outStream: out, verbose: opts.verbose, debug: opts.debug, color: opts.color, maxCols: opts.maxCols});
  return {log, chunks, data, fail};
}

const targets = ['log', 'verbose', 'notverbose', 'verbose.or', 'notverbose.or', 'verbose.always'];
function target(log, name) {
  return name.split('.').reduce((l, p) => (p === 'log' ? l : l[p]), log);
}
const msgMethods = ['write', 'writeln', 'warn', 'error', 'ok', 'errorlns', 'oklns', 'success', 'fail', 'header', 'subhead', 'debug', 'writelns'];

function randMsgArgs() {
  const n = pick([0, 1, 1, 1, 2, 3]);
  const args = [];
  for (let i = 0; i < n; i++) {
    if (i === 0 && chance(0.7)) args.push(chance(0.3) ? randFormat() + wordString(int(4), false) : wordString(int(30), true));
    else args.push(randValue(2, {noFunctions: true}));
  }
  return args;
}

function randFlags() {
  switch (int(6)) {
    case 0: return ['--verbose', '--gruntfile=/x/Gruntfile.js'].slice(0, int(3));
    case 1: return [];
    case 2: return 'string';
    case 3: return undefined;
    case 4: return chance(0.1) ? withCycle({a: 1}) : randValue(3, {noFunctions: true});
    default: {
      const o = {};
      const n = int(6);
      for (let i = 0; i < n; i++) o[pick(keyPool.filter((k) => k !== '__proto__'))] = pick([true, false, 'str', 1, null, undefined, [1, 'a'], {x: true}, new Date(0), function f() {}, NaN, 'q"\n']);
      return o;
    }
  }
}

const logCases = [];
for (let c = 0; c < 400; c++) {
  const gruntMode = chance(0.6);
  const color = chance(0.2);
  colors.enabled = color;
  chalk.level = color ? 1 : 0;
  const opts = {verbose: chance(0.5), debug: chance(0.3) ? pick([true, 1, 0]) : false, color: chance(0.7) ? false : true, maxCols: chance(0.2) ? 10 + int(60) : null};
  const {log, chunks, data, fail} = makeLogger(gruntMode, color, opts);
  const ops = [];
  const n = 1 + int(14);
  for (let i = 0; i < n; i++) {
    const r = rand();
    const t = pick(targets);
    const l = target(log, t);
    if (r < 0.08) {
      const muted = chance(0.5);
      ops.push({muted});
      log.muted = muted;
    } else if (r < 0.16) {
      const key = pick(['verbose', 'debug', 'color']);
      const val = pick([true, false]);
      ops.push({set: key, val});
      if (gruntMode) data[key] = val; else log.options[key] = val;
    } else if (r < 0.24) {
      const flags = randFlags();
      const prefix = pick([undefined, 'Options', 'Command-line options', '']);
      const op = {t, m: 'writeflags', a: [encode(flags)], prefix: prefix || ''};
      try { l.writeflags(flags, prefix); } catch (e) { op.throws = e.message; }
      ops.push(op);
    } else if (r < 0.30) {
      const widths = [1 + int(20), 1 + int(20), 1 + int(30)];
      const texts = [wordString(int(6), true), '', wordString(int(12), true)];
      ops.push({t, m: 'writetableln', widths, texts});
      l.writetableln(widths, texts);
    } else if (r < 0.34) {
      const items = [randValue(1, {noFunctions: true}), 'b', randString(4)];
      ops.push({t, m: 'wordlist', a: items.map(encode), ret: l.wordlist(items)});
    } else {
      const m = pick(msgMethods);
      const args = randMsgArgs();
      ops.push({t, m, a: args.map(encode)});
      l[m].apply(l, args);
    }
  }
  logCases.push({grunt: gruntMode, color, opts: {verbose: opts.verbose, debug: encode(opts.debug), color: opts.color, maxCols: opts.maxCols || 0}, ops, out: chunks.join(''), errorcount: fail.errorcount, hasLogged: log.hasLogged});
}

process.stdout.write(JSON.stringify({format: formatCases, markup: markupCases, wrap: wrapCases, table: tableCases, log: logCases}));
