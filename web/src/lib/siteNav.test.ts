import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// A SOURCE-TEXT AUDIT, not an import test — this module pulls in `@lucide/svelte`
// icon components, which are `.svelte` files vitest.config.ts cannot transform (no
// Svelte plugin; see jobActionStrip.test.ts's own comment on the same constraint).
const SOURCE = readFileSync(join(import.meta.dirname, 'siteNav.ts'), 'utf8');

describe('siteNav', () => {
  it('has an Open entry targeting /open', () => {
    expect(SOURCE).toMatch(/open:\s*\{\s*href:\s*'\/open',\s*label:\s*'Open'/);
  });

  // /open is a data page, reachable from the footer and the menu. The homepage row is
  // the shortcut to the catalogue, and it is not one of those.
  //
  // How LONG that row may be is asserted in siteNav.spec.ts instead, against the real
  // imported value and beside the measurement that decides the number. It was a second
  // `toHaveLength` here, which is how adding a destination failed a test whose own name
  // was about something else.
  it('does not add Open to HEADER_LINKS', () => {
    const headerLinks = SOURCE.slice(SOURCE.indexOf('export const HEADER_LINKS'));
    const listBody = headerLinks.slice(0, headerLinks.indexOf('] as const'));
    expect(listBody).not.toContain('NAV.open');
  });
});
