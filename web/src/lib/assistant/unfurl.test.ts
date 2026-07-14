import { describe, it, expect } from 'vitest';
import { parseJobSegments } from './unfurl';

describe('parseJobSegments', () => {
  it('returns a single markdown segment when there are no job links', () => {
    const segs = parseJobSegments('Here is some **advice** and a list.');
    expect(segs).toEqual([{ kind: 'markdown', text: 'Here is some **advice** and a list.' }]);
  });

  it('splits a bare job URL out of surrounding prose, in order', () => {
    const segs = parseJobSegments('Check https://freehire.dev/jobs/senior-go-dev now');
    expect(segs).toEqual([
      { kind: 'markdown', text: 'Check ' },
      { kind: 'job', slug: 'senior-go-dev', url: 'https://freehire.dev/jobs/senior-go-dev' },
      { kind: 'markdown', text: ' now' },
    ]);
  });

  it('unfurls a markdown-link form and drops the label (card shows the real title)', () => {
    const segs = parseJobSegments('- [Senior Go Engineer](https://freehire.dev/jobs/senior-go)');
    // The leading "- " list marker is dropped once the link becomes a card.
    expect(segs).toEqual([
      { kind: 'job', slug: 'senior-go', url: 'https://freehire.dev/jobs/senior-go' },
    ]);
  });

  it('stacks multiple links, dropping the whitespace between them', () => {
    const segs = parseJobSegments(
      'Top picks:\nhttps://freehire.dev/jobs/a\nhttps://freehire.dev/jobs/b',
    );
    // The lone "\n" between the two cards is whitespace-only → dropped, so the
    // cards stack cleanly.
    expect(segs.map((s) => s.kind)).toEqual(['markdown', 'job', 'job']);
    expect(segs.filter((s) => s.kind === 'job').map((s) => (s as { slug: string }).slug)).toEqual([
      'a',
      'b',
    ]);
  });

  it('drops list-marker-only fragments so a bulleted job list renders as clean cards', () => {
    const segs = parseJobSegments(
      '- [Role A](https://freehire.dev/jobs/a)\n- [Role B](https://freehire.dev/jobs/b)',
    );
    // The "- " and "\n- " fragments are list-marker-only → dropped.
    expect(segs.map((s) => s.kind)).toEqual(['job', 'job']);
  });

  it('keeps prose segments that merely contain a list marker', () => {
    const segs = parseJobSegments('Here are roles:\n\nhttps://freehire.dev/jobs/a');
    expect(segs[0]).toEqual({ kind: 'markdown', text: 'Here are roles:\n\n' });
    expect(segs[1]?.kind).toBe('job');
  });

  it('emits a card for each occurrence of a duplicated slug', () => {
    const segs = parseJobSegments(
      'https://freehire.dev/jobs/dup and again https://freehire.dev/jobs/dup',
    );
    const jobs = segs.filter((s) => s.kind === 'job');
    expect(jobs).toHaveLength(2);
    expect(jobs.every((j) => (j as { slug: string }).slug === 'dup')).toBe(true);
  });

  it('excludes trailing punctuation from the slug', () => {
    const segs = parseJobSegments('See https://freehire.dev/jobs/foo.');
    expect(segs).toEqual([
      { kind: 'markdown', text: 'See ' },
      { kind: 'job', slug: 'foo', url: 'https://freehire.dev/jobs/foo' },
      { kind: 'markdown', text: '.' },
    ]);
  });

  it('matches the www host', () => {
    const segs = parseJobSegments('https://www.freehire.dev/jobs/abc');
    expect(segs).toEqual([
      { kind: 'job', slug: 'abc', url: 'https://freehire.dev/jobs/abc' },
    ]);
  });

  it('leaves non-job freehire links as markdown', () => {
    const text = 'Company: https://freehire.dev/companies/acme and board https://freehire.dev/b/x';
    expect(parseJobSegments(text)).toEqual([{ kind: 'markdown', text }]);
  });

  it('leaves external /jobs/ links as markdown (host must be freehire)', () => {
    const text = 'External https://example.com/jobs/x here';
    expect(parseJobSegments(text)).toEqual([{ kind: 'markdown', text }]);
  });

  it('ignores a bare /jobs/slug with no host (avoids false positives)', () => {
    const text = 'the /jobs/ page lists roles';
    expect(parseJobSegments(text)).toEqual([{ kind: 'markdown', text }]);
  });
});
