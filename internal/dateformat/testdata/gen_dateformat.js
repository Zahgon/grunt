// Records dateformat 4.6.3 output for internal/dateformat fixtures. `new Date()`
// without arguments is pinned to a per-case "now".
const fs = require('fs');
const RealDate = Date;
let NOW = 0;
global.Date = class extends RealDate {
  constructor(...a) { if (a.length === 0) super(NOW); else super(...a); }
};
const df = require('dateformat');
let seed = 4242;
function rnd() { seed = (seed * 1103515245 + 12345) & 0x7fffffff; return seed / 0x80000000; }
function pick(a) { return a[Math.floor(rnd() * a.length)]; }
const named = Object.keys(df.masks);
const tokens = ['d', 'dd', 'ddd', 'dddd', 'DDD', 'DDDD', 'D', 'DD', 'DDDDD', 'm', 'mm', 'mmm', 'mmmm', 'mmmmm', 'y', 'yy', 'yyy', 'yyyy', 'yyyyy', 'h', 'hh', 'hhh', 'H', 'HH', 'M', 'MM', 's', 'ss', 'l', 'L', 't', 'tt', 'T', 'TT', 'Z', 'o', 'p', 'S', 'W', 'WW', 'WWW', 'N', '"quoted d"', "'it''s'", '"', "'", ' ', '-', ':', '/', 'x', 'é', ',', 'UTC:', 'GMT:'];
function enc(v) {
  if (v === undefined) return { t: 'undef' };
  if (v === null) return { t: 'null' };
  if (v instanceof RealDate) return { t: 'date', v: Number.isNaN(v.getTime()) ? 'NaN' : v.getTime() };
  if (typeof v === 'number') return { t: 'num', v: Number.isNaN(v) ? 'NaN' : v === Infinity ? 'Infinity' : v === -Infinity ? '-Infinity' : v };
  if (typeof v === 'string') return { t: 'str', v };
  if (typeof v === 'boolean') return { t: 'bool', v };
  if (Array.isArray(v)) return { t: 'arr', v: v.map(enc) };
  if (typeof v === 'object') return { t: 'obj' };
  throw new Error('unencodable');
}
function randTime() {
  const r = rnd();
  if (r < 0.35) return Math.floor((rnd() * 2 - 1) * 8.64e15);
  if (r < 0.55) return Math.floor((rnd() * 2 - 1) * 6.4e13); // years about -60 to 4000
  if (r < 0.7) return Math.floor(-62198755200000 + (rnd() * 2 - 1) * 4e12); // around year 0
  if (r < 0.8) return pick([8.64e15, -8.64e15, 8.64e15 - 1, -8.64e15 + 1, 8.64e15 - 864e5 * 3, -8.64e15 + 864e5 * 2]);
  return Math.floor(1.6e12 + rnd() * 2e11);
}
function randMask() {
  const r = rnd();
  if (r < 0.25) return pick(named);
  if (r < 0.3) return pick([undefined, null, '', 0, 5, 'constructor', '__proto__', 'toString', 'nope', 'UTC:isoDate', 'GMT:default']);
  let s = '';
  const n = 1 + Math.floor(rnd() * 6);
  for (let i = 0; i < n; i++) s += pick(tokens);
  return s;
}
const cases = [];
function record(date, mask, utc, gmt, now) {
  NOW = now;
  let out, err = null;
  try { out = df(date, mask, utc, gmt); } catch (e) { err = String(e); }
  cases.push({ date: enc(date), mask: enc(mask), utc: !!utc, gmt: !!gmt, now, out: err === null ? out : null, err });
}
const allTokens = 'd dd ddd dddd DDD DDDD m mm mmm mmmm yy yyyy h hh H HH M MM s ss l L t tt T TT Z o p S W WW N';
for (let i = 0; i < 3500; i++) {
  const t = randTime();
  const now = rnd() < 0.3 ? t + pick([0, -864e5, 864e5, 3600e3, -7200e3, 2 * 864e5, 864e5 - 1]) : 1.7e12;
  const r = rnd();
  const mask = r < 0.4 ? allTokens : randMask();
  let date = new Date(t);
  const k = rnd();
  if (k < 0.15) date = t;
  else if (k < 0.2) date = new Date(t).toISOString();
  record(date, mask, rnd() < 0.2, rnd() < 0.1, Math.abs(now) <= 8.64e15 ? now : 1.7e12);
}
const odd = [undefined, null, 0, -0, '', NaN, false, true, 1, 1.5, -1.5, 8.64e15 + 1, Infinity, 'isoDate', 'shortTime', 'mmmm d', 'Jan 2 2024', '2024-01-02', 'garbage 1', 'nodigits', [2024, 1, 2], [], ['2024-01-02'], {}, new Date(NaN), '1e3', '  2024  '];
for (const d of odd) for (const m of [undefined, 'isoDateTime', 'dddd DDDD W', 'UTC:h:MM TT Z o p']) record(d, m, false, false, 1700000000000);
fs.writeFileSync(process.argv[2], JSON.stringify(cases));
console.log(cases.length);
