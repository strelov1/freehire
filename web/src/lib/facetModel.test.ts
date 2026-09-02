import { describe, it, expect } from 'vitest';
import {
  emptyFacet,
  emptyFilters,
  filtersToParams,
  filtersFromParams,
  activeFilterCount,
  canonicalQuery,
  generalCountsCoverRole,
  savedSearchQuery,
  signOf,
  facetSetSign,
  facetCycle,
  facetPick,
  facetToggleSign,
  facetAdd,
  facetRemove,
  filtersWithRole,
  defaultSortFor,
  effectiveSort,
  sortOptionsFor,
  selectedSortFor,
  type FacetState,
  type JobFilters,
  type JobSort,
} from './facetModel';
import { must } from './utils';

// A JobFilters carrying a query and an EXPLICITLY chosen ordering — the pair the sort
// default depends on. Pass `null` for "the caller has not chosen one".
function withQuery(q: string, sort: JobSort | null): JobFilters {
  return { ...emptyFilters(), q, sort };
}

// A JobFilters seeded with one facet's state, for serialization tests.
function withSkills(st: Partial<FacetState>): JobFilters {
  const f = emptyFilters();
  f.facets.skills = { include: [], exclude: [], matchAll: false, ...st };
  return f;
}

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

  it('counts as one active filter, including at zero', () => {
    const f = emptyFilters();
    f.experienceYearsMax = 0;
    expect(activeFilterCount(f)).toBe(1);
  });
});

