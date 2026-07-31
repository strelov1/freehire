// How much of the design system the app actually reaches for.
//
// A REPO-BOUNDARY check, not a package check: it reads ../../web/src, which the
// package itself knows nothing about. If design-system/ is ever extracted, this
// script stays with the repo.
//
// Phase 7 found eleven primitives built, tested, storybooked, documented and
// unreachable from app code with every CI job green — the design-system job proved
// the package built, the web job proved the app built, and both were true whether
// or not they were connected. `export *` fixed reachability. Reachability is not
// use: a primitive can be reachable from every file and called by none, and until
// this script nothing said so.
//
// The census counts FILES that import a primitive, not occurrences. The question
// is whether the app reaches for the primitive when it needs one, and a file that
// reaches once has answered it. Occurrences would also swing on any refactor that
// splits or merges a component.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { ratchet } from './ratchet.mjs';
import { packageSrc, repoRelative, sourceFiles, stripComments, webSrc } from './source.mjs';

const PACKAGE = 'freehire-design-system';
const DOOR = '$lib/ui';

const indexPath = join(packageSrc, 'index.ts');
const baselinePath = join(fileURLToPath(new URL('.', import.meta.url)), 'adoption-baseline.json');

// Derived from the package's own export surface, so a primitive added to the
// system joins the census by itself. A list of fifteen names kept here would
// drift the way the DSDS props and the Storybook argTypes already have — and a
// census that silently stops counting a primitive is worse than none.
export function primitivesFrom(indexSource) {
  return [...indexSource.matchAll(/export\s*\{\s*default\s+as\s+(\w+)\s*\}/g)].map((m) => m[1]);
}

// `import <clause> from '<specifier>'`. Lazy up to the first `from` so a
// multi-line specifier list is one match. A stylesheet's `@import "..."` has no
// `from` clause and is never seen — which is what keeps app.css's required
// import of the package's theme.css out of the door rule.
const IMPORT = /\bimport\s+([\s\S]*?)\bfrom\s*['"]([^'"]+)['"]/g;

export function readImports(source) {
  const door = [];
  const direct = [];

  for (const [, clause, specifier] of stripComments(source).matchAll(IMPORT)) {
    if (specifier === PACKAGE || specifier.startsWith(`${PACKAGE}/`)) direct.push(specifier);
    if (specifier === DOOR) door.push(...names(clause));
  }

  return { door, direct };
}

// The imported name, not the local alias: renaming a primitive at the call site
// does not change which primitive the file reached for.
function names(clause) {
  const braced = clause.match(/\{([\s\S]*)\}/);
  if (!braced) return [];
  return braced[1]
    .split(',')
    .map((name) => name.trim().replace(/^type\s+/, '').split(/\s+as\s+/)[0].trim())
    .filter(Boolean);
}

function main() {
  const primitives = primitivesFrom(readFileSync(indexPath, 'utf-8'));
  const counts = Object.fromEntries(primitives.map((name) => [name, 0]));
  const trespass = [];

  for (const path of sourceFiles(webSrc)) {
    const { door, direct } = readImports(readFileSync(path, 'utf-8'));
    if (direct.length > 0) trespass.push(`${repoRelative(path)}: imports ${direct.join(', ')}`);
    for (const name of new Set(door)) {
      if (name in counts) counts[name] += 1;
    }
  }

  const unused = primitives.filter((name) => counts[name] === 0);
  const used = primitives.length - unused.length;
  console.log(`adoption: ${used}/${primitives.length} primitives have a consumer in web/src`);
  if (unused.length > 0) {
    // Named every run, passing or failing. This is the finding the script was
    // written for; it should not take a rerun with a flag to see it.
    console.log(`  unused: ${unused.join(', ')}`);
  }

  // Not ratcheted. $lib/ui exists precisely so the app never names the package,
  // there are zero of these, and zero is a number a wall can hold.
  for (const line of trespass) {
    console.error(`✗ ${line} — app code reaches the design system through ${DOOR} only`);
  }

  const update = process.argv.includes('--update');
  const { ok, lines } = ratchet({ counts, baselinePath, direction: 'up', update });
  for (const line of lines) console.error(ok ? `  ${line}` : `✗ adoption: ${line}`);

  // Two different failures, and the closing line has to name the one that
  // happened: a trespass is a wall to move back off, a movement is a number to
  // record, and telling someone to rerun with --update over a stray import
  // sends them the wrong way.
  if (trespass.length > 0) {
    console.error(`\nImport from ${DOOR} instead. It re-exports the package wholesale, so there is\nnothing reachable by naming ${PACKAGE} that is not reachable through the door.`);
  }
  if (!ok) {
    console.error('\nAdoption is held at exactly its baseline. A primitive gaining consumers is\ngood news the baseline has to record: rerun with --update and commit the diff.');
  }
  if (!ok || trespass.length > 0) process.exit(1);
  console.log('✓ adoption matches its baseline.');
}


if (import.meta.url === pathToFileURL(process.argv[1] ?? '').href) main();
