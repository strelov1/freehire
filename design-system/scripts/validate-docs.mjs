// Validates the DSDS entity JSON in docs/dsds/. Two classes of check: that the
// files are structurally what a docs site would expect, and that what they claim
// about the package is still true. The second is the one that earns its keep — an
// entity is a hand-written copy of a component or a token family, and nothing but
// this script notices when the original moves.
import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { stripComments } from './source.mjs';

const root = join(import.meta.dirname, '..');
const docsDir = join(root, 'docs', 'dsds');
const srcDir = join(root, 'src');

let errors = 0;
function fail(message) {
  console.error(`✗ ${message}`);
  errors++;
}

const CLOSERS = { '{': '}', '[': ']', '(': ')' };
const QUOTES = new Set(['"', "'", '`']);

// Index of the quote closing the one at `open`. -1 if unterminated.
function skipString(source, open) {
  const quote = source[open];
  for (let i = open + 1; i < source.length; i++) {
    if (source[i] === '\\') i++;
    else if (source[i] === quote) return i;
  }
  return -1;
}

// Index of the brace/bracket/paren closing the one at `open`, skipping over string
// and template literals so a delimiter inside a default value cannot unbalance the
// count. -1 if the source never closes it.
function matchClose(source, open) {
  const stack = [CLOSERS[source[open]]];
  for (let i = open + 1; i < source.length; i++) {
    const char = source[i];
    if (char === '\\') i++;
    else if (QUOTES.has(char)) {
      i = skipString(source, i);
      if (i < 0) return -1;
    } else if (CLOSERS[char]) stack.push(CLOSERS[char]);
    else if (char === stack.at(-1)) {
      stack.pop();
      if (stack.length === 0) return i;
    }
  }
  return -1;
}

// Index of the last character of the string or balanced group opening at `i`, or
// `i` itself where neither opens. Every scan below advances through source this
// way, so that a delimiter nested inside one is never read as structure.
function skipGroup(source, i) {
  if (QUOTES.has(source[i])) return skipString(source, i);
  if (CLOSERS[source[i]]) return matchClose(source, i);
  return i;
}

function skipSpace(source, from) {
  let i = from;
  while (/\s/.test(source[i] ?? '')) i++;
  return i;
}

// Whether the destructure ending at `from` is the one $props() fills.
function isPropsDestructure(source, from) {
  let i = skipSpace(source, from);
  // What may sit between the two is a type annotation, itself often a whole brace
  // block. Skip balanced groups rather than scanning for the next '=', which an
  // arrow type (`() => void`) would hand us far too early.
  if (source[i] === ':') {
    for (i++; i < source.length; i++) {
      if (source[i] === '=' && source[i + 1] !== '>' && source[i + 1] !== '=') break;
      i = skipGroup(source, i);
      if (i < 0) return false;
    }
  }
  if (source[i] !== '=') return false;
  return source.startsWith('$props(', skipSpace(source, i + 1));
}

// The members of a destructure, split on the commas that separate them rather than
// on every comma — a default value is free to contain its own.
function splitMembers(body) {
  const members = [];
  let start = 0;
  for (let i = 0; i < body.length; i++) {
    if (body[i] === ',') {
      members.push(body.slice(start, i));
      start = i + 1;
      continue;
    }
    i = skipGroup(body, i);
    if (i < 0) break;
  }
  members.push(body.slice(start));
  return members;
}

// The prop names a component accepts, spelled the way a call site writes them:
// `class: className` is passed as class, and the rest spread keeps its `...` so the
// docs can name the passthrough. Exported for the test, and because a docs site
// would want the same list.
export function propsFrom(source) {
  const src = stripComments(source);
  for (const match of src.matchAll(/\blet\s*\{/g)) {
    const open = match.index + match[0].length - 1;
    const close = matchClose(src, open);
    if (close < 0 || !isPropsDestructure(src, close + 1)) continue;
    return splitMembers(src.slice(open + 1, close))
      .map((member) => member.trim().match(/^(?:\.\.\.)?[A-Za-z_$][\w$]*/)?.[0])
      .filter(Boolean);
  }
  return [];
}

// A hand-written list against the real one, in both directions. Omitting half the
// original is as wrong as inventing an entry, and only the first is the kind of
// drift a reader would never notice — so no check here reports one without the other.
function drift(shipped, documented) {
  return {
    undocumented: shipped.filter((name) => !documented.includes(name)),
    gone: documented.filter((name) => !shipped.includes(name)),
  };
}

// Every story file some entity claims, so the sweep at the end can name the ones
// nobody claims.
const claimedStories = new Set();

function checkEntity(file, path, entity) {
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

  const target = entity.source?.file && resolve(dirname(path), entity.source.file);
  if (!target || !existsSync(target)) return;

  // A token list is a copy of the token file's keys. Drift either way means the
  // docs describe a palette the package no longer ships.
  if (entity.tokens) {
    const shipped = Object.keys(JSON.parse(readFileSync(target, 'utf-8'))).filter(
      (name) => !name.startsWith('$'),
    );
    const { undocumented, gone } = drift(shipped, entity.tokens);
    if (undocumented.length) {
      fail(`${file}: entity "${entity.id}" does not document ${undocumented.join(', ')}`);
    }
    if (gone.length) {
      fail(`${file}: entity "${entity.id}" documents tokens that no longer exist — ${gone.join(', ')}`);
    }
  }

  // Same rule one level down, for the props of a component. The prop list is the
  // half of an entity an agent reads to decide what it may pass, so a prop missing
  // from it is a capability nothing can discover — Button grew target/rel and the
  // entity did not, taking with it the reverse-tabnabbing default the component
  // sets so no call site has to.
  if (entity.props && target.endsWith('.svelte')) {
    const shipped = propsFrom(readFileSync(target, 'utf-8'));
    const { undocumented, gone } = drift(
      shipped,
      entity.props.map((prop) => prop.name),
    );
    if (undocumented.length) {
      fail(`${file}: entity "${entity.id}" does not document prop(s) ${undocumented.join(', ')}`);
    }
    if (gone.length) {
      fail(`${file}: entity "${entity.id}" documents prop(s) the component no longer takes — ${gone.join(', ')}`);
    }
  }
}

function main() {
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

    for (const entity of data.entities) checkEntity(file, path, entity);

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
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? '').href) main();
