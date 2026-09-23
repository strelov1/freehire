import { describe, it, expect } from 'vitest';
import { DISALLOWED, SEARCH_CRAWLERS, robotsBody } from './robots';

const BODY = robotsBody('https://freehire.me');

// Parse the way a crawler does: a User-agent line opens a group, and the directives
// under it belong to that group until the next one. Asserting on the raw string
// instead would pass for a file whose rules sit in the wrong group.
function groups(body: string): Map<string, string[]> {
  const parsed = new Map<string, string[]>();
  let current: string[] | undefined;
  for (const line of body.split('\n')) {
    const agent = line.match(/^User-agent:\s*(.+)$/)?.[1];
    if (agent) {
      current = [];
      parsed.set(agent.trim(), current);
      continue;
    }
    const disallow = line.match(/^Disallow:\s*(.+)$/)?.[1];
    if (disallow && current) current.push(disallow.trim());
    if (line.startsWith('#')) current = undefined;
  }
  return parsed;
}

describe('robots.txt', () => {
  it('gives every crawler the wildcard group', () => {
    expect(groups(BODY).get('*')).toEqual([...DISALLOWED]);
  });

  // The rule this file exists to protect. A crawler obeys the single most specific
  // group that matches it and ignores the wildcard entirely, so a named group that
  // lists only /api/ hands Google /my/ and the new-thread form.
  it.each(SEARCH_CRAWLERS)('repeats every wildcard rule in the %s group', (agent) => {
    const rules = groups(BODY).get(agent);
    expect(rules).toBeDefined();
    for (const path of DISALLOWED) expect(rules).toContain(path);
  });

  it('takes /api/ from the search crawlers and only them', () => {
    const parsed = groups(BODY);
    expect(parsed.get('*')).not.toContain('/api/');
    for (const agent of SEARCH_CRAWLERS) expect(parsed.get(agent)).toContain('/api/');
  });

  // /signin is disallowed for the reason the new-thread form is, only larger: every
  // job page links to it carrying that page's own URL in `returnTo`, so its address
  // space is the size of the catalogue. Measured 2026-09-20: 127,799 fetches across
  // 41,180 distinct /signin URLs in one day, 10% of everything the host served.
  //
  // Asserted on the wildcard group specifically, because that is the group the AI
  // crawlers doing this read. A named group would leave them on the old rules.
  it('keeps every crawler off /signin', () => {
    expect(groups(BODY).get('*')).toContain('/signin');
    for (const agent of SEARCH_CRAWLERS) expect(groups(BODY).get(agent)).toContain('/signin');
  });

  // The discussion rule covers the SUBTREE, not just the new-thread form under it.
  // A `Disallow` is a prefix match, so the bare path closes /discussion itself,
  // /discussion/new and /discussion/[id] in one line — and the bare path is the
  // point: measured 2026-09-22, the thread pages themselves were empty shells, 16
  // of 16 sampled. Asserting the prefix rather than the old /new suffix is what
  // stops a well-meant narrowing from silently reopening ~321k identical pages.
  it('keeps every crawler off the whole discussion subtree', () => {
    for (const agent of ['*', ...SEARCH_CRAWLERS]) {
      const rules = groups(BODY).get(agent);
      expect(rules).toContain('/jobs/*/discussion');
      expect(rules).toContain('/companies/*/discussion');
      expect(rules).not.toContain('/companies/*/discussion/new');
    }
  });

  // The API pointer is aimed at agents, which read the wildcard group — a crawler
  // that follows it is the cheap outcome the comment block argues for.
  it('still advertises the API and the sitemap', () => {
    expect(BODY).toContain('Sitemap: https://freehire.me/sitemap.xml');
    expect(BODY).toContain('https://freehire.me/api/v1/jobs/search?q=golang');
    expect(BODY).toContain('https://freehire.me/llms.txt');
  });
});
