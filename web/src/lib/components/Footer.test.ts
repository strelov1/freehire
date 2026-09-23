import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// A SOURCE-TEXT AUDIT, not a mounted-component test — web/ has no component-test
// infrastructure at all (see jobActionStrip.test.ts's own comment: no Svelte
// plugin, no DOM in vitest.config.ts).
//
// The account shell (/my/*) needed a footer, but not the marketing one: the four
// link columns, the popular-collections strip and the Product Hunt badge are noise
// next to an app surface with its own sidebar nav. `compact` keeps everything below
// the bottom bar (copyright, cookie settings, social links, open-source note) and
// drops the rest, reusing the bottom bar's own markup rather than forking a second
// component.
const SOURCE = readFileSync(join(import.meta.dirname, 'Footer.svelte'), 'utf8');

// The offset of the `{/if}` that closes the `{#if !compact}` block: before it is the
// marketing chrome the compact footer drops, after it the bottom bar every footer keeps.
// Three of the tests below need that same boundary, and spelling the nested indexOf out
// each time is how the second and third copies came to differ from the first.
const GUARD_END = SOURCE.indexOf('{/if}', SOURCE.indexOf('productHunt.href'));

describe('Footer compact mode', () => {
  it('declares a compact prop defaulting to false', () => {
    expect(SOURCE).toMatch(/let\s*\{\s*compact\s*=\s*false\s*\}/);
  });

  it('skips the link groups, popular collections, and Product Hunt badge when compact', () => {
    const groupsAt = SOURCE.indexOf('{#each groups as group');
    const guardAt = SOURCE.lastIndexOf('{#if !compact}', groupsAt);
    expect(guardAt, 'expected an {#if !compact} guard before the link groups').toBeGreaterThan(-1);

    const producthuntAt = SOURCE.indexOf('productHunt.href');
    expect(GUARD_END, 'expected the {#if !compact} guard to still be open at the Product Hunt badge').toBeGreaterThan(producthuntAt);
  });

  it('still renders the bottom bar (copyright, cookie settings, social, open-source note) unconditionally', () => {
    const bottomBar = SOURCE.slice(GUARD_END);
    expect(bottomBar).toContain('©');
    expect(bottomBar).toContain('Cookie settings');
    expect(bottomBar).toContain('SOCIAL_LINKS');
    expect(bottomBar).toContain('View source on GitHub');
  });

  it('skips the store buttons when compact — they sit in the same guarded block', () => {
    const eachAt = SOURCE.indexOf('{#each stores as store');
    // Asserted before the comparison, not folded into it: indexOf answers -1 when the
    // block is gone, and -1 is less than any offset — so deleting the store buttons
    // outright would have satisfied "they are inside the guard".
    expect(eachAt, 'expected the store buttons to be rendered at all').toBeGreaterThan(-1);
    expect(eachAt).toBeLessThan(GUARD_END);
  });

  it("only borders the bottom bar's own top when compact (no divider from a section that isn't rendered)", () => {
    const commentAt = SOURCE.indexOf('Bottom bar:');
    const innerDivAt = SOURCE.indexOf('mx-auto flex max-w-6xl flex-col');
    const wrapperOpenTag = SOURCE.slice(commentAt, innerDivAt);
    expect(wrapperOpenTag).toContain('compact ?');
  });
});

describe('Footer store buttons', () => {
  it('links the App Store listing with NO country segment', () => {
    // The listing's own share link is `/br/app/...`, which pins every visitor to the
    // Brazilian storefront — a wrong-country page still loads, so nothing downstream
    // could catch it. Without a country Apple redirects each visitor to their own.
    expect(SOURCE).toContain('https://apps.apple.com/app/freehire-open-job-search/id6801885119');
    expect(SOURCE).not.toMatch(/apps\.apple\.com\/[a-z]{2}\//);
  });

  it('takes the Chrome Web Store URL from $lib/extensionLinks rather than spelling it again', () => {
    expect(SOURCE).toContain("import { EXTENSION_STORE_URL } from '$lib/extensionLinks'");
    expect(SOURCE).toContain('href: EXTENSION_STORE_URL');
    expect(SOURCE).not.toContain('chromewebstore.google.com');
  });

  it('opens both in a new tab with a safe rel', () => {
    const eachAt = SOURCE.indexOf('{#each stores as store');
    const block = SOURCE.slice(eachAt, SOURCE.indexOf('{/each}', eachAt));
    expect(block).toContain('target="_blank"');
    expect(block).toContain('rel="noopener noreferrer"');
  });
});
