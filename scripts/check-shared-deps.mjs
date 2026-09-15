#!/usr/bin/env node
// Checks that `web/`, `design-system/` and `extension/` agree on the version of every
// package whose TYPES cross the boundary between them.
//
// `design-system/` is consumed as a symlink, not a copy — pnpm's `link:../design-system`
// from `web/`, npm's `file:../design-system` from `extension/`. Each package therefore
// installs its own `node_modules`, and a package depended on by both ends up on disk twice.
// For a value that costs a few kilobytes. For a TYPE it costs correctness: TypeScript
// identifies a type by where it was declared, so `LucideIcon` from the design system's copy
// and `LucideIcon` from the consumer's copy are two unrelated types that happen to share a
// name, and a component prop typed with one rejects a value typed with the other.
//
// Found the hard way (freehire#2835, #2836): Dependabot raised `@lucide/svelte` in
// `design-system/` alone, 1.45 widened the `name` prop to admit `null`, and `svelte-check`
// reported eight errors in `web/` about a type being unassignable to itself. Dependabot
// cannot prevent this — it reads three package.json files as three unrelated projects and
// has no way to know one is symlinked into the others.
//
// The scope is DERIVED, not listed: whatever the design system imports as a type today is
// what must agree today. A hand-written list would answer for the packages someone
// remembered, and a new type import would join the public API silently.
//
// Usage: node scripts/check-shared-deps.mjs

import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';

const CONSUMERS = ['web', 'extension'];
const LIBRARY = 'design-system';

/**
 * The bare package names the design system imports as types, read off its own source.
 *
 * Stories are excluded: `@storybook/svelte` types appear in `*.stories.ts` only, and a
 * consumer never imports a story — that type never crosses the boundary, so requiring the
 * consumers to pin Storybook would be a false alarm.
 *
 * A subpath import (`svelte/elements`) is folded back to the package that ships it, since
 * that is what a package.json can pin.
 */
function typeSurface() {
  const dir = join(LIBRARY, 'src');
  const names = new Set();
  for (const file of readdirSync(dir)) {
    if (file.includes('.stories.')) continue;
    if (!file.endsWith('.svelte') && !file.endsWith('.ts')) continue;
    const src = readFileSync(join(dir, file), 'utf8');
    for (const m of src.matchAll(/import\s+type\s[^;]*?from\s+'([^']+)'/g)) {
      const spec = m[1];
      if (spec.startsWith('.')) continue; // relative — inside the library, never duplicated
      const parts = spec.split('/');
      names.add(spec.startsWith('@') ? parts.slice(0, 2).join('/') : parts[0]);
    }
  }
  return names;
}

/**
 * What a package.json DECLARES for `name`, or undefined.
 *
 * `peerDependencies` is deliberately not read. A library states its peer range as wide as it
 * can honestly support (`"svelte": "^5"`) precisely so a consumer may choose within it —
 * comparing that to a consumer's `^5.57.0` would report every correctly-written library as
 * broken.
 */
function declared(pkg, name) {
  const json = JSON.parse(readFileSync(join(pkg, 'package.json'), 'utf8'));
  return json.dependencies?.[name] ?? json.devDependencies?.[name];
}

/**
 * Whether two declared ranges are close enough to put one copy of the type on disk.
 *
 * The comparison is the literal range, not a semver one. Asking whether the ranges OVERLAP
 * is the intuitive rule and it is the wrong one: `^1.25.0` and `^1.45.0` overlap, and those
 * are the exact two ranges that put @lucide/svelte on disk twice and produced the eight
 * errors this check exists to prevent. A range says what is PERMITTED; two permissive ranges
 * resolve to whatever each lockfile happened to pin, which is how they drifted apart in the
 * first place.
 *
 * Reading the lockfiles instead would measure the installed versions — the thing that
 * actually breaks — but it means parsing two formats and it answers nothing on a fresh clone
 * that has not installed yet. This check is meant to catch the CAUSE, at the moment the
 * ranges diverge; `svelte-check` in CI already catches the consequence.
 *
 * So equality is deliberately strict, and will object to `^5.57.0` against `^5.57.1` even
 * though both install the same svelte. That false alarm is cheap: its fix is the same single
 * action as a real one — raise the lagging package.json — and a check that nags is recoverable
 * in a way that a check that stayed quiet through #2835 is not.
 *
 * @param {string} a range declared by the design system
 * @param {string} b range declared by the consumer
 * @returns {boolean} true when they agree, false when the mismatch should fail the check
 */
function agree(a, b) {
  return a === b;
}

let failed = false;
for (const name of [...typeSurface()].sort()) {
  const lib = declared(LIBRARY, name);
  if (!lib) continue; // the library gets it transitively; nothing to hold the consumers to
  for (const consumer of CONSUMERS) {
    const own = declared(consumer, name);
    if (!own) continue; // this consumer never names it directly — no second copy to skew
    if (agree(lib, own)) continue;
    console.error(
      `${name}: ${LIBRARY} declares ${lib}, ${consumer} declares ${own} — ` +
        `a type from this package crosses the link: boundary, so the two copies must match.`,
    );
    failed = true;
  }
}

if (failed) {
  console.error('\nRaise the lagging package.json to match, then reinstall so the lockfile follows.');
  process.exit(1);
}
console.log(`check-shared-deps: ${[...typeSurface()].sort().join(', ')} agree across ${[LIBRARY, ...CONSUMERS].join(', ')}.`);
