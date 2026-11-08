// Builds the logo and flag stacks the /collections hub draws beside each collection,
// and writes them to the snapshot the hub reads. Run by hand, from web/:
//
//   pnpm gen:collection-stacks                              # against https://freehire.me
//   API_BASE=http://localhost:8080 pnpm gen:collection-stacks
//
// A snapshot rather than a request-time lookup: who leads a collection moves over
// weeks, not minutes, while working it out costs a 24-posting search per row — some
// eighty of them — that every render would otherwise repeat. Calls go one at a time
// and wait out a 429, so the run is slow and complete rather than fast and full of
// holes. A row the API answers nothing for is simply left out; the hub falls back to
// the row's own mark.
//
// The registry is TypeScript, so it is loaded through Vite's SSR module loader — the
// same resolver the app builds with — rather than through a second copy of it here.

import { writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const API_BASE = (process.env.API_BASE ?? 'https://freehire.me').replace(/\/$/, '');
const SNAPSHOT_PATH = fileURLToPath(new URL('../src/lib/data/collectionStacks.json', import.meta.url));

// How many logos or flags a stack shows.
const STACK = 4;

// Sections whose filter collections carry a company stack. The rest keep their mark
// on purpose: a language, a framework or a city already has a logo or a flag of its
// own, and a remote row gets its region's flags instead.
const COMPANY_STACK_GROUPS = new Set(['field', 'ai', 'seniority', 'role', 'industry', 'job-type', 'company-type']);

// The company collections whose members stand for "who hires here" on a filter
// collection's row. The head of an unrestricted feed is mostly staffing agencies and
// job boards (DevOps: Manpower, Michael Page, Hunters Group); within these lists it
// is employers a visitor recognises. Every one is an existing `collections` value.
const NOTABLE_COLLECTIONS = [
  'mag7',
  'bigtech',
  'unicorn',
  'ai',
  'ai-native',
  'fortune500',
  'yc',
  'a16z-portfolio',
  'european',
];

// How many postings a company stack is counted over.
const SAMPLE = 24;

async function get(path, attempt = 0) {
  const res = await fetch(`${API_BASE}${path}`, { headers: { accept: 'application/json' } });
  if (res.status === 429 && attempt < 8) {
    const wait = Number(res.headers.get('retry-after')) || 15;
    await new Promise((resolve) => setTimeout(resolve, wait * 1000));
    return get(path, attempt + 1);
  }
  if (!res.ok) throw new Error(`${res.status} ${path}`);
  return res.json();
}

// The most frequent employers among a filter collection's postings at notable companies.
async function leadingCompanies(query) {
  const q = new URLSearchParams(query);
  for (const c of NOTABLE_COLLECTIONS) q.append('collections', c);
  q.set('limit', String(SAMPLE));
  const { data } = await get(`/api/v1/jobs/search?${q}`);
  const tally = new Map();
  for (const job of data) {
    if (!job.company) continue;
    const key = job.company_slug || job.company;
    const entry = tally.get(key) ?? { name: job.company, n: 0 };
    entry.n++;
    tally.set(key, entry);
  }
  // Stable sort: equal counts keep feed order.
  return [...tally.values()]
    .sort((a, b) => b.n - a.n)
    .slice(0, STACK)
    .map((e) => e.name);
}

// The countries holding the most of a regional collection's postings.
async function leadingCountries(query) {
  const q = new URLSearchParams(query);
  q.set('facets', 'countries');
  const { data } = await get(`/api/v1/jobs/facets?${q}`);
  return Object.entries(data.facets.countries ?? {})
    .sort((a, b) => b[1] - a[1])
    .slice(0, STACK)
    .map(([code]) => code);
}

// A company collection's members with the most open jobs.
async function collectionLeaders(slug) {
  const q = new URLSearchParams({ collections: slug, limit: String(STACK) });
  const { data } = await get(`/api/v1/companies?${q}`);
  return data.map((company) => company.name);
}

const vite = await createServer({
  configFile: false,
  root: fileURLToPath(new URL('..', import.meta.url)),
  logLevel: 'error',
  appType: 'custom',
  server: { middlewareMode: true },
  optimizeDeps: { noDiscovery: true, include: [] },
});
const { FILTER_COLLECTIONS, COLLECTIONS, toQuery } = await vite.ssrLoadModule('/src/lib/collections.ts');
await vite.close();

const stacks = {};
const failed = [];
async function record(slug, kind, load) {
  try {
    const value = await load();
    if (value.length > (kind === 'flags' ? 1 : 0)) stacks[slug] = { [kind]: value };
  } catch (err) {
    failed.push(`${slug}: ${err.message}`);
  }
}

for (const fc of FILTER_COLLECTIONS) {
  if (COMPANY_STACK_GROUPS.has(fc.group)) {
    await record(fc.slug, 'companies', () => leadingCompanies(toQuery(fc.params)));
  } else if (fc.group === 'remote' && !('countries' in fc.params)) {
    // A regional remote row has no single country to fly; one naming a country keeps its own flag.
    await record(fc.slug, 'flags', () => leadingCountries(toQuery(fc.params)));
  }
}
for (const c of COLLECTIONS) {
  await record(c.slug, 'companies', () => collectionLeaders(c.slug));
}

// Sorted keys, so a re-run that finds the same leaders produces the same file.
const sorted = Object.fromEntries(
  Object.keys(stacks)
    .sort()
    .map((k) => [k, stacks[k]]),
);
await writeFile(SNAPSHOT_PATH, `${JSON.stringify(sorted, null, 2)}\n`);
console.log(`wrote ${Object.keys(sorted).length} stacks to ${SNAPSHOT_PATH}`);
if (failed.length > 0) {
  console.error(`failed ${failed.length}:\n  ${failed.join('\n  ')}`);
  process.exitCode = 1;
}
