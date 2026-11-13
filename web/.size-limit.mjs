// Brotli budgets for the client JS that `pnpm run build` ships (`pnpm run size`).
//
// File names are content-hashed and route nodes are renumbered whenever a route is added, so
// neither a glob nor a fixed list can name them. The files are resolved at run time from the
// build's own manifests instead:
//   - the entry (start + app and everything they import) from `client.imports` in the server
//     manifest;
//   - a route's files from the server manifest's `routes` (its layout nodes and leaf), then
//     each node's static `imports` closure in the Vite client manifest.
// A route's budget covers what the browser fetches ON TOP of the entry, so a chunk moving
// between the two never counts twice. Dynamic imports are not followed: they load on demand.
//
// Anything that cannot be resolved throws. A budget that silently measures nothing passes
// forever, which is worse than having none.
//
// To raise a budget on purpose, edit its limit below in the same PR and say why in the PR body.
import { existsSync, readFileSync } from 'node:fs';

const webDir = `${import.meta.dirname}/`;
const clientDir = 'build/client';
const serverManifestPath = '.svelte-kit/output/server/manifest.js';
const clientManifestPath = '.svelte-kit/output/client/.vite/manifest.json';
const nodeKey = (n) => `.svelte-kit/generated/build/client-optimized/nodes/${n}.js`;

// Each limit is the size measured on 2026-11-07 plus about 10%, in bytes.
const ENTRY_LIMIT = 73_200;
const ROUTE_LIMITS = {
  '/': 366_500,
  '/jobs': 258_700,
  '/jobs/[slug]': 354_100,
  '/companies/[slug]': 275_900,
  // The CV-first flow replaced the seven-step tour on 2026-11-12: 180.9 kB measured, plus ~10%.
  '/onboarding': 199_000,
  '/my/tracking': 315_700,
  '/agents': 312_200,
  '/docs/api': 176_300,
};

function fail(message) {
  throw new Error(`size-limit: ${message}. Run "pnpm run build" first.`);
}

for (const path of [serverManifestPath, clientManifestPath]) {
  if (!existsSync(webDir + path)) fail(`${path} is missing`);
}
if (!existsSync(webDir + clientDir)) fail(`${clientDir} is missing`);

const { manifest } = await import(webDir + serverManifestPath);
const viteManifest = JSON.parse(readFileSync(webDir + clientManifestPath, 'utf8'));

function toPaths(files) {
  if (files.length === 0) fail('a budget resolved to no files');
  return files.map((file) => {
    const path = `${clientDir}/${file}`;
    if (!existsSync(webDir + path)) fail(`${path} is missing`);
    return path;
  });
}

const entryFiles = manifest.client?.imports;
if (!Array.isArray(entryFiles)) fail('client.imports is missing from the server manifest');

function routeFiles(id) {
  const route = manifest.routes.find((r) => r.id === id);
  if (!route?.page) fail(`route ${id} not found in the server manifest`);
  const nodes = [...route.page.layouts, route.page.leaf].filter((n) => n != null);
  const files = new Set();
  const seen = new Set();
  const visit = (key) => {
    if (seen.has(key)) return;
    seen.add(key);
    const chunk = viteManifest[key];
    if (!chunk) fail(`${key} is missing from the client manifest (route ${id})`);
    if (!entryFiles.includes(chunk.file)) files.add(chunk.file);
    chunk.imports?.forEach(visit);
  };
  nodes.forEach((n) => visit(nodeKey(n)));
  return [...files].filter((file) => file.endsWith('.js'));
}

export default [
  {
    name: 'entry',
    path: toPaths(entryFiles),
    brotli: true,
    limit: ENTRY_LIMIT,
  },
  ...Object.entries(ROUTE_LIMITS).map(([id, limit]) => ({
    name: `route ${id}`,
    path: toPaths(routeFiles(id)),
    brotli: true,
    limit,
  })),
];
