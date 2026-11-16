import { describe, it, expect } from 'vitest';
import {
  emptyFacet,
  emptyFilters,
  filtersToParams,
  filtersFromParams,
  filtersWithParts,
  displayQuery,
  activeFilterCount,
  type FacetState,
  type JobFilters,
} from './facetModel';
import { canonicalQuery, savedSearchQuery, queryNarrows } from './savedSearchQuery';
import { signOf, facetSetSign, facetCycle, facetPick, facetToggleSign, facetAdd, facetRemove } from './facetSign';
import { withQuery, withSkills } from './facetModelTestUtils';
import { must } from './utils';

// The skills facet is always present (emptyFacets seeds every FACET), so this read
// is safe; the helper drops the index-signature `| undefined` for terser asserts.
const sk = (f: JobFilters): FacetState => must(f.facets.skills);

describe('emptyFacet', () => {
  it('is two empty sets and OR mode', () => {
    expect(emptyFacet()).toEqual({ include: [], exclude: [], matchAll: false });
  });
});

describe('filtersToParams', () => {
  it('serializes include to one comma-joined bare param and exclude to <param>_exclude', () => {
    const p = filtersToParams(withSkills({ include: ['nodejs', 'react'], exclude: ['php', 'java'] }));
    expect(p.getAll('skills')).toEqual(['nodejs,react']);
    expect(p.getAll('skills_exclude')).toEqual(['php,java']);
  });

  it('emits <param>_mode=and only when matchAll and more than one included value', () => {
    expect(filtersToParams(withSkills({ include: ['go', 'rust'], matchAll: true })).get('skills_mode')).toBe('and');
    expect(filtersToParams(withSkills({ include: ['go'], matchAll: true })).get('skills_mode')).toBeNull();
    expect(filtersToParams(withSkills({ include: ['go', 'rust'], matchAll: false })).get('skills_mode')).toBeNull();
  });

  it('omits a facet with no values', () => {
    expect(filtersToParams(emptyFilters()).toString()).toBe('');
  });

  // q_fields is how a title suggestion's click restricts `q` to the title field (see
  // apiSuggestions.ts) — it has to survive filtersToParams or it never reaches the
  // list's own reload, pagination, or facet-count requests, all of which read the
  // filter state through this function rather than the original navigation URL.
  it('serializes qFields as a comma-joined q_fields param', () => {
    const p = filtersToParams({ ...emptyFilters(), qFields: ['title'] });
    expect(p.get('q_fields')).toBe('title');
  });

  it('omits q_fields when unset', () => {
    expect(filtersToParams(emptyFilters()).has('q_fields')).toBe(false);
  });
});

describe('filtersFromParams', () => {
  it('reads the bare param into include and <param>_exclude into exclude', () => {
    const f = filtersFromParams(new URLSearchParams('skills=nodejs&skills_exclude=php&skills_exclude=.net'));
    expect(sk(f).include).toEqual(['nodejs']);
    expect(sk(f).exclude).toEqual(['php', '.net']);
  });

  it('reads <param>_mode=and into matchAll', () => {
    const f = filtersFromParams(new URLSearchParams('skills=go&skills=rust&skills_mode=and'));
    expect(sk(f).matchAll).toBe(true);
  });

  it('drops a value from include when it also appears in exclude (exclude wins)', () => {
    const f = filtersFromParams(new URLSearchParams('skills=php&skills_exclude=php'));
    expect(sk(f).include).toEqual([]);
    expect(sk(f).exclude).toEqual(['php']);
  });

  it('de-duplicates repeated URL values', () => {
    const f = filtersFromParams(new URLSearchParams('skills=go&skills=go'));
    expect(sk(f).include).toEqual(['go']);
  });

  it('de-duplicates repeated exclude values', () => {
    const f = filtersFromParams(new URLSearchParams('skills_exclude=go&skills_exclude=go'));
    expect(sk(f).exclude).toEqual(['go']);
  });

  it('splits a comma-joined value into multiple included values', () => {
    const f = filtersFromParams(new URLSearchParams('skills=go,react'));
    expect(sk(f).include).toEqual(['go', 'react']);
  });

  it('splits a comma-joined value in <param>_exclude too', () => {
    const f = filtersFromParams(new URLSearchParams('skills_exclude=java,cpp'));
    expect(sk(f).exclude).toEqual(['java', 'cpp']);
  });

  it('still parses the old repeated-key form (backward compatibility)', () => {
    const f = filtersFromParams(new URLSearchParams('skills=go&skills=react&skills=aws'));
    expect(sk(f).include).toEqual(['go', 'react', 'aws']);
  });

  it('unions a comma-joined entry with a repeated key for the same param', () => {
    const f = filtersFromParams(new URLSearchParams('skills=go,react&skills=aws'));
    expect(sk(f).include).toEqual(['go', 'react', 'aws']);
  });

  it('drops stray/doubled commas without producing an empty value', () => {
    const f = filtersFromParams(new URLSearchParams('skills=go,,react,'));
    expect(sk(f).include).toEqual(['go', 'react']);
  });

  it('reads q_fields into qFields', () => {
    expect(filtersFromParams(new URLSearchParams('q_fields=title')).qFields).toEqual(['title']);
  });

  it('reads a missing q_fields as null, not an empty array', () => {
    expect(filtersFromParams(new URLSearchParams('')).qFields).toBeNull();
  });
});

