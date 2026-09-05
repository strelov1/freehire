import { describe, expect, it } from 'vitest';
import { accountNav } from '../accountNav';
import { messages } from './shell';
import { t } from './t';

// The nav catalog is keyed by href and falls back to accountNav's own English
// label for a href it has no entry for. That fallback is right — a forgotten
// section renders in English rather than blank — but it is also silent, and it
// stayed silent for three sections at once (`/my/integrations`, `/my/lists`,
// `/my/webhook`), each added weeks after the catalog was written.
//
// These tests are the noise that fallback deliberately does not make. Each one
// collects every offender before asserting, so a run reports all of them at
// once — which is what the three-at-a-time drift actually needed.
//
// Scoped to `en` and `ru`: the four supported-but-untranslated locales carry no
// navItems map at all, and requiring one would forbid exactly the incremental
// translation the catalog is shaped for.
describe('account nav label catalog', () => {
  const hrefs: string[] = accountNav.map((item) => item.href);
  const en: Record<string, string> = t(messages, 'en').navItems;
  const ru: Record<string, string> = t(messages, 'ru').navItems;

  // A section whose Russian label is legitimately the same word as its English
  // one belongs here, so that "translated to identical text" stays a deliberate,
  // reviewed statement rather than something the drift check cannot tell from
  // "nobody wrote a translation". Empty today.
  const SAME_IN_RUSSIAN = new Set<string>();

  it('has an English label for every navigation section', () => {
    const missing = hrefs.filter((href) => !(href in en));
    expect(missing, 'navigation sections with no entry in the label catalog').toEqual([]);
  });

  it('has a Russian label for every navigation section', () => {
    // `t(messages, 'ru')` is the Russian translation merged over the English
    // source, so an untranslated section is present but still English — the
    // per-key fallback working as designed. Equality with the English label is
    // therefore the signal, and the only one available from a merged catalog.
    const untranslated = hrefs.filter(
      (href) => href in en && !SAME_IN_RUSSIAN.has(href) && ru[href] === en[href],
    );
    expect(untranslated, 'navigation sections still showing their English label in Russian').toEqual(
      [],
    );
  });

  it('English label matches the navigation item it renders for', () => {
    // `navLabel` returns `navItems[href] ?? item.label`, and the catalog now
    // covers every href — so `accountNav`'s own `label` is unreachable, and
    // renaming a section there changes nothing on screen. That file reads like
    // the place a label is authored (it carries the naming rationale, and
    // accountNav.test.ts asserts one label outright), so the two must be kept
    // saying the same thing. Adding a section is caught above; renaming one is
    // caught here, and renaming is the likelier edit.
    const drifted = accountNav
      .filter((item) => item.href in en && en[item.href] !== item.label)
      .map((item) => `${item.href}: catalog "${en[item.href]}" vs nav "${item.label}"`);
    expect(drifted, 'sections whose catalog label no longer matches accountNav.ts').toEqual([]);
  });

  it('carries no label for a section the navigation no longer has', () => {
    const stale = Object.keys(en).filter((href) => !hrefs.includes(href));
    expect(stale, 'labels left behind by a removed or renamed section').toEqual([]);
  });
});
