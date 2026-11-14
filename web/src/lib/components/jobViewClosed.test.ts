import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// A SOURCE-TEXT AUDIT, for the reason jobViewDirectApply.test.ts gives: `web/` has no
// component-test infrastructure. A closed posting withholds every Apply control, and those
// carried the only link to the posting's own page — so a reader who doubted the closure had
// no way to check it (freehire#4021). The closure notice carries that link now, as a plain
// link rather than an Apply.
// The notice is its own component, which JobView renders under the title and above the CTAs.
const VIEW = readFileSync(join(import.meta.dirname, 'JobView.svelte'), 'utf8');
const NOTICE = readFileSync(join(import.meta.dirname, 'JobClosedNotice.svelte'), 'utf8');

const start = NOTICE.indexOf('{#if job.closed_at}');
const notice = start === -1 ? '' : NOTICE.slice(start);

describe('JobView closure notice', () => {
  it('finds the block it audits', () => {
    expect(notice).toContain('no longer accepting applications');
  });

  it('is rendered by JobView above the calls to action', () => {
    const at = VIEW.indexOf('<JobClosedNotice');
    expect(at).toBeGreaterThan(-1);
    expect(at).toBeLessThan(VIEW.indexOf('{@render ctaGroup()}', at));
  });

  it("links the posting's own page, unfollowed", () => {
    expect(notice).toContain('href={job.url}');
    expect(notice).toContain('rel="nofollow noopener noreferrer"');
  });

  it('offers no outside page for a vacancy published here', () => {
    expect(notice).toContain('{#if job.url && !job.direct_apply}');
  });

  it('is not counted as an application', () => {
    expect(notice).not.toContain('onApplyClick');
  });
});