describe('filtersToParams / filtersFromParams round-trip', () => {
  it('round-trips a multi-value facet through the new comma-joined serialization', () => {
    const f = withSkills({ include: ['go', 'react', 'aws'], exclude: ['java'] });
    const back = filtersFromParams(filtersToParams(f));
    expect(sk(back).include).toEqual(['go', 'react', 'aws']);
    expect(sk(back).exclude).toEqual(['java']);
  });

  it('round-trips an old-style repeated-key URL to the same filter state as the new form', () => {
    const oldStyle = filtersFromParams(new URLSearchParams('skills=go&skills=react'));
    const newStyle = filtersFromParams(new URLSearchParams('skills=go,react'));
    expect(oldStyle).toEqual(newStyle);
  });

  it('round-trips qFields', () => {
    const f = { ...emptyFilters(), qFields: ['title'] };
    expect(filtersFromParams(filtersToParams(f)).qFields).toEqual(['title']);
  });
});

// A completion from the suggestions endpoint composes a phrase — see the doc comment
// on filtersWithParts. qFields travels with q as ONE discrete replacement: applying a
// suggestion with no title part must clear whatever restriction a PREVIOUS suggestion
// left behind, not merge with it.
describe('filtersWithParts', () => {
  it('sets q and every named facet in one write', () => {
    const got = filtersWithParts(emptyFilters(), [['category', 'backend']], 'Backend Engineer');
    expect(got.q).toBe('Backend Engineer');
    expect(got.facets.category?.include).toEqual(['backend']);
  });

  it('sets qFields when given', () => {
    const got = filtersWithParts(emptyFilters(), [], '"Founding Engineer"', ['title']);
    expect(got.qFields).toEqual(['title']);
  });

  it('clears a previous qFields when applying a suggestion with none of its own', () => {
    const withRestriction = { ...emptyFilters(), qFields: ['title'] };
    const got = filtersWithParts(withRestriction, [['category', 'backend']], 'Backend Engineer');
    expect(got.qFields).toBeNull();
  });
});

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

describe('autoApplyAvailable', () => {
  it('is absent from the URL when unset', () => {
    expect(filtersToParams(emptyFilters()).has('auto_apply_available')).toBe(false);
  });

  it('serializes true and reads it back', () => {
    const f = emptyFilters();
    f.autoApplyAvailable = true;
    const p = filtersToParams(f);
    expect(p.get('auto_apply_available')).toBe('true');
    expect(filtersFromParams(p).autoApplyAvailable).toBe(true);
  });

  // True-or-absent, like the backend facet it drives: turning the checkbox off
  // must remove the param entirely, never write `auto_apply_available=false` —
  // the index never carries an explicit false, so that value would silently
  // filter to nothing instead of clearing the filter.
  it('never serializes false — omits the param instead', () => {
    const f = emptyFilters();
    f.autoApplyAvailable = false;
    expect(filtersToParams(f).has('auto_apply_available')).toBe(false);
  });

  it('counts as one active filter when true', () => {
    const f = emptyFilters();
    f.autoApplyAvailable = true;
    expect(activeFilterCount(f)).toBe(1);
  });
});

describe('hideSeen', () => {
  it('serializes as hide_seen=true and reads it back', () => {
    const p = filtersToParams({ ...emptyFilters(), hideSeen: true });
    expect(p.get('hide_seen')).toBe('true');
    expect(filtersFromParams(p).hideSeen).toBe(true);
  });

  it('is absent when off, and only the literal true turns it on', () => {
    expect(filtersToParams(emptyFilters()).has('hide_seen')).toBe(false);
    expect(filtersFromParams(new URLSearchParams('hide_seen=1')).hideSeen).toBe(false);
  });

  // A feed preference, like the ordering: it is not a filter of the catalogue, so the
  // badge and the clear-all count must not treat it as one.
  it('is not counted as an active filter', () => {
    expect(activeFilterCount({ ...emptyFilters(), hideSeen: true })).toBe(0);
  });

  // A digest has no "seen" to apply, so a saved search must neither store the flag
  // nor read as dirty when the visitor toggles it.
  it('is left out of the saved-search query', () => {
    const f: JobFilters = { ...emptyFilters(), q: 'go', hideSeen: true };
    expect(new URLSearchParams(savedSearchQuery(f)).has('hide_seen')).toBe(false);
    expect(savedSearchQuery(f)).toBe(savedSearchQuery({ ...f, hideSeen: false }));
  });
});

