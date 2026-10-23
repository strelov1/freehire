import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// A SOURCE-TEXT AUDIT. The anti-FOUC script in app.html is author-written, so
// SvelteKit does not nonce it: the CSP admits it by the SHA-256 of its exact text,
// written by hand into vite.config.ts. Any edit to that script — a renamed import in
// a comment was enough (#3441) — moves the hash, and the browser then blocks the
// whole block with nothing failing but a console line and a flash of the wrong theme.
const APP_HTML = readFileSync(join(import.meta.dirname, 'app.html'), 'utf8');
const VITE_CONFIG = readFileSync(join(import.meta.dirname, '..', 'vite.config.ts'), 'utf8');

describe('anti-FOUC script CSP hash', () => {
  it('is the hash vite.config.ts admits', () => {
    // Plain slicing between the exact tags, the same span the hash recipe in
    // vite.config.ts reads — app.html is our own file, not HTML to sanitise.
    const open = APP_HTML.indexOf('<script>');
    expect(open, 'app.html carries no inline <script>').toBeGreaterThanOrEqual(0);
    const start = open + '<script>'.length;
    const body = APP_HTML.slice(start, APP_HTML.indexOf('</script>', start));
    const hash = `sha256-${createHash('sha256').update(body).digest('base64')}`;
    expect(VITE_CONFIG).toContain(`'${hash}'`);
  });
});
