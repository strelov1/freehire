// Asserts that things style themselves from the token scale rather than from
// values nothing owns — a hex, a Tailwind arbitrary value, or a hue off
// Tailwind's built-in palette. All three are the same failure: a value that
// exists in one file and nowhere in tokens/*.tokens.json, so the theme cannot
// move it and the dark selector cannot override it. Nothing else notices,
// because a hex is valid CSS and the other two are valid Tailwind, and the
// build stays green either way.
//
// TWO RADII, and deliberately not the same rule.
//
//   design-system/src — fifteen files, currently clean, so a violation is always
//                       a mistake. Hard fail, subject only to ALLOWED.
//   ../../web/src     — 216 files carrying hundreds of violations that no single
//                       change can remove. Held at exactly their current count
//                       per file, because a rule nobody can satisfy is a rule
//                       that gets switched off rather than obeyed.
//
// The web half is a REPO-BOUNDARY check: it reads a directory the package itself
// knows nothing about. If design-system/ is ever extracted, it stays behind.
//
// One definition per detector, shared by both radii. A second copy is how the
// DSDS props and the Storybook argTypes drifted from the components they
// describe — and a forked detector drifts silently, one radius quietly ceasing
// to catch what the other still does.
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { ratchet } from './ratchet.mjs';
import { packageSrc, repoRelative, sourceFiles, stripComments, webSrc } from './source.mjs';

// A colour written out rather than referenced. `color()` and `lab()`/`lch()` are
// in here for completeness — the palette is oklch, so they would be an import
// from somewhere else entirely.
const COLOUR = /#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?|oklch|oklab|lab|lch|color)\(/g;

// A Tailwind arbitrary *value* — `p-[7px]`, `bg-[#fff]`, `w-[calc(100%-2rem)]`.
// The negative lookahead is what keeps arbitrary *variants* out: `[&_tr]:border-b`
// is a selector, not a value, and no token could replace it. The leading hyphen
// keeps TypeScript indexing (`sizes[size]`, `HTMLButtonElement[]`) out.
const ARBITRARY = /-\[[^\]]+\](?!:)/g;

// A utility built on Tailwind's own colour scale. Neither a literal nor an
// arbitrary value, so neither detector above can see it — and in web/src it is
// the majority of what there is. It is off the token scale all the same:
// `text-amber-600` is a fixed hue the theme cannot move and `.dark` gets no say
// in. Restricted to the colour-bearing prefixes on purpose; `p-4`, `z-50` and
// `grid-cols-3` are scales the design system does not own.
const PALETTE =
  /\b(?:bg|text|border|ring|fill|stroke|from|via|to|decoration|outline|shadow|accent|caret|divide|placeholder)-(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-(?:50|950|[1-9]00)\b/g;

// Exported as functions, not as the regexes themselves: a /g regex carries
// lastIndex between calls, so a shared one handed to `.test()` alternates true
// and false. `String.match` resets it; nothing outside should have to know that.
export const DETECTORS = {
  'colour literal': (line) => line.match(COLOUR) ?? [],
  'Tailwind arbitrary value': (line) => line.match(ARBITRARY) ?? [],
  'raw palette utility': (line) => line.match(PALETTE) ?? [],
};

export const RADII = {
  package: ['colour literal', 'Tailwind arbitrary value'],
  web: ['colour literal', 'Tailwind arbitrary value', 'raw palette utility'],
};

// Deliberate exceptions, package radius only. Each must still match something —
// a stale entry is an allowance nobody is using, and it would quietly cover the
// next violation that lands in the same file.
const ALLOWED = [
  {
    file: 'avatar.svelte',
    kind: 'colour literal',
    reason:
      'one hue per name at two fixed lightnesses; a token per hue makes no sense, ' +
      'and the pair carries its own contrast so it needs no theme override',
  },
  {
    file: 'avatar.svelte',
    kind: 'Tailwind arbitrary value',
    reason:
      "the xs tile (size-5, promoted from CompanyLogo's smallest call sites) is too small " +
      "for any stock text size — 9px is a deliberate one-off, not a value worth a token",
  },
  {
    file: 'provider-icon.svelte',
    kind: 'colour literal',
    reason:
      "Google's brand mark is specified in these four exact colours; a token would " +
      'misrepresent the mark, and no other provider path uses a literal (they follow ' +
      'currentColor instead)',
  },
  {
    file: 'section-label.svelte',
    kind: 'Tailwind arbitrary value',
    reason:
      '0.2em tracking for one mono heading style; Tailwind\'s own scale tops out at ' +
      'tracking-widest (0.1em) and no other primitive needs this value',
  },
];

const baselinePath = join(fileURLToPath(new URL('.', import.meta.url)), 'web-token-baseline.json');

function hitsIn(source, kinds) {
  const hits = [];
  for (const [index, line] of stripComments(source).split('\n').entries()) {
    for (const kind of kinds) {
      for (const text of DETECTORS[kind](line)) hits.push({ kind, line: index + 1, text });
    }
  }
  return hits;
}

let errors = 0;
function fail(message) {
  console.error(`✗ ${message}`);
  errors++;
}

// The package radius: zero, and it stays zero.
function checkPackage() {
  const used = new Set();
  const files = readdirSync(packageSrc).filter((f) => f.endsWith('.svelte')).sort();

  for (const file of files) {
    for (const hit of hitsIn(readFileSync(join(packageSrc, file), 'utf-8'), RADII.package)) {
      const exception = ALLOWED.find((a) => a.file === file && a.kind === hit.kind);
      if (exception) {
        used.add(exception);
        continue;
      }
      fail(`${file}:${hit.line}: ${hit.kind} — ${hit.text}`);
    }
  }

  for (const exception of ALLOWED) {
    if (used.has(exception)) continue;
    fail(`allowlist: ${exception.file} no longer needs its exception — drop it from this script`);
  }

  return files.length;
}

// The web radius: held per file, so a regression names the file that caused it
// rather than moving a global total by one, and the baseline doubles as the
// ranked list of what is left to fix.
function checkWeb(update) {
  const counts = {};
  const hits = new Map();

  for (const path of sourceFiles(webSrc)) {
    const found = hitsIn(readFileSync(path, 'utf-8'), RADII.web);
    if (found.length === 0) continue;
    const name = repoRelative(path);
    counts[name] = found.length;
    hits.set(name, found);
  }

  const { ok, lines, moved } = ratchet({ counts, baselinePath, direction: 'down', update });
  if (ok) {
    for (const line of lines) console.log(`  ${line}`);
  } else {
    for (const line of lines) fail(`web: ${line}`);
    // Naming the file is not enough to act on: a count went 3 → 4 and the
    // question is which line is the fourth. A file's hits are few; print them.
    for (const { key } of moved) {
      for (const hit of hits.get(key) ?? []) {
        console.error(`    ${key}:${hit.line}: ${hit.kind} — ${hit.text}`);
      }
    }
  }

  return Object.values(counts).reduce((sum, n) => sum + n, 0);
}

function main() {
  const update = process.argv.includes('--update');
  const components = checkPackage();
  const total = checkWeb(update);

  if (errors > 0) {
    console.error(`\n${errors} problem(s).
In design-system/src: use a token from tokens/*.tokens.json, or add a deliberate
exception to ALLOWED in this script with the reason it is one.
In web/src: the count is held per file. Fewer is good news the baseline has to
record — rerun with --update and commit the diff.`);
    process.exit(1);
  }

  console.log(`✓ token coverage: ${components} primitives clean, web/src held at ${total} violations.`);
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? '').href) main();
