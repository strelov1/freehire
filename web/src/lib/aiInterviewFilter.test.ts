import { describe, it, expect } from 'vitest';
import { emptyFilters, filtersToParams, filtersFromParams, activeFilterCount } from './facetModel';

// One boolean, not the three states `clearance` carries: that facet answers two
// different people, while nobody searches FOR an AI screen. The only useful question
// is "not these".
//
// The param names the facet's VALUE (`ai_interview=false` means "postings whose company
// is not reported"), while the control names the user's INTENT ("hide them"), so the two
// read inverted — asserted in both directions here for the same reason the clearance
// mapping is.
describe('AI-interview filter', () => {
  it('defaults to off, writing no param', () => {
    const f = emptyFilters();
    expect(f.hideAIInterview).toBe(false);
    expect(filtersToParams(f).has('ai_interview')).toBe(false);
  });

  // Not `ai_interview=true` inverted at the edge, and not an equality on false at the
  // index: nothing there is ever written false, so the negative has to be asked as the
  // negation of the positive. The param value is what the backend turns into that.
  it('hiding serializes to ai_interview=false', () => {
    const f = emptyFilters();
    f.hideAIInterview = true;
    expect(filtersToParams(f).get('ai_interview')).toBe('false');
  });

  it('round-trips through the URL', () => {
    const f = emptyFilters();
    f.hideAIInterview = true;
    const back = filtersFromParams(filtersToParams(f));
    expect(back.hideAIInterview).toBe(true);
  });

  // A shared link with an unrecognised value must leave the control off rather than in
  // a state it cannot render — the same rule the clearance parse follows.
  it('reads an unrecognised value as off', () => {
    const back = filtersFromParams(new URLSearchParams('ai_interview=maybe'));
    expect(back.hideAIInterview).toBe(false);
  });

  // A filter the mobile badge does not count is a filter a person cannot tell is on.
  it('counts toward the active filter badge', () => {
    const f = emptyFilters();
    const before = activeFilterCount(f);
    f.hideAIInterview = true;
    expect(activeFilterCount(f)).toBe(before + 1);
  });
});
