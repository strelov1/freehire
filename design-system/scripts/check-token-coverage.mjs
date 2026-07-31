// Asserts that the primitives style themselves from tokens: no colour literal and
// no Tailwind arbitrary value in src/*.svelte. Both are the same failure — a value
// that exists in one component and nowhere in tokens/*.tokens.json, so the theme
// cannot move it and the dark selector cannot override it. Nothing else notices:
// a hex is valid CSS, an arbitrary value is valid Tailwind, and the build stays
// green either way.
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';

const srcDir = join(import.meta.dirname, '..', 'src');

// A colour written out rather than referenced. `color()` and `lab()`/`lch()` are
// in here for completeness — the palette is oklch, so they would be an import
// from somewhere else entirely.
const COLOUR = /#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?|oklch|oklab|lab|lch|color)\(/g;

// A Tailwind arbitrary *value* — `p-[7px]`, `bg-[#fff]`, `w-[calc(100%-2rem)]`.
// The negative lookahead is what keeps arbitrary *variants* out: `[&_tr]:border-b`
// is a selector, not a value, and no token could replace it. The leading hyphen
// keeps TypeScript indexing (`sizes[size]`, `HTMLButtonElement[]`) out.
const ARBITRARY = /-\[[^\]]+\](?!:)/g;

// Deliberate exceptions. Each must still match something — a stale entry is an
// allowance nobody is using, and it would quietly cover the next violation that
// lands in the same file.
const ALLOWED = [
  {
    file: 'avatar.svelte',
    pattern: COLOUR,
    reason:
      'one hue per name at two fixed lightnesses; a token per hue makes no sense, ' +
      'and the pair carries its own contrast so it needs no theme override',
  },
];

// Comments describe violations as often as they commit them — this file's own
// header would trip both patterns.
function stripComments(source) {
  return source
    .replaceAll(/<!--[\s\S]*?-->/g, '')
    .replaceAll(/\/\*[\s\S]*?\*\//g, '')
    .replaceAll(/(^|[\s;{(])\/\/[^\n]*/g, '$1');
}

let errors = 0;
function fail(message) {
  console.error(`✗ ${message}`);
  errors++;
}

const used = new Set();

for (const file of readdirSync(srcDir).filter((f) => f.endsWith('.svelte')).sort()) {
  const lines = stripComments(readFileSync(join(srcDir, file), 'utf-8')).split('\n');

  for (const [kind, pattern] of [
    ['colour literal', COLOUR],
    ['Tailwind arbitrary value', ARBITRARY],
  ]) {
    const exception = ALLOWED.find((a) => a.file === file && a.pattern === pattern);
    for (const [i, line] of lines.entries()) {
      const hits = line.match(pattern);
      if (!hits) continue;
      if (exception) {
        used.add(exception);
        continue;
      }
      fail(`${file}:${i + 1}: ${kind} — ${hits.join(', ')}`);
    }
  }
}

for (const exception of ALLOWED) {
  if (used.has(exception)) continue;
  fail(`allowlist: ${exception.file} no longer needs its exception — drop it from this script`);
}

if (errors > 0) {
  console.error(`\n${errors} violation(s). Use a token from tokens/*.tokens.json, or add a
deliberate exception to the ALLOWED list in this script with the reason it is one.`);
  process.exit(1);
}

console.log(`✓ token coverage: ${readdirSync(srcDir).filter((f) => f.endsWith('.svelte')).length} components, no unowned colour or arbitrary value.`);
