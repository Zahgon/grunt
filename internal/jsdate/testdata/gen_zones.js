// Records V8's local-time behaviour in several time zones for internal/jsdate
// (zones.json). Run with: node testdata/gen_zones.js > testdata/zones.json
// Node resets its time zone cache whenever process.env.TZ changes.
"use strict";
const zones = ["UTC", "America/New_York", "Europe/Paris", "Europe/Dublin", "Europe/London", "Asia/Kolkata",
  "Asia/Kathmandu", "Australia/Sydney", "America/St_Johns", "America/Sao_Paulo", "Etc/GMT+5", "Pacific/Chatham"];
const instants = [0, -1, 1704189600000, 1719914400000, Date.UTC(1950, 5, 1), Date.UTC(1800, 0, 1, 12),
  Date.UTC(2100, 6, 4, 3, 4, 5), Date.UTC(-50, 2, 3), 8.64e15, -8.64e15, Date.UTC(2024, 2, 10, 6, 59, 59, 999),
  Date.UTC(2024, 2, 10, 7), Date.UTC(2024, 10, 3, 5, 30), Date.UTC(2024, 10, 3, 6, 30), Date.UTC(2024, 2, 31, 0, 59),
  Date.UTC(2024, 2, 31, 1), Date.UTC(2024, 9, 27, 0, 30), Date.UTC(2024, 9, 27, 1, 30), Date.UTC(2024, 3, 6, 15, 30)];
const locals = [[2024, 0, 2, 10], [2024, 2, 10, 2, 30], [2024, 10, 3, 1, 30], [2024, 2, 31, 2, 30], [2024, 9, 27, 1, 30],
  [2024, 9, 27, 2, 30], [2024, 3, 7, 2, 30], [2024, 8, 8, 0, 0], [99, 0, 1], [1800, 0, 1], [275760, 8, 13], [275760, 8, 12, 23],
  [-271821, 3, 20], [-271821, 3, 19, 23]];
const strings = ["2024-01-02T10:00", "2024-01-02", "Jan 2 2024 10:00", "2024/03/10 02:30", "2024-11-03T01:30:00",
  "Tue Jan 02 2024 10:00:00 GMT+0000 (Coordinated Universal Time)", "1/2/2024", "+275760-09-13T00:00"];
const num = (v) => (Number.isNaN(v) ? null : v);
const out = [];
for (const zone of zones) {
  process.env.TZ = zone;
  const z = { zone, instants: [], locals: [], parse: [] };
  for (const t of instants) {
    const d = new Date(t);
    z.instants.push([t, String(d), d.getTimezoneOffset(), [d.getFullYear(), d.getMonth(), d.getDate(), d.getHours(),
      d.getMinutes(), d.getSeconds(), d.getMilliseconds(), d.getDay()], d.toLocaleString()]);
  }
  for (const a of locals) z.locals.push([a, num(new Date(...a).getTime())]);
  for (const s of strings) z.parse.push([s, num(Date.parse(s))]);
  out.push(z);
}
process.stdout.write(JSON.stringify(out) + "\n");
