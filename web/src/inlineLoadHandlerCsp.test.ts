import { globSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// A SOURCE-TEXT AUDIT. For `onload`/`onerror` on <img>, <iframe>, <link> and the other
// load elements, Svelte's SSR writes an inline `onerror="this.__e=event"` into the HTML to
// replay an event that lands before hydration. Our script-src CSP is nonce + hash, and no
// nonce or hash admits an attribute handler, so the browser blocks it and logs a violation
// for every broken image. `{@attach onImageError(...)}` (#lib/ui) does the same job with no
// SSR output. Plain text search, because ast-grep's HTML parser gives up on Svelte markup
// (`{src}`, `=>` inside a tag) and would pass silently.
const ROOT = join(import.meta.dirname, '..', '..');
const SOURCES = ['web/src', 'design-system/src'].flatMap((dir) => globSync(`${dir}/**/*.svelte`, { cwd: ROOT }));

describe('no SSR inline load/error handlers', () => {
  it('finds the components to audit', () => {
    expect(SOURCES.length).toBeGreaterThan(100);
  });

  it('uses onImageError instead of onload={…}/onerror={…}', () => {
    const offenders = SOURCES.filter((file) => /\son(?:load|error)=\{/.test(readFileSync(join(ROOT, file), 'utf8')));
    expect(offenders).toEqual([]);
  });
});
