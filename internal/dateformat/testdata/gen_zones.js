// Records dateformat 4.6.3 output in several time zones for internal/dateformat
// (zones.json). `new Date()` without arguments is pinned to each case's "now", and
// Node resets its time zone cache whenever process.env.TZ changes.
// Run with NODE_PATH pointing at a node_modules holding dateformat 4.6.3:
//   node testdata/gen_zones.js > testdata/zones.json
"use strict";
const RealDate = Date;
let NOW = 0;
global.Date = class extends RealDate {
  constructor(...a) { if (a.length === 0) super(NOW); else super(...a); }
};
const df = require("dateformat");
let seed = 777;
function rnd() { seed = (seed * 1103515245 + 12345) & 0x7fffffff; return seed / 0x80000000; }
function pick(a) { return a[Math.floor(rnd() * a.length)]; }
const zones = ["America/New_York", "Europe/Dublin", "Asia/Kolkata", "Australia/Sydney", "America/St_Johns", "Etc/GMT-14"];
const masks = ["default", "longTime", "isoDateTime", "expiresHeaderFormat", "isoUtcDateTime", "Z o p", "W WW N",
  "DDD DDDD d m yyyy H:MM", "UTC:Z o p W N DDDD", "GMT:Z o", "dd mmm yyyy HH:MM:ss.l Z", "h:MM TT Z"];
const fixed = [Date.UTC(2024, 0, 1), Date.UTC(2024, 11, 30, 23), Date.UTC(2024, 2, 10, 7), Date.UTC(2024, 9, 27, 1),
  Date.UTC(2021, 0, 3, 12), Date.UTC(1800, 5, 1), Date.UTC(1970, 0, 1), Date.UTC(2100, 0, 1)];
const out = [];
for (const zone of zones) {
  process.env.TZ = zone;
  const cases = [];
  for (let i = 0; i < 60; i++) {
    const date = i < fixed.length ? fixed[i] : Math.floor((rnd() * 2 - 1) * 4e12) + 1.6e12;
    const now = rnd() < 0.5 ? date + Math.floor((rnd() * 2 - 1) * 2 * 864e5) : date;
    const mask = pick(masks);
    const utc = rnd() < 0.2;
    NOW = now;
    cases.push({ date, mask, utc, now, out: df(new Date(date), mask, utc) });
  }
  out.push({ zone, cases });
}
process.stdout.write(JSON.stringify(out) + "\n");
