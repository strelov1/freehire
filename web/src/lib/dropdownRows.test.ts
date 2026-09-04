import { describe, it, expect } from 'vitest';
import { dropdownRows, namedCompanies } from './dropdownRows';
import type { Suggestion } from './suggestions';
import type { Job, CompanyListItem } from './types';

// The dropdown holds three sections — what to search, what exists, who is hiring —
// and the keyboard has to run through all of them as ONE list. Every off-by-one in
// arrow navigation lives here, so the flattening is a function rather than three
// index offsets computed in the template.

const suggestion = (slug: string): Suggestion => ({ kind: 'category', slug, label: slug });
const job = (slug: string): Job => ({ public_slug: slug, title: slug }) as unknown as Job;
const company = (slug: string): CompanyListItem => ({ slug, name: slug }) as unknown as CompanyListItem;

describe('dropdownRows', () => {
  it('runs suggestions, then postings, then companies', () => {
    const rows = dropdownRows({
      suggestions: [suggestion('a')],
      jobs: [job('j')],
      companies: [company('c')],
      text: 'x',
    });
    expect(rows.map((r) => r.kind)).toEqual(['suggestion', 'job', 'company', 'text']);
  });

  // The free-text row is last because it is the fallback, and absent on an empty box
  // because there is no text to offer searching for.
  it('offers the free-text row last, and only with text to search', () => {
    expect(dropdownRows({ suggestions: [], jobs: [], companies: [], text: 'x' }).at(-1)?.kind).toBe(
      'text',
    );
    expect(dropdownRows({ suggestions: [suggestion('a')], jobs: [], companies: [], text: '  ' })
      .map((r) => r.kind)).toEqual(['suggestion']);
  });

  // A pasted URL is not text anybody wrote to be searched for: the sections behind it
  // are answers to a full-text search that finds nothing, so the panel becomes one row.
  it('replaces the whole panel with the link row when the text is a link', () => {
    const link = { url: 'https://acme.com/jobs/1', ownSlug: null };
    const rows = dropdownRows({
      suggestions: [suggestion('a')],
      jobs: [job('j')],
      companies: [company('c')],
      text: 'https://acme.com/jobs/1',
      link,
    });
    expect(rows).toEqual([{ kind: 'link', link, key: 'l', first: true }]);
  });

  it('keeps the ordinary panel when the text is not a link', () => {
    const rows = dropdownRows({
      suggestions: [suggestion('a')],
      jobs: [],
      companies: [],
      text: 'go',
      link: null,
    });
    expect(rows.map((r) => r.kind)).toEqual(['suggestion', 'text']);
  });

  it('skips a section that has nothing rather than leaving a gap', () => {
    const rows = dropdownRows({
      suggestions: [],
      jobs: [job('j')],
      companies: [],
      text: 'x',
    });
    expect(rows.map((r) => r.kind)).toEqual(['job', 'text']);
  });

  it('numbers rows continuously so one arrow press crosses a section boundary', () => {
    const rows = dropdownRows({
      suggestions: [suggestion('a'), suggestion('b')],
      jobs: [job('j')],
      companies: [company('c')],
      text: 'x',
    });
    expect(rows.map((_, i) => i)).toEqual([0, 1, 2, 3, 4]);
    expect(rows[2]?.kind).toBe('job');
  });

  it('marks the first row of each section so a heading renders once', () => {
    const rows = dropdownRows({
      suggestions: [suggestion('a'), suggestion('b')],
      jobs: [job('j')],
      companies: [],
      text: 'x',
    });
    expect(rows.map((r) => r.first)).toEqual([true, false, true, true]);
  });

  it('gives every row a key that cannot collide across sections', () => {
    // `backend` is both a role slug and a company slug; a key of the slug alone would
    // duplicate, and a duplicate key in an {#each} is what took the site down before.
    const rows = dropdownRows({
      suggestions: [suggestion('backend')],
      jobs: [job('backend')],
      companies: [company('backend')],
      text: 'x',
    });
    const keys = rows.map((r) => r.key);
    expect(new Set(keys).size).toBe(keys.length);
  });

  it('is empty when there is nothing to show at all', () => {
    expect(dropdownRows({ suggestions: [], jobs: [], companies: [], text: '' })).toEqual([]);
  });
});

// The companies endpoint matches fuzzily, which is right for a companies PAGE and
// wrong for a dropdown section three rows tall. Observed in the browser: typing
// `product own` offered Energise.pro, CLOVERDALE FOODS CO and GreenStar Food Co+op —
// none of them anything to do with the query, and enough to make the whole dropdown
// look broken.
describe('namedCompanies', () => {
  const named = (...names: string[]) =>
    names.map((name) => ({ name, slug: name })) as unknown as CompanyListItem[];

  it('drops a company the query does not name', () => {
    const got = namedCompanies(named('Energise.pro', 'CLOVERDALE FOODS CO'), 'product own');
    expect(got).toEqual([]);
  });

  it('keeps a company the query names', () => {
    expect(namedCompanies(named('Google'), 'google').map((c) => c.name)).toEqual(['Google']);
  });

  it('keeps a company the query has only started naming', () => {
    expect(namedCompanies(named('Google'), 'goog').map((c) => c.name)).toEqual(['Google']);
  });

  it('ignores case, since a company writes its own name however it likes', () => {
    expect(namedCompanies(named('CLOVERDALE FOODS CO'), 'cloverdale').map((c) => c.name)).toEqual([
      'CLOVERDALE FOODS CO',
    ]);
  });

  // Observed in the browser: `java dev` offered "Senior Lead Java Developer near York,
  // UK" and "Solicitud De Empleo Para Java Developer En Encora" — job titles sitting in
  // the company field, which is what aggregators with no employer column produce. A
  // containment rule admits them honestly: the words really are in there.
  it('drops a name that merely contains the query somewhere inside it', () => {
    const got = namedCompanies(named('Senior Lead Java Developer near York, UK'), 'java dev');
    expect(got).toEqual([]);
  });

  // `reject` looks like it should reach The Reject Shop. It does not, and that is not
  // this function's doing: `/companies?q=reject` returns nothing at all, so the row
  // never arrives to be judged. Asserting the prefix rule rather than a leading-article
  // exception keeps the test describing what the system does.
  it('judges by the whole name, article and all', () => {
    expect(namedCompanies(named('The Reject Shop'), 'reject')).toEqual([]);
    expect(namedCompanies(named('The Reject Shop'), 'the reject').map((c) => c.name)).toEqual([
      'The Reject Shop',
    ]);
  });

  it('keeps the longer spellings of a company the query starts', () => {
    const got = namedCompanies(named('Google', 'GOOGLE ASIA PACIFIC PTE. LTD.', 'Google India'), 'google');
    expect(got).toHaveLength(3);
  });

  it('offers nothing for an empty query rather than everything', () => {
    expect(namedCompanies(named('Google'), '   ')).toEqual([]);
  });

  it('caps how many survive', () => {
    const got = namedCompanies(named('Googol A', 'Googol B', 'Googol C', 'Googol D'), 'googol', 3);
    expect(got).toHaveLength(3);
  });
});
