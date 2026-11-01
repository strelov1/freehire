import { describe, expect, it } from 'vitest';
import { activeFilterCount, canonicalQuery, emptyFilters, filtersFromParams, filtersToParams } from './facetModel';
import { FACETS } from './facets';
import { EQUITY_VALUES } from './generated/contracts';
import { SHORT_APPLICATION_MAX } from './jobFacts';

// Equity is a chip facet over the generated vocabulary plus the reserved `not_stated`
// (the attribute's absence, search.EquityNotStated). Short application is a numeric
// bound, `apply_questions_max`, which the modal's toggle sets to SHORT_APPLICATION_MAX.
describe('equity facet', () => {
  const def = FACETS.find((f) => f.param === 'equity');

  it('offers every generated value, then the uncounted not-stated chip', () => {
    const opts = def?.options ?? [];
    expect(opts.map((o) => o.value)).toEqual([...EQUITY_VALUES, 'not_stated']);
    expect(opts.map((o) => o.label)).toEqual(['Salary + equity', 'Equity only', 'Not stated']);
    expect(opts.find((o) => o.value === 'not_stated')?.uncounted).toBe(true);
    expect(def?.excludable).toBe(true);
  });

  it('round-trips included and excluded values through the URL', () => {
    const f = emptyFilters();
    f.facets.equity = { include: ['salary_and_equity', 'equity_only'], exclude: ['not_stated'], matchAll: false };
    const p = filtersToParams(f);
    expect(p.get('equity')).toBe('salary_and_equity,equity_only');
    expect(p.get('equity_exclude')).toBe('not_stated');
    expect(filtersFromParams(p).facets.equity).toEqual(f.facets.equity);
  });

  it('counts each selected value towards the filter badge', () => {
    expect(activeFilterCount(filtersFromParams(new URLSearchParams('equity=equity_only,not_stated')))).toBe(2);
  });
});

describe('short application bound', () => {
  it('writes no param by default', () => {
    expect(filtersToParams(emptyFilters()).has('apply_questions_max')).toBe(false);
  });

  it('round-trips the toggle bound through the URL', () => {
    const f = emptyFilters();
    f.applyQuestionsMax = SHORT_APPLICATION_MAX;
    const p = filtersToParams(f);
    expect(p.get('apply_questions_max')).toBe('3');
    expect(filtersFromParams(p).applyQuestionsMax).toBe(3);
  });

  // Zero is a real bound ("nothing beyond the basics"), and a bound from a link or the
  // API is kept as it came rather than snapped to the toggle's.
  it.each([
    ['0', 0],
    ['5', 5],
    [' 2 ', 2],
  ])('reads apply_questions_max=%s as %i', (raw, want) => {
    expect(filtersFromParams(new URLSearchParams({ apply_questions_max: raw })).applyQuestionsMax).toBe(want);
  });

  it.each(['', ' ', '-1', '1.5', 'three'])('ignores the malformed bound %j', (raw) => {
    expect(filtersFromParams(new URLSearchParams({ apply_questions_max: raw })).applyQuestionsMax).toBeNull();
  });

  it('counts once towards the filter badge', () => {
    expect(activeFilterCount(filtersFromParams(new URLSearchParams('apply_questions_max=3')))).toBe(1);
  });

  // An ordinary search filter, so a saved search keeps it (unlike hide_seen).
  it('survives into the saved-search query', () => {
    expect(canonicalQuery('apply_questions_max=3&equity=equity_only')).toBe('equity=equity_only&apply_questions_max=3');
  });
});
