// Records V8 Date behaviour for internal/jsdate fixtures.
const fs = require('fs');
let seed = 12345;
function rnd() { seed = (seed * 1103515245 + 12345) & 0x7fffffff; return seed / 0x80000000; }
function pick(a) { return a[Math.floor(rnd() * a.length)]; }
function num(v) { return Number.isNaN(v) ? null : (Object.is(v, -0) ? 0 : v); }

const curated = [
  '', ' ', 'x', '2024', '2024-01', '2024-01-02', '2024-01-02T10:00', '2024-01-02T10:00:00', '2024-01-02T10:00:00Z',
  '2024-01-02T10:00:00.123Z', '2024-01-02T10:00:00.1Z', '2024-01-02T10:00:00.12345678901Z', '2024-01-02T10:00:00+05:30',
  '2024-01-02T10:00:00-0800', '2024-01-02T24:00', '2024-01-02T24:00:01', '2024-01-02T25:00', '2024-13-01', '2024-02-30',
  '2024-00-10', '+275760-09-13T00:00:00.000Z', '+275760-09-13T00:00:00.001Z', '-271821-04-20T00:00:00.000Z',
  '-271821-04-19T23:59:59.999Z', '-000000-01-01', '+000000-01-01', '-000001-01-01', '+002024-01-02', '0000-01-01', '0099-01-01',
  'Jan 2 2024', 'Jan 2, 2024', '2 Jan 2024', 'January 2, 2024 10:00:00', 'Tue, 02 Jan 2024 10:00:00 GMT',
  'Tue Jan 02 2024 10:00:00 GMT+0000 (Coordinated Universal Time)', 'Tue Jan 02 2024 10:00:00 GMT-0800 (PST)',
  '2024/01/02', '2024/01/02 10:00', '2024/1/2 10:00:00.5', '1/2/2024', '1/2/24', '1/2/50', '1/2', 'Jan 5', 'March 7',
  '1.2.2024', '10:00', '10:00 2024-01-02', '2024-01-02 10:00', '2024-01-02 10:00Z', '2024-01-02 10:00 PM', '12:30 am Jan 1 2000',
  '13:00 pm 1/1/2000', 'Thu, 01 Jan 1970 00:00:00 GMT+0100', 'Thu, 01 Jan 1970 00:00:00 UTC+1', 'Thu, 01 Jan 1970 00:00:00 EST',
  'Thu, 01 Jan 1970 00:00:00 PDT', 'Jan 1 1970 00:00:00 z', 'foo 2024', 'foo2024', '2024 foo', '(comment) Jan 1 2000',
  'Jan (x(y)z) 1 2000', 'Jan 1 2000)', '-1', '-2024', '+2024', '2024-01-02T10:00:00 Z', 'T10:00', '2024T10:00',
  '2024-01T10:00Z', '1970-01-01T00:00:00.000+23:59', '1970-01-01T00:00:00.000+24:00', 'Jan 1 2000 10:00 +999999:',
  'Jan 1 2000 10:00 +99999:', 'Jan 1 2000 10:00 GMT+12345', 'Jan 1 2000 10:00 GMT-8', 'Jan 1 2000 10:00 GMT+530',
  'Jan 1 2000 10::', 'Jan 1 2000 10::30', 'Jan 1 2000 10:30:', 'Jan 1 2000 10:30:15.', 'Jan 1 2000 10.30', 'Jan 1 2000 10:30.5',
  '2000-01-01\u00002', 'Jan 1 2000', 'Jan 1 2000', '１/2/2000', 'Jan 1 2000 😀', '2024-01-02T10:00:00.Z',
  '12/31/1969 23:59:59.999', '99999-01-01', '123456789012 Jan', 'Jan 1 000000000002000', 'Sat, 1 Jan 2000', 'sunday jan 1 2000',
  'Jan 32 2000', 'Feb 29 2001', 'Feb 29 2000', '2000 Jan 1', '1 2000 Jan', '31/12/2000', '12/31/2000', '0/1/2000',
];
const parts = ['2024', '1970', '0', '00', '01', '1', '2', '12', '13', '24', '31', '32', '59', '60', '99', '100', '275760', '-', '+', '/', ':', '.', ',', ' ', ' ', '  ', 'T', 'Z', 'z', 'GMT', 'UTC', 'UT', 'EST', 'pdt', 'am', 'PM', 'Jan', 'feb', 'MARCH', 'Sept', 'Dec', 'Mon', 'Tuesday', 'x', '(', ')', '(c)', '123', '1234', '12345', '123456', '0001', '000123', '\t', '\n', 'é', '_'];
const strings = new Set(curated);
// Structured ISO-ish strings.
const isoYears = ['1970', '2024', '0000', '9999', '+012345', '-000001', '+275760', '-271821', '1600'];
for (let i = 0; i < 1500; i++) {
  let s = pick(isoYears);
  if (rnd() < 0.8) s += '-' + pick(['01', '02', '06', '12', '13', '00', '1']);
  if (rnd() < 0.7) s += '-' + pick(['01', '15', '29', '30', '31', '32', '00', '5']);
  if (rnd() < 0.7) {
    s += pick(['T', 't', ' ']) + pick(['00', '10', '23', '24', '25', '7']) + ':' + pick(['00', '30', '59', '60']);
    if (rnd() < 0.6) s += ':' + pick(['00', '45', '59', '60']);
    if (rnd() < 0.4) s += '.' + pick(['0', '1', '12', '123', '1234', '000001', '999', '5']);
    if (rnd() < 0.6) s += pick(['Z', 'z', '+05:30', '-08:00', '+0530', '-1200', '+24:00', '+23:60', '+5:30', ' GMT', '']);
  }
  strings.add(s);
}
// Legacy-style combinations.
const months = ['Jan', 'February', 'mar', 'APR', 'May', 'June', 'jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec', 'Foo'];
const days = ['Mon', 'Tue,', 'Wednesday', 'thu', ''];
for (let i = 0; i < 1500; i++) {
  const d = String(Math.floor(rnd() * 34)), y = pick(['2024', '1999', '70', '49', '50', '0', '12345', '1', '275760', '275761']);
  const m = pick(months), mn = String(Math.floor(rnd() * 14));
  const time = rnd() < 0.6 ? ' ' + Math.floor(rnd() * 26) + ':' + String(Math.floor(rnd() * 61)).padStart(2, '0') + (rnd() < 0.5 ? ':' + Math.floor(rnd() * 61) : '') + (rnd() < 0.3 ? pick([' am', ' PM', 'pm']) : '') : '';
  const zone = rnd() < 0.4 ? pick([' GMT', ' UTC', ' GMT+0100', ' -0500', ' +01:00', ' (UTC)', ' EST', ' z', ' +5', '+1']) : '';
  let s;
  switch (Math.floor(rnd() * 6)) {
    case 0: s = pick(days) + ' ' + m + ' ' + d + ' ' + y + time + zone; break;
    case 1: s = pick(days) + ' ' + d + ' ' + m + ' ' + y + time + zone; break;
    case 2: s = mn + '/' + d + '/' + y + time + zone; break;
    case 3: s = y + '/' + mn + '/' + d + time + zone; break;
    case 4: s = y + '-' + mn + '-' + d + time + zone; break;
    default: s = m + '-' + d + '-' + y + time + zone;
  }
  strings.add(s);
}
// Random token soup.
for (let i = 0; i < 2500; i++) {
  let s = '';
  const n = 1 + Math.floor(rnd() * 8);
  for (let j = 0; j < n; j++) s += pick(parts);
  strings.add(s);
}
const parse = [...strings].map(s => [s, num(Date.parse(s))]);

