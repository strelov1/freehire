#!/usr/bin/env node
// Checks that every relative Markdown link in the repository points at a file that exists.
//
// This repository navigates by document. AGENTS.md is a table of links into the blocks,
// each block's AGENTS.md links into its packages, and the architecture doc links across all
// of it — a reader, human or otherwise, is expected to follow those rather than to search.
// A link that no longer resolves does not merely fail: it sends the reader somewhere
// confidently, which is worse than saying nothing. Nothing else in the repository reads
// them, and a rename is silent.
//
// Usage: node scripts/check-doc-links.mjs

import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { dirname, join, normalize } from 'node:path';

// Tracked files only, from git. A walk would have to re-derive .gitignore, and would read
// whatever a local build left behind — node_modules alone carries thousands of READMEs.
function tracked() {
  const out = execFileSync('git', ['ls-files', '-z'], { encoding: 'utf8' });
  return out.split('\0').filter(Boolean);
}

const TRACKED = new Set(tracked());
// Directory prefixes, so a link to a folder resolves. git tracks files, not directories.
const TRACKED_DIRS = new Set();
for (const f of TRACKED) {
  let d = dirname(f);
  while (d && d !== '.' && !TRACKED_DIRS.has(d)) {
    TRACKED_DIRS.add(d);
    d = dirname(d);
  }
}

/**
 * A target counts only if git knows it.
 *
 * `existsSync` alone answers a different question: it says yes to a build artifact, to an
 * ignored file and to anything a local run left behind — so a link would pass on the
 * machine that wrote it and fail in CI, which is the one place the check is not watched by
 * the person who can fix it. It also says yes to a path that walks out of the repository
 * entirely; `../../../../etc/hosts` is not a broken link by that measure, and is not a link
 * anyone can follow either.
 */
function resolvesInRepo(fromFile, target) {
  const p = normalize(join(dirname(fromFile), target));
  if (p.startsWith('..')) return false; // escaped the repository root
  return TRACKED.has(p) || TRACKED_DIRS.has(p);
}

/**
 * Blanks out fenced blocks and inline code, replacing them with spaces so every remaining
 * offset still lines up with the original text.
 *
 * WITHOUT THIS THE CHECK IS MOSTLY FALSE POSITIVES, and they look exactly like real ones. A
 * Go generic signature — `Run[C](ctx context.Context, claimer Claimer[C])` — is a bracket
 * followed by a parenthesis, which is precisely the shape of a Markdown link. Six of the
 * ten findings on the first run were that.
 *
 * A fence may be indented up to three spaces and may use tildes, and BOTH FORMS OCCUR HERE:
 * a code block nested in a numbered list is indented, and this repository has several. A
 * pattern anchored at column zero reads their contents as prose, so the trap is one Go
 * signature inside a list item away.
 */
function stripCode(text) {
  const blank = (m) => m.replace(/[^\n]/g, ' ');
  return text
    .replace(/^ {0,3}```[\s\S]*?^ {0,3}```/gm, blank) // fenced, backticks
    .replace(/^ {0,3}~~~[\s\S]*?^ {0,3}~~~/gm, blank) // fenced, tildes
    .replace(/`[^`\n]*`/g, blank); // inline
}

const LINK = /\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)/g;

function isExternal(target) {
  return (
    target.startsWith('http://') ||
    target.startsWith('https://') ||
    target.startsWith('mailto:') ||
    target.startsWith('#') ||
    // Root-relative: a site route served by the SPA, not a path on disk.
    target.startsWith('/')
  );
}

const broken = [];
let checked = 0;

for (const file of TRACKED) {
  if (!file.endsWith('.md')) continue;
  const text = stripCode(readFileSync(file, 'utf8'));
  for (const m of text.matchAll(LINK)) {
    const target = m[1].split('#')[0].trim();
    if (!target || isExternal(target)) continue;
    checked += 1;
    if (!resolvesInRepo(file, decodeURIComponent(target))) {
      broken.push({ file, target, line: text.slice(0, m.index).split('\n').length });
    }
  }
}

if (broken.length > 0) {
  for (const b of broken) console.error(`${b.file}:${b.line}: broken link -> ${b.target}`);
  console.error(`\n${broken.length} broken link(s) out of ${checked} relative links checked.`);
  process.exit(1);
}

console.log(`check-doc-links: ${checked} relative links, all resolve.`);
