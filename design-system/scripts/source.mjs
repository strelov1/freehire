// Reading the repo, shared by the verification scripts that cross the package
// boundary. Both walk web/src, both name files the same way, and both have to
// read source with its comments gone.
import { readdirSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptsDir = fileURLToPath(new URL('.', import.meta.url));

export const webSrc = join(scriptsDir, '..', '..', 'web', 'src');
export const packageSrc = join(scriptsDir, '..', 'src');

// Comments describe violations as often as they commit them: check-token-coverage's
// own header names a hex and an arbitrary value, and a file that stopped using a
// primitive often keeps the import in a comment. Both checks read source looking
// for exactly the thing a comment is most likely to be talking about.
export function stripComments(source) {
  return source
    .replaceAll(/<!--[\s\S]*?-->/g, '')
    .replaceAll(/\/\*[\s\S]*?\*\//g, '')
    .replaceAll(/(^|[\s;{(])\/\/[^\n]*/g, '$1');
}

// Sorted, so a report and a baseline read in the same order every run.
export function* sourceFiles(dir) {
  const entries = readdirSync(dir, { withFileTypes: true });
  for (const entry of entries.sort((a, b) => a.name.localeCompare(b.name))) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) yield* sourceFiles(path);
    else if (/\.(svelte|ts)$/.test(entry.name)) yield path;
  }
}

// Repo-relative, so a baseline committed here does not carry whoever's checkout
// wrote it — including a worktree under .claude/.
export function repoRelative(path) {
  return path.slice(path.indexOf('/web/src/') + 1);
}
