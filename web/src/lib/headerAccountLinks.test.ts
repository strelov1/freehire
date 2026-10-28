import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// A SOURCE-TEXT AUDIT, not an import test — this module pulls in `@lucide/svelte`
// icon components, which are `.svelte` files vitest.config.ts cannot transform (no
// Svelte plugin; see jobActionStrip.test.ts's own comment on the same constraint).
//
// The header's own curated subset of the account nav — shared by HeaderMenu's
// mobile drawer and HeaderProfileMenu's desktop panel, so the two cannot drift the
// way siteNav.ts's own comment describes happening to HEADER_LINKS/NAV before that
// list was unified.
const SOURCE = readFileSync(join(import.meta.dirname, 'headerAccountLinks.ts'), 'utf8');

describe('headerAccountLinks', () => {
  it('lists the header account items in order', () => {
    const hrefs = [...SOURCE.matchAll(/href:\s*'([^']+)'/g)].map((m) => m[1]);
    expect(hrefs).toEqual([
      '/my/activity',
      '/my/tracking',
      '/my/inbox',
      '/my/assistant',
      '/my/cvs',
      '/my/notifications/searches',
      '/my/integrations/api-keys',
      '/my/submissions',
    ]);
  });

  it('labels each item for display', () => {
    const labels = [...SOURCE.matchAll(/label:\s*'([^']+)'/g)].map((m) => m[1]);
    expect(labels).toEqual([
      'Activity',
      'Tracking',
      'Inbox',
      'Agent',
      'Tailor',
      'Search alerts',
      'API keys',
      'My submissions',
    ]);
  });

  it('exports the accountLinks array', () => {
    expect(SOURCE).toContain('export const accountLinks');
  });
});