// Date.UTC
const vals = [0, 1, -1, 99, 100, 1970, 2024, -271821, 275760, 275761, 1e6, 1e6 + 1, -1e6, 1.5, -0.5, 49.9, 99.5, NaN, Infinity, -Infinity, 11, 12, 13, -13, 1e7, -1e7, 1e8, 31, 32, 365000000, -365000000, 3.6e6, 8.64e15];
const utc = [];
for (let i = 0; i < 3000; i++) {
  const a = [pick(vals), pick(vals), pick(vals), pick(vals), pick(vals), pick(vals), pick(vals)];
  if (rnd() < 0.5) { a[2] = 1 + Math.floor(rnd() * 31); a[3] = Math.floor(rnd() * 24); a[4] = a[5] = a[6] = 0; }
  if (rnd() < 0.3) { a[0] = Math.floor(rnd() * 600000) - 300000; a[1] = Math.floor(rnd() * 12); }
  const r = Date.UTC(...a);
  utc.push([a.map(v => Number.isNaN(v) ? 'NaN' : v === Infinity ? 'Infinity' : v === -Infinity ? '-Infinity' : v), num(r)]);
}
// toISOString and fields.
const times = [0, -1, 1, 8.64e15, -8.64e15, 8.64e15 - 1, -62198755200000, -62198755200001, 253402300799999, 253402300800000, -62167219200000, -62167219200001, 951782400000, NaN];
for (let i = 0; i < 3000; i++) {
  const r = rnd();
  times.push(r < 0.5 ? Math.floor((rnd() * 2 - 1) * 8.64e15) : r < 0.8 ? Math.floor((rnd() * 2 - 1) * 1e13) : Math.floor(rnd() * 4e12));
}
const iso = times.map(t => {
  const d = new Date(t);
  if (Number.isNaN(t)) return [null, null];
  return [t, d.toISOString(), [d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate(), d.getUTCHours(), d.getUTCMinutes(), d.getUTCSeconds(), d.getUTCMilliseconds(), d.getUTCDay()]];
});
fs.writeFileSync(process.argv[2], JSON.stringify({ parse, utc, iso }));
console.log(parse.length, utc.length, iso.length);
