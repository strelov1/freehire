// Renders the /docs/api and /docs/api/internal Scalar fragments at BUILD time. A route
// imports its generated OpenAPI document with a `?scalar-html` suffix and gets the
// rendered HTML as a plain string; the client hydrates it with the same config (see
// scalarConfig.ts), so the two never disagree about what the spec says.
//
// Why not at request time: rendering the 250 KB spec on a cold server process blocked
// that worker's event loop for ~25 s in production, once per worker after every
// deploy. Every request queued on the worker stalled with it, and job pages whose API
// call had long since answered came back as 504s when their 10 s timeout fired first.
//
// Why not by prerendering the page: the root layout renders signed-in chrome from the
// session cookie, and `frame-ancestors` only exists as a response header (see the CSP
// in vite.config.ts) — a prerendered page would lose both.
//
// @scalar/server-side-rendering is imported dynamically here and nowhere in src/routes:
// a transitive dependency (stringify-object -> is-identifier -> super-regex ->
// make-asynchronous) starts a worker_threads.Worker at import time, which crashes
// inside SvelteKit's build-time route analysis worker. In a Vite plugin it runs in the
// main build process, and the server bundle never contains it at all.
import { readFile } from 'node:fs/promises';
import type { Plugin } from 'vite';
import { scalarConfigFromContent } from './scalarConfig.ts';

const SUFFIX = '?scalar-html';

export function scalarHtmlPlugin(): Plugin {
  return {
    name: 'freehire:scalar-html',
    load: {
      // The filter keeps the bundler from calling into JS for every other module.
      filter: { id: /\?scalar-html$/ },
      async handler(id) {
        const file = id.slice(0, -SUFFIX.length);
        this.addWatchFile(file);
        const spec = JSON.parse(await readFile(file, 'utf8')) as Record<string, unknown>;
        const { renderApiReferenceToString } = await import('@scalar/server-side-rendering');
        const html = await renderApiReferenceToString(scalarConfigFromContent(spec));
        // The id still ends in .json?…; without this the bundler could take the code for JSON.
        return { code: `export default ${JSON.stringify(html)};`, moduleType: 'js' };
      },
    },
  };
}
