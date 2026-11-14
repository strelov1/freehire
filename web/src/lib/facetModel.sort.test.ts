import { describe, it, expect } from 'vitest';
import {
  emptyFilters,
  filtersToParams,
  filtersFromParams,
  activeFilterCount,
  savedSearchQuery,
  defaultSortFor,
  effectiveSort,
  matchSortNeedsSkills,
  sortOptionsFor,
  selectedSortFor,
} from './facetModel';
import { withQuery, withSkills } from './facetModelTestUtils';
import { must } from './utils';

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

  // Most viewed is never a contextual default, so it always serializes, and it round
  // trips through the wire name the endpoint accepts.
  it('round trips most-viewed through sort=view_count', () => {
    expect(filtersToParams(withQuery('', 'views')).get('sort')).toBe('view_count');
    expect(filtersToParams(withQuery('go', 'views')).get('sort')).toBe('view_count');
    expect(filtersFromParams(new URLSearchParams('sort=view_count')).sort).toBe('views');
  });

  // `relevance` collapses to `newest` when the query empties because it has nothing left
  // to rank against. `views` ranks by a stored figure, so it must survive that — the
  // ordering the caller chose is still perfectly servable.
  it('keeps most-viewed when the query is cleared', () => {
    expect(effectiveSort(withQuery('', 'views'))).toBe('views');
    expect(filtersToParams(withQuery('', 'views')).get('sort')).toBe('view_count');
  });
});

// The option list is the sort control's visibility rule, kept pure and out of the
// component so it can be tested at all — the same argument that put effectiveSort here.
describe('sort options', () => {
  // Match is offered to everyone now, including a visitor with no skills on file. It
  // used to be hidden from them, which answered "why can I not sort by fit?" with
  // nothing at all; the feed explains what the ordering needs instead (see
  // matchSortNeedsSkills).
  it('offers relevance only under a query, and match always', () => {
    expect(sortOptionsFor('').map((o) => o.value)).toEqual(['newest', 'views', 'match']);
    expect(sortOptionsFor('go').map((o) => o.value)).toEqual(['relevance', 'newest', 'views', 'match']);
  });

  // `views` ranks by a stored figure, so unlike `relevance` it needs no query text.
  it('offers most-viewed unconditionally', () => {
    for (const q of ['', 'go']) {
      expect(sortOptionsFor(q).map((o) => o.value)).toContain('views');
    }
  });

  // `relevance` is the one ordering that can still be asked for and not offered: it
  // ranks against query text, and a shared link can arrive without any. The control must
  // name what the server will actually serve — a select with nothing selected would be a
  // blank control over a real ordering.
  it('shows what the server will serve when the chosen ordering cannot be offered', () => {
    expect(selectedSortFor(withQuery('', 'relevance'))).toBe('newest');
    expect(selectedSortFor(withQuery('go', 'relevance'))).toBe('relevance');
    expect(selectedSortFor(withQuery('', 'match'))).toBe('match');
  });

  it('always names an option that exists', () => {
    for (const q of ['', 'go']) {
      for (const sort of ['match', 'relevance', 'views', 'newest'] as const) {
        const f = withQuery(q, sort);
        expect(sortOptionsFor(q).map((o) => o.value)).toContain(selectedSortFor(f));
      }
    }
  });
});