describe('the retired role facet', () => {
  // `role` is gone from FACETS, so emptyFilters no longer seeds it and the generic
  // param path has nothing to serialize. A stale `role=` in a URL is simply not read —
  // the API reports it through meta.ignored_params instead.
  it('is not a declared facet, so a stale param round-trips to nothing', () => {
    expect(emptyFilters().facets.role).toBeUndefined();

    const back = filtersFromParams(new URLSearchParams('role=senior_backend&category=backend'));
    expect(back.facets.role).toBeUndefined();
    expect(must(back.facets.category).include).toEqual(['backend']);
  });
});

describe('canonicalQuery', () => {
  it('is idempotent for a mixed include/exclude query', () => {
    const q = 'skills=nodejs&skills_exclude=php';
    const once = canonicalQuery(q);
    expect(canonicalQuery(once)).toBe(once);
    expect(new URLSearchParams(once).getAll('skills')).toEqual(['nodejs']);
    expect(new URLSearchParams(once).getAll('skills_exclude')).toEqual(['php']);
  });
});

// The ordering is a view preference, not part of WHICH jobs a saved search is about.
// The digest matcher already reads a stored query for `q` and the filter only and
// orders by its own clock (internal/engage/notify/match.go), so two sets differing
// only by sort deliver identical mail — they are duplicates, and the key must say so.
describe('savedSearchQuery', () => {
  it('drops the ordering from the saved-search key', () => {
    expect(new URLSearchParams(savedSearchQuery(withQuery('go', 'views'))).get('sort')).toBeNull();
    expect(new URLSearchParams(savedSearchQuery(withQuery('go', 'newest'))).get('sort')).toBeNull();
    expect(new URLSearchParams(savedSearchQuery(withQuery('go', 'match'))).get('sort')).toBeNull();
  });

  it('keeps every filter that decides which jobs are in the set', () => {
    const q = savedSearchQuery({ ...withSkills({ include: ['go'] }), sort: 'views' });
    expect(new URLSearchParams(q).getAll('skills')).toEqual(['go']);
    expect(new URLSearchParams(q).get('sort')).toBeNull();
  });

  // The point of the fix: picking an ordering must not fork the saved search it came
  // from into a second set that mails the same jobs.
  it('makes two orderings of one filter set the same saved search', () => {
    expect(savedSearchQuery(withQuery('go', 'views'))).toBe(savedSearchQuery(withQuery('go', 'newest')));
    expect(canonicalQuery('q=go&sort=view_count')).toBe(canonicalQuery('q=go'));
  });
});

describe('activeFilterCount', () => {
  it('counts included and excluded values plus scalar filters', () => {
    const f = withSkills({ include: ['a', 'b'], exclude: ['c'] });
    f.visa = true;
    f.salaryMin = 100000;
    expect(activeFilterCount(f)).toBe(5); // 2 include + 1 exclude + visa + salary
  });
});

describe('sign transitions (pure)', () => {
  const inc = (): FacetState => ({ include: ['go'], exclude: [], matchAll: false });

  it('signOf reports the value state', () => {
    expect(signOf(emptyFacet(), 'go')).toBe('off');
    expect(signOf({ include: ['go'], exclude: [], matchAll: false }, 'go')).toBe('include');
    expect(signOf({ include: [], exclude: ['go'], matchAll: false }, 'go')).toBe('exclude');
  });

  it('facetSetSign moves a value between sets and does not mutate the input', () => {
    const st = inc();
    const next = facetSetSign(st, 'go', 'exclude');
    expect(next).toEqual({ include: [], exclude: ['go'], matchAll: false });
    expect(st).toEqual({ include: ['go'], exclude: [], matchAll: false }); // unchanged
    expect(facetSetSign(next, 'go', 'off')).toEqual({ include: [], exclude: [], matchAll: false });
  });

  it('facetCycle goes off -> include -> exclude -> off', () => {
    let st = emptyFacet();
    st = facetCycle(st, 'go');
    expect(signOf(st, 'go')).toBe('include');
    st = facetCycle(st, 'go');
    expect(signOf(st, 'go')).toBe('exclude');
    st = facetCycle(st, 'go');
    expect(signOf(st, 'go')).toBe('off');
  });

  it('facetPick adds to include, and removes when already selected in either set', () => {
    expect(signOf(facetPick(emptyFacet(), 'go'), 'go')).toBe('include');
    expect(signOf(facetPick({ include: ['go'], exclude: [], matchAll: false }, 'go'), 'go')).toBe('off');
    expect(signOf(facetPick({ include: [], exclude: ['go'], matchAll: false }, 'go'), 'go')).toBe('off');
  });

  it('facetToggleSign flips include and exclude', () => {
    expect(signOf(facetToggleSign({ include: ['go'], exclude: [], matchAll: false }, 'go'), 'go')).toBe('exclude');
    expect(signOf(facetToggleSign({ include: [], exclude: ['go'], matchAll: false }, 'go'), 'go')).toBe('include');
  });

  it('facetAdd adds to include, no-op on blank or existing value', () => {
    expect(facetAdd(emptyFacet(), '  go ').include).toEqual(['go']);
    expect(facetAdd(inc(), 'go')).toEqual(inc()); // duplicate
    expect(facetAdd(emptyFacet(), '   ')).toEqual(emptyFacet()); // blank
    expect(facetAdd({ include: [], exclude: ['go'], matchAll: false }, 'go').exclude).toEqual(['go']); // already excluded -> no-op
  });

  it('facetRemove clears the value from both sets', () => {
    expect(facetRemove({ include: ['go'], exclude: [], matchAll: false }, 'go')).toEqual(emptyFacet());
    expect(facetRemove({ include: [], exclude: ['go'], matchAll: false }, 'go')).toEqual(emptyFacet());
  });
});

