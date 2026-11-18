// Shared fixture encoding for internal/lodash: values are tagged so that undefined,
// NaN, -0, key order, dates, regexps and named callbacks survive JSON.
const _ = require('grunt-legacy-util')._;

// Named callbacks; internal/lodash/fixture_test.go implements the same set.
const callbacks = {
  double: x => x * 2,
  isEven: x => x % 2 == 0,
  gt2: x => x > 2,
  negate: x => -x,
  floor: Math.floor,
  upper: s => String(s).toUpperCase(),
  len: s => String(s).length,
  add: (a, b) => a + b,
  argstr: (...a) => a.map(String).join('|'),
  keyOf: (v, k) => k,
  isString: x => typeof x === 'string',
  truthy: x => x,
  throws: () => { throw new Error('boom'); },
  wrapUpper: (m, ...rest) => '<' + String(m).toUpperCase() + '>',
};

function enc(v) {
  if (v === undefined) return { t: 'u' };
  if (v === null) return { t: 'n' };
  switch (typeof v) {
    case 'boolean': return { t: 'b', v };
    case 'string': return { t: 's', v };
    case 'number':
      if (Number.isNaN(v)) return { t: 'd', v: 'NaN' };
      if (v === Infinity) return { t: 'd', v: 'Infinity' };
      if (v === -Infinity) return { t: 'd', v: '-Infinity' };
      if (Object.is(v, -0)) return { t: 'd', v: '-0' };
      return { t: 'd', v };
    case 'function': {
      for (const k in callbacks) if (callbacks[k] === v) return { t: 'f', v: k };
      return { t: 'f', v: '' };
    }
  }
  if (Array.isArray(v)) {
    const items = [];
    for (let i = 0; i < v.length; i++) items.push(i in v ? enc(v[i]) : { t: 'u' });
    return { t: 'a', v: items };
  }
  if (v instanceof Date) return { t: 'D', v: Number.isNaN(v.getTime()) ? 'NaN' : v.getTime() };
  if (v instanceof RegExp) return { t: 'r', v: [v.source, v.flags] };
  if (v instanceof Error) return { t: 'e', v: [v.name, v.message] };
  return { t: 'o', v: Object.keys(v).map(k => [k, enc(v[k])]) };
}

function dec(e) {
  switch (e.t) {
    case 'u': return undefined;
    case 'n': return null;
    case 'b': case 's': return e.v;
    case 'd': return typeof e.v === 'string' ? (e.v === '-0' ? -0 : Number(e.v)) : e.v;
    case 'f': return callbacks[e.v];
    case 'a': return e.v.map(dec);
    case 'D': return new Date(typeof e.v === 'string' ? NaN : e.v);
    case 'r': return new RegExp(e.v[0], e.v[1]);
    case 'e': { const err = new Error(e.v[1]); err.name = e.v[0]; return err; }
    case 'o': { const o = {}; for (const [k, v] of e.v) o[k] = dec(v); return o; }
  }
  throw new Error('bad tag ' + e.t);
}

// record calls _[fn](...args) for every [fn, args] pair (args given as plain JS
// values) and returns fixture cases. `after` captures the arguments after the call,
// for functions that mutate them.
function record(calls) {
  const cases = [];
  for (const [fn, args] of calls) {
    const encArgs = args.map(enc);
    const live = encArgs.map(dec);
    const c = { fn, args: encArgs };
    try { c.out = enc(_[fn](...live)); } catch (e) { c.err = String(e); }
    c.after = live.map(enc);
    cases.push(c);
  }
  return cases;
}

// Deterministic PRNG for argument generation.
let seed = 99;
function rnd() { seed = (seed * 1103515245 + 12345) & 0x7fffffff; return seed / 0x80000000; }
function pick(a) { return a[Math.floor(rnd() * a.length)]; }

module.exports = { _, callbacks, enc, dec, record, rnd, pick };
