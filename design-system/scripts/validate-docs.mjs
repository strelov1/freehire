// Validates the DSDS entity JSON in docs/dsds/. Two classes of check: that the
// files are structurally what a docs site would expect, and that what they claim
// about the package is still true. The second is the one that earns its keep — an
// entity is a hand-written copy of a component or a token family, and nothing but
// this script notices when the original moves.
import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';

const root = join(import.meta.dirname, '..');
const docsDir = join(root, 'docs', 'dsds');
const srcDir = join(root, 'src');

let errors = 0;
function fail(message) {
  console.error(`✗ ${message}`);
  errors++;
}

// Every story file some entity claims, so the sweep at the end can name the ones
// nobody claims.
const claimedStories = new Set();

const files = readdirSync(docsDir).filter((file) => file.endsWith('.json'));
if (files.length === 0) fail('docs/dsds/: no JSON files — the docs are gone, not valid');

for (const file of files) {
  const path = join(docsDir, file);
  let data;
  try {
    data = JSON.parse(readFileSync(path, 'utf-8'));
  } catch (e) {
    fail(`${file}: invalid JSON — ${e.message}`);
    continue;
  }
  if (!Array.isArray(data.entities)) {
    fail(`${file}: missing "entities" array`);
    continue;
  }

  for (const entity of data.entities) {
    if (!entity.id) fail(`${file}: entity missing "id"`);
    if (!entity.type) fail(`${file}: entity "${entity.id}" missing "type"`);
    if (!entity.name) fail(`${file}: entity "${entity.id}" missing "name"`);

    // File references are relative to the JSON that holds them.
    const stories = entity.stories ?? [];
    for (const ref of [entity.source, ...stories]) {
      if (!ref?.file) continue;
      const target = resolve(dirname(path), ref.file);
      if (existsSync(target)) continue;
      fail(`${file}: entity "${entity.id}" points at a missing file — ${ref.file}`);
    }
    for (const story of stories) {
      if (story.file) claimedStories.add(resolve(dirname(path), story.file));
    }

    // A token list is a copy of the token file's keys. Drift either way means the
    // docs describe a palette the package no longer ships.
    if (entity.tokens && entity.source?.file) {
      const target = resolve(dirname(path), entity.source.file);
      if (existsSync(target)) {
        const authored = Object.keys(JSON.parse(readFileSync(target, 'utf-8')));
        const shipped = authored.filter((name) => !name.startsWith('$'));
        const documented = new Set(entity.tokens);
        const undocumented = shipped.filter((name) => !documented.has(name));
        const gone = entity.tokens.filter((name) => !shipped.includes(name));
        if (undocumented.length) {
          fail(`${file}: entity "${entity.id}" does not document ${undocumented.join(', ')}`);
        }
        if (gone.length) {
          fail(`${file}: entity "${entity.id}" documents tokens that no longer exist — ${gone.join(', ')}`);
        }
      }
    }
  }

  console.log(`✓ ${file}: ${data.entities.length} entities`);
}

// The other direction: a story file added to the package and never documented.
const unclaimed = readdirSync(srcDir)
  .filter((file) => file.endsWith('.stories.ts'))
  .filter((file) => !claimedStories.has(join(srcDir, file)));
if (unclaimed.length) fail(`src/: story files no entity claims — ${unclaimed.join(', ')}`);

if (errors > 0) {
  console.error(`\n${errors} error(s) found.`);
  process.exit(1);
} else {
  console.log('\nAll DSDS docs valid.');
}