// A title suggestion's click wraps `q` in Meilisearch's quoting syntax (see
// apiSuggestions.ts's quoteForTitleSearch) so the search engine matches it
// correctly — but that wrapper is a query-construction detail, not what the
// visitor typed, and must never reach a filter chip, the search box, or an
// analytics event as literal quote marks.
describe('displayQuery', () => {
  it('strips a matching pair of wrapping quotes', () => {
    expect(displayQuery('"Founding Engineer"')).toBe('Founding Engineer');
  });

  it('leaves unquoted text untouched', () => {
    expect(displayQuery('Founding Engineer')).toBe('Founding Engineer');
  });

  it('leaves a lone quote character untouched', () => {
    expect(displayQuery('"')).toBe('"');
  });

  it('leaves a quote at only one end untouched', () => {
    expect(displayQuery('"Founding Engineer')).toBe('"Founding Engineer');
    expect(displayQuery('Founding Engineer"')).toBe('Founding Engineer"');
  });

  it('leaves the empty string untouched', () => {
    expect(displayQuery('')).toBe('');
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

describe('formerEmployers ("Hiring again")', () => {
  it('round-trips as former_employers=true and is absent when off', () => {
    expect(filtersToParams(emptyFilters()).has('former_employers')).toBe(false);
    const p = filtersToParams({ ...emptyFilters(), formerEmployers: true });
    expect(p.get('former_employers')).toBe('true');
    expect(filtersFromParams(p).formerEmployers).toBe(true);
    expect(filtersFromParams(new URLSearchParams('former_employers=false')).formerEmployers).toBe(false);
  });

  it('counts as an active filter', () => {
    expect(activeFilterCount({ ...emptyFilters(), formerEmployers: true })).toBe(1);
  });

  // The digest matcher replays a saved query with nobody to resolve a bank for, so an
  // alert carrying it would mail the whole catalogue.
  it('never reaches a saved search', () => {
    const q = savedSearchQuery({ ...emptyFilters(), q: 'go', formerEmployers: true });
    expect(new URLSearchParams(q).has('former_employers')).toBe(false);
    expect(new URLSearchParams(q).get('q')).toBe('go');
  });
});

describe('watchlist', () => {
  it('round-trips as watchlist=true and is absent when off', () => {
    expect(filtersToParams(emptyFilters()).has('watchlist')).toBe(false);
    const p = filtersToParams({ ...emptyFilters(), watchlist: true });
    expect(p.get('watchlist')).toBe('true');
    expect(filtersFromParams(p).watchlist).toBe(true);
    expect(filtersFromParams(new URLSearchParams('watchlist=false')).watchlist).toBe(false);
  });

  it('counts as an active filter', () => {
    expect(activeFilterCount({ ...emptyFilters(), watchlist: true })).toBe(1);
  });

  // The saved search `watchlist=true` is the alert itself, so unlike the personal
  // former-employers filter it has to survive into the stored query.
  it('is kept in a saved search, beside the other filters', () => {
    const q = savedSearchQuery({ ...emptyFilters(), q: 'go', watchlist: true, formerEmployers: true });
    expect(new URLSearchParams(q).get('watchlist')).toBe('true');
    expect(new URLSearchParams(q).has('former_employers')).toBe(false);
    expect(canonicalQuery('watchlist=true&sort=newest')).toBe('watchlist=true');
  });

  it('lets an alert on the watchlist alone carry subscriptions', () => {
    expect(queryNarrows('watchlist=true')).toBe(true);
  });
});