describe('role facet round-trips through the generic param path', () => {
  it('serializes include/exclude/mode and reads them back', () => {
    const f = emptyFilters();
    // role is a registered FACET, so emptyFilters seeds it.
    f.facets.role = { include: ['senior_backend', 'lead_frontend'], exclude: ['fractional_cto'], matchAll: true };

    const p = filtersToParams(f);
    expect(p.getAll('role')).toEqual(['senior_backend,lead_frontend']);
    expect(p.getAll('role_exclude')).toEqual(['fractional_cto']);
    expect(p.get('role_mode')).toBe('and');

    const back = filtersFromParams(p);
    const role = must(back.facets.role);
    expect(role.include).toEqual(['senior_backend', 'lead_frontend']);
    expect(role.exclude).toEqual(['fractional_cto']);
    expect(role.matchAll).toBe(true);
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

// The sort vocabulary is `relevance` / `newest` / `match`, and its DEFAULT depends on
// whether there is query text — mirroring the endpoint, which orders by relevance under
// a query and by posting date without one (internal/api/handler/search.go). Serializing
// the default therefore means writing nothing: the absence of `sort` is exactly how the
// backend already spells both defaults. Every unrecognised value — including the retired
// `sort=cv` — reads as that default rather than erroring, because shared links and saved
// searches still carry old ones.
describe('sort', () => {
  it('ignores a legacy sort=cv param — falls back to the default feed, not an error', () => {
    expect(filtersFromParams(new URLSearchParams('sort=cv'))).toEqual(filtersFromParams(new URLSearchParams('')));
  });

  // An unrecognised value is not a choice, so it parses to `null` — "unchosen" — and
  // resolves through the contextual default like any other link that names no ordering.
  it('reads an unknown sort value as no choice at all', () => {
    expect(filtersFromParams(new URLSearchParams('sort=bogus')).sort).toBeNull();
    expect(effectiveSort(filtersFromParams(new URLSearchParams('sort=bogus')))).toBe('newest');
    expect(effectiveSort(filtersFromParams(new URLSearchParams('q=go&sort=bogus')))).toBe('relevance');
  });

  it('defaults to newest while browsing and to relevance under a query', () => {
    expect(defaultSortFor('')).toBe('newest');
    expect(defaultSortFor('go')).toBe('relevance');
    expect(effectiveSort(filtersFromParams(new URLSearchParams('')))).toBe('newest');
    expect(effectiveSort(filtersFromParams(new URLSearchParams('q=go')))).toBe('relevance');
  });

  it('writes nothing for either contextual default', () => {
    expect(filtersToParams(emptyFilters()).get('sort')).toBeNull();
    expect(filtersToParams(withQuery('go', 'relevance')).get('sort')).toBeNull();
    expect(new URLSearchParams(savedSearchQuery(withSkills({ include: ['go'] }))).get('sort')).toBeNull();
  });

  // The bug this replaces: `newest` was the unconditional default, so it was never
  // serialized, so a text search carried no `sort` and the server ranked by relevance
  // while the control still read "Newest".
  it('sends newest explicitly once it stops being the default', () => {
    expect(filtersToParams(withQuery('go', 'newest')).get('sort')).toBe('posted_at');
    expect(filtersFromParams(new URLSearchParams('q=go&sort=posted_at')).sort).toBe('newest');
  });

  it('round-trips the match sort', () => {
    expect(filtersFromParams(new URLSearchParams('sort=match')).sort).toBe('match');
    expect(filtersToParams({ ...emptyFilters(), sort: 'match' }).get('sort')).toBe('match');
  });

  it('keeps a match sort alongside the other filters', () => {
    const f = filtersFromParams(new URLSearchParams('sort=match&countries=DE'));
    expect(f.sort).toBe('match');
    expect(must(f.facets.countries).include).toEqual(['DE']);
  });

  // A signed-out visitor opening a shared match link must keep the param: the server
  // degrades the ordering for them, and signing in should then just work.
  it('preserves the match sort through a params round trip', () => {
    const f = filtersFromParams(new URLSearchParams('sort=match'));
    expect(filtersToParams(f).get('sort')).toBe('match');
  });

  // Relevance has nothing to rank against once the query goes. The collapse is a pure
  // function rather than an effect so the control and the serializer read one rule.
  it('collapses relevance to newest when the query is empty', () => {
    expect(effectiveSort(withQuery('', 'relevance'))).toBe('newest');
    expect(effectiveSort(withQuery('go', 'relevance'))).toBe('relevance');
    expect(effectiveSort(withQuery('', 'match'))).toBe('match');
  });

  it('serializes a stranded relevance selection as the browse default', () => {
    expect(filtersToParams(withQuery('', 'relevance')).get('sort')).toBeNull();
  });

  // The bug the review caught: storing the RESOLVED default made "the browse feed
  // defaulted to newest" look identical to "the caller asked for newest", so typing
  // into the search box carried sort=posted_at into a text search and date-ordered it.
  // An unchosen ordering is null, and null follows the query.
  it('does not pin an unchosen ordering when a query is typed', () => {
    const browsing = filtersFromParams(new URLSearchParams(''));
    expect(browsing.sort).toBeNull();
    expect(effectiveSort(browsing)).toBe('newest');

    const searching = { ...browsing, q: 'golang' };
    expect(effectiveSort(searching)).toBe('relevance');
    expect(filtersToParams(searching).get('sort')).toBeNull();
  });

  // Why that matters beyond the ordering: savedSearchQuery is filtersToParams(...),
  // so a spurious sort param makes the live filters compare unequal to the saved
  // search they came from — which reads as "dirty" and creates a duplicate on save.
  it('keeps a typed query comparing equal to the saved search it came from', () => {
    const saved = savedSearchQuery(filtersFromParams(new URLSearchParams('q=go')));
    const typed = savedSearchQuery({ ...filtersFromParams(new URLSearchParams('')), q: 'go' });

    expect(typed).toBe(saved);
  });

  it('still sends an explicitly chosen newest under a query', () => {
    expect(filtersToParams(withQuery('go', 'newest')).get('sort')).toBe('posted_at');
  });
});

// The option list is the sort control's visibility rule, kept pure and out of the
// component so it can be tested at all — the same argument that put effectiveSort here.
describe('sort options', () => {
  it('offers relevance only under a query, and match only when it can be served', () => {
    expect(sortOptionsFor('', false).map((o) => o.value)).toEqual(['newest']);
    expect(sortOptionsFor('go', false).map((o) => o.value)).toEqual(['relevance', 'newest']);
    expect(sortOptionsFor('', true).map((o) => o.value)).toEqual(['newest', 'match']);
    expect(sortOptionsFor('go', true).map((o) => o.value)).toEqual(['relevance', 'newest', 'match']);
  });

  // A shared ?sort=match link opened signed out: the param survives (the server degrades
  // the ordering rather than refusing it), but the control cannot show an option it does
  // not offer. It shows what the server will actually serve — a select with nothing
  // selected would be a blank control over a real ordering.
  it('shows what the server will serve when the chosen ordering cannot be offered', () => {
    expect(selectedSortFor(withQuery('go', 'match'), false)).toBe('relevance');
    expect(selectedSortFor(withQuery('', 'match'), false)).toBe('newest');
    expect(selectedSortFor(withQuery('go', 'match'), true)).toBe('match');
  });

  it('always names an option that exists', () => {
    for (const q of ['', 'go']) {
      for (const matchAvailable of [false, true]) {
        const f = withQuery(q, 'match');
        const values = sortOptionsFor(q, matchAvailable).map((o) => o.value);
        expect(values).toContain(selectedSortFor(f, matchAvailable));
      }
    }
  });
});

// Choosing a role suggestion under the header search box replaces the typed text
// with the role facet. Both happen in ONE state change: applied separately, the
// search would briefly AND a half-typed query against the role and return fewer
// jobs than either filter alone — a suggestion that empties the page.
describe('filtersWithRole', () => {
  it('turns the role on and empties the text query', () => {
    const after = filtersWithRole({ ...emptyFilters(), q: 'data an' }, 'data_analytics');
    expect(after.q).toBe('');
    expect(must(after.facets.role).include).toEqual(['data_analytics']);
  });

  it('adds to the roles already chosen rather than replacing them', () => {
    const after = filtersWithRole(filtersWithRole(emptyFilters(), 'backend'), 'frontend');
    expect(must(after.facets.role).include).toEqual(['backend', 'frontend']);
  });

  it('switches a role from excluded to included', () => {
    // Suggestions are withheld for roles already INCLUDED, not for excluded ones, so
    // an excluded role is still offered. Adding it must flip the sign: a no-op would
    // clear the typed text and change nothing else, which reads as a broken click.
    const before = emptyFilters();
    before.facets.role = { include: [], exclude: ['data_analytics'], matchAll: false };
    const after = filtersWithRole(before, 'data_analytics');
    expect(must(after.facets.role).include).toEqual(['data_analytics']);
    expect(must(after.facets.role).exclude).toEqual([]);
  });

  it('leaves every other filter alone', () => {
    const before = withSkills({ include: ['go'] });
    before.postedWithinDays = 7;
    const after = filtersWithRole(before, 'backend');
    expect(must(after.facets.skills).include).toEqual(['go']);
    expect(after.postedWithinDays).toBe(7);
  });

  it('does not mutate the filters it was given', () => {
    const before = { ...emptyFilters(), q: 'data an' };
    filtersWithRole(before, 'data_analytics');
    expect(before.q).toBe('data an');
    expect(must(before.facets.role).include).toEqual([]);
  });
});

describe('generalCountsCoverRole', () => {
  it('covers role when the scope carries no text query', () => {
    expect(generalCountsCoverRole(new URLSearchParams('regions=latam,global'))).toBe(true);
  });

  it('covers role for the bare, unfiltered scope', () => {
    expect(generalCountsCoverRole(new URLSearchParams())).toBe(true);
  });

  it('does not cover role once a text query narrows the scope', () => {
    expect(generalCountsCoverRole(new URLSearchParams('regions=latam,global&q=python'))).toBe(
      false,
    );
  });

  it('treats an empty q the way filtersToParams does — as no query at all', () => {
    expect(generalCountsCoverRole(new URLSearchParams('q='))).toBe(true);
  });
});
