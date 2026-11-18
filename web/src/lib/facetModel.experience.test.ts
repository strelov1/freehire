import { describe, it, expect } from 'vitest';
import { emptyFilters, filtersToParams, filtersFromParams, activeFilterCount } from './facetModel';

describe('experienceYearsMax', () => {
  it('is absent from the URL when unset', () => {
    expect(filtersToParams(emptyFilters()).has('experience_years_max')).toBe(false);
  });

  it('serializes a bound and reads it back', () => {
    const f = emptyFilters();
    f.experienceYearsMax = 3;
    const p = filtersToParams(f);
    expect(p.get('experience_years_max')).toBe('3');
    expect(filtersFromParams(p).experienceYearsMax).toBe(3);
  });

  // Zero is the entry-level selector, not "unset". A falsy guard here would drop the
  // one bound juniors need, and the URL would silently widen back to the whole
  // catalogue while the control still showed the leftmost stop.
  it('serializes a zero bound rather than treating it as unset', () => {
    const f = emptyFilters();
    f.experienceYearsMax = 0;
    const p = filtersToParams(f);
    expect(p.get('experience_years_max')).toBe('0');
    expect(filtersFromParams(p).experienceYearsMax).toBe(0);
  });

  // `Number(' ')` is 0 and `' '` is truthy, so a whitespace-only value slips past a
  // naive presence check and lands on the entry-level filter — a shared or
  // hand-edited link would narrow to almost nothing without saying why.
  it('reads a junk, blank or negative URL value back as no bound', () => {
    for (const raw of ['', ' ', '%20', 'abc', '-1', '2.5']) {
      const back = filtersFromParams(new URLSearchParams(`experience_years_max=${raw}`));
      expect(back.experienceYearsMax, `experience_years_max=${raw}`).toBeNull();
    }
  });

  // The server reads this param untrimmed, so ` 3` is no bound there. Trimming here
  // would show an "Up to 3 years" chip over a list the server never narrowed.
  it('reads a value with surrounding whitespace as no bound, like the server', () => {
    for (const raw of ['%203', '3%20', '%203%20']) {
      const f = filtersFromParams(new URLSearchParams(`experience_years_max=${raw}`));
      expect(f.experienceYearsMax, `experience_years_max=${raw}`).toBeNull();
      expect(activeFilterCount(f), `experience_years_max=${raw}`).toBe(0);
    }
  });

  it('still reads plain 0 and 3 as bounds', () => {
    expect(filtersFromParams(new URLSearchParams('experience_years_max=0')).experienceYearsMax).toBe(0);
    expect(filtersFromParams(new URLSearchParams('experience_years_max=3')).experienceYearsMax).toBe(3);
  });

  it('counts as one active filter, including at zero', () => {
    const f = emptyFilters();
    f.experienceYearsMax = 0;
    expect(activeFilterCount(f)).toBe(1);
  });
});

describe('experienceIncludeUnspecified', () => {
  it('serializes beside a bound and reads it back', () => {
    const f = emptyFilters();
    f.experienceYearsMax = 3;
    f.experienceIncludeUnspecified = true;
    const p = filtersToParams(f);
    expect(p.get('experience_include_unspecified')).toBe('true');
    expect(filtersFromParams(p).experienceIncludeUnspecified).toBe(true);
  });

  // Without a bound there is nothing to widen, so the param would be noise in every
  // shared link and saved search that once had a bound and then cleared it.
  it('is not written when no bound is set', () => {
    const f = emptyFilters();
    f.experienceIncludeUnspecified = true;
    expect(filtersToParams(f).has('experience_include_unspecified')).toBe(false);
  });

  it('does not count as a filter of its own', () => {
    const f = emptyFilters();
    f.experienceYearsMax = 3;
    f.experienceIncludeUnspecified = true;
    expect(activeFilterCount(f)).toBe(1);
  });
});

describe('salary bounds', () => {
  it('serializes each bound under its own param and reads both back', () => {
    const f = emptyFilters();
    f.salaryMin = 100000;
    f.salaryMax = 150000;
    const p = filtersToParams(f);
    expect(p.get('salary_min')).toBe('100000');
    expect(p.get('salary_max')).toBe('150000');
    const back = filtersFromParams(p);
    expect(back.salaryMin).toBe(100000);
    expect(back.salaryMax).toBe(150000);
  });

  it('writes nothing for an unset ceiling, and leaves the floor alone without one', () => {
    expect(filtersToParams(emptyFilters()).has('salary_max')).toBe(false);
    expect(filtersFromParams(new URLSearchParams('salary_max=150000')).salaryMin).toBeNull();
    expect(filtersFromParams(new URLSearchParams('salary_min=100000')).salaryMax).toBeNull();
  });

  // The /jobs loader forwards the address bar to the search API verbatim, so a bound
  // the server reads and this parser does not is applied to the first page with no
  // control showing it — then dropped for good by the next filter change, which
  // re-serializes from this model. Go reads both params with strconv.Atoi.
  it('rejects the forms strconv.Atoi rejects, so the two ends cannot disagree', () => {
    for (const raw of ['', ' ', 'abc', '1e5', '1.5', '0x10', ' 150000', '150000 ', 'Infinity']) {
      const p = new URLSearchParams();
      p.set('salary_min', raw);
      p.set('salary_max', raw);
      const f = filtersFromParams(p);
      expect(f.salaryMin, `salary_min=${JSON.stringify(raw)}`).toBeNull();
      expect(f.salaryMax, `salary_max=${JSON.stringify(raw)}`).toBeNull();
    }
  });

  // ...and keeps every value it accepts, zero included: the server applies
  // `salary_max=0`, so reading it as unset here would leave an empty list that no
  // chip explains.
  it('keeps the integer forms strconv.Atoi accepts', () => {
    for (const [raw, want] of [
      ['150000', 150000],
      ['+150000', 150000],
      ['0150000', 150000],
      ['0', 0],
    ] as const) {
      const p = new URLSearchParams();
      p.set('salary_max', raw);
      expect(filtersFromParams(p).salaryMax, raw).toBe(want);
    }
  });

  it('counts the range as one active filter, however many ends are set', () => {
    const none = activeFilterCount(emptyFilters());
    const f = emptyFilters();
    f.salaryMax = 150000;
    expect(activeFilterCount(f)).toBe(none + 1);
    f.salaryMin = 100000;
    expect(activeFilterCount(f)).toBe(none + 1);
  });
});

describe('seniorityIncludeUnspecified', () => {
  it('serializes beside an included level and reads it back', () => {
    const f = emptyFilters();
    f.facets.seniority = { include: ['intern'], exclude: [], matchAll: false };
    f.seniorityIncludeUnspecified = true;
    const p = filtersToParams(f);
    expect(p.get('seniority_include_unspecified')).toBe('true');
    expect(filtersFromParams(p).seniorityIncludeUnspecified).toBe(true);
  });

  // Excluding levels already keeps the unlabelled postings, and with nothing included
  // there is no group to widen — so the param would only be noise in a shared link.
  it('is not written without an included level', () => {
    const f = emptyFilters();
    f.facets.seniority = { include: [], exclude: ['senior'], matchAll: false };
    f.seniorityIncludeUnspecified = true;
    expect(filtersToParams(f).has('seniority_include_unspecified')).toBe(false);
  });
});
