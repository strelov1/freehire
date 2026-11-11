// Gives every date-time in the compiled contract ogen's full-precision layout.
//
// ogen writes a date-time as RFC3339 to the whole second unless the schema names a layout
// (x-ogen-time-format). Go's encoding/json, which every route answered with before the
// contract, writes RFC3339Nano, and a client that hands a timestamp back as a cursor or a
// comparison token needs the fraction to land on the same row. Parsing with this layout
// still accepts a time without a fraction. Runs after `tsp compile`, before ogen.
import { readFileSync, writeFileSync } from 'node:fs';

const RFC3339_NANO = '2006-01-02T15:04:05.999999999Z07:00';
const path = process.argv[2] ?? 'openapi/api.openapi.json';
const spec = JSON.parse(readFileSync(path, 'utf8'));

let marked = 0;
(function walk(node) {
  if (Array.isArray(node)) return node.forEach(walk);
  if (node === null || typeof node !== 'object') return;
  if (node.format === 'date-time') {
    node['x-ogen-time-format'] = RFC3339_NANO;
    marked++;
  }
  Object.values(node).forEach(walk);
})(spec);

writeFileSync(path, JSON.stringify(spec, null, 2) + '\n');
console.log(`openapi-time-format: ${marked} date-time schemas`);