// Choosing a role suggestion under the header search box replaces the typed text
// with the role facet. Both happen in ONE state change: applied separately, the
// search would briefly AND a half-typed query against the role and return fewer
// jobs than either filter alone — a suggestion that empties the page.
describe('the two date bounds', () => {
  // They are two questions, not two spellings of one: `posted_within_days` bounds the
  // date the SOURCE states (which some boards restate every crawl) and
  // `open_within_days` bounds when we first saw the posting. A test that only checked
  // the round trip would pass with the two wired to the same field.
  it('serializes each bound under its own param', () => {
    const f = emptyFilters();
    f.openWithinDays = 30;
    f.postedWithinDays = 3;
    const p = filtersToParams(f);
    expect(p.get('open_within_days')).toBe('30');
    expect(p.get('posted_within_days')).toBe('3');
  });

  it('reads each bound back independently', () => {
    const f = filtersFromParams(new URLSearchParams('open_within_days=30&posted_within_days=3'));
    expect(f.openWithinDays).toBe(30);
    expect(f.postedWithinDays).toBe(3);
  });

  it('leaves the other bound null when only one is given', () => {
    expect(filtersFromParams(new URLSearchParams('open_within_days=7')).postedWithinDays).toBeNull();
    expect(filtersFromParams(new URLSearchParams('posted_within_days=7')).openWithinDays).toBeNull();
  });

  it('writes nothing for an unset open bound', () => {
    expect(filtersToParams(emptyFilters()).has('open_within_days')).toBe(false);
  });

  // The same guard the backend applies: a value that cannot be a positive day count
  // imposes no bound, rather than narrowing the list to nothing.
  it('reads a non-positive or non-integer open bound as no bound', () => {
    for (const raw of ['', '0', '-3', 'soon', '2.5', ' ']) {
      const f = filtersFromParams(new URLSearchParams(`open_within_days=${encodeURIComponent(raw)}`));
      expect(f.openWithinDays, `open_within_days=${JSON.stringify(raw)}`).toBeNull();
    }
  });

  // The two parsers must agree on what a day count LOOKS like, not just on what it
  // means. Go reads these params with strconv.Atoi, which rejects exponent, decimal and
  // hex forms; Number() accepts all three. A link carrying ?open_within_days=1e2 would
  // otherwise show an active "Last 100 days" chip over a list the server never bounded.
  it('rejects the forms strconv.Atoi rejects, so the two ends cannot disagree', () => {
    for (const raw of ['1e2', '1.0', '0x10', '7 ', ' 7', '007.0', 'Infinity']) {
      const p = new URLSearchParams();
      p.set('open_within_days', raw);
      p.set('posted_within_days', raw);
      const f = filtersFromParams(p);
      expect(f.openWithinDays, `open_within_days=${JSON.stringify(raw)}`).toBeNull();
      expect(f.postedWithinDays, `posted_within_days=${JSON.stringify(raw)}`).toBeNull();
    }
  });

  // ...while the forms Atoi DOES accept keep working: leading zeros and a leading `+`
  // both parse in Go, so rejecting them here would be the same divergence mirrored.
  it('keeps the integer forms strconv.Atoi accepts', () => {
    for (const [raw, want] of [
      ['007', 7],
      ['+7', 7],
      ['7', 7],
    ] as const) {
      const p = new URLSearchParams();
      p.set('open_within_days', raw);
      expect(filtersFromParams(p).openWithinDays, raw).toBe(want);
    }
  });

  // The Go side drops a bound past a century because the day-to-duration arithmetic
  // wraps above ~106,751 days (see maxWithinDays). The parser mirrors it so the chip
  // never claims a bound the server refused.
  it('drops a bound further back than the server will honour', () => {
    for (const raw of ['36501', '106752', '200000', '2147483647']) {
      const p = new URLSearchParams();
      p.set('open_within_days', raw);
      expect(filtersFromParams(p).openWithinDays, raw).toBeNull();
    }
    const at = new URLSearchParams();
    at.set('open_within_days', '36500');
    expect(filtersFromParams(at).openWithinDays).toBe(36500);
  });

  it('counts each bound toward the filter badge', () => {
    const none = activeFilterCount(emptyFilters());
    const f = emptyFilters();
    f.openWithinDays = 30;
    expect(activeFilterCount(f)).toBe(none + 1);
    f.postedWithinDays = 3;
    expect(activeFilterCount(f)).toBe(none + 2);
  });
});

// The feed's answer to "I picked Match and it did not rank anything". The ordering needs
// the viewer's own skills; without them the server degrades to newest and, told nothing,
// the visitor reads that as the sort being broken.
describe('matchSortNeedsSkills', () => {
  it('asks for skills only when match is the ordering in force', () => {
    expect(matchSortNeedsSkills(withQuery('', 'match'), false)).toBe(true);
    expect(matchSortNeedsSkills(withQuery('', 'newest'), false)).toBe(false);
    expect(matchSortNeedsSkills(withQuery('', 'views'), false)).toBe(false);
  });

  it('stays quiet once the viewer has skills to rank against', () => {
    expect(matchSortNeedsSkills(withQuery('', 'match'), true)).toBe(false);
  });

  // A query plus ?sort=match still means match: `relevance` is only the DEFAULT under a
  // query, not an override of a stated one.
  it('asks under a query too', () => {
    expect(matchSortNeedsSkills(withQuery('go', 'match'), false)).toBe(true);
  });

  // Nobody asked for match here, so there is nothing to explain.
  it('says nothing when no ordering was stated', () => {
    expect(matchSortNeedsSkills(emptyFilters(), false)).toBe(false);
  });
});
