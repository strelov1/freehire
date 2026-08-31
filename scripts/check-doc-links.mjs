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
import { existsSync, readFileSync } from 'node:fs';
import { dirname, join, normalize } from 'node:path';

// Tracked files only, from git. A walk would have to re-derive .gitignore, and would read
// whatever a local build left behind — node_modules alone carries thousands of READMEs.
function trackedMarkdown() {
  const out = execFileSync('git', ['ls-files', '-z', '*.md'], { encoding: 'utf8' });
  return out.split('\0').filter(Boolean);
}

/**
 * Blanks out fenced blocks and inline code, replacing them with spaces so every remaining
 * offset still lines up with the original text.
 *
 * WITHOUT THIS THE CHECK IS MOSTLY FALSE POSITIVES, and they look exactly like real ones. A
 * Go generic signature — `Run[C](ctx context.Context, claimer Claimer[C])` — is a bracket
 * followed by a parenthesis, which is precisely the shape of a Markdown link. Six of the
 * ten findings on the first run were that.
 */
function stripCode(text) {
  const blank = (m) => m.replace(/[^\n]/g, ' ');
  return text
    .replace(/^```[\s\S]*?^```/gm, blank) // fenced
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

for (const file of trackedMarkdown()) {
  const text = stripCode(readFileSync(file, 'utf8'));
  for (const m of text.matchAll(LINK)) {
    const target = m[1].split('#')[0].trim();
    if (!target || isExternal(target)) continue;
    checked += 1;
    if (!existsSync(normalize(join(dirname(file), decodeURIComponent(target))))) {
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
