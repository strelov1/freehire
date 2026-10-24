import { describe, expect, it } from 'vitest';
import type { FacetDef } from './facets';
import { unsummarized } from './filterSummary';

const def = (param: string, label = param) => ({ param, label }) as FacetDef;

describe('unsummarized', () => {
  // The sidebar names its groups by hand, and twice a facet was simply left off the
  // list — `source` and `reality` narrowed the feed with no chip to remove them. Every
  // facet the hand list does not place falls through to a group of its own.
  it('returns every facet no group has shown, in declaration order', () => {
    const facets = [def('category'), def('source', 'Source'), def('reality', 'Posting reality')];
    expect(unsummarized(facets, new Set(['category']), []).map((d) => d.label)).toEqual([
      'Source',
      'Posting reality',
    ]);
  });

  it('leaves out the facets the host page excludes', () => {
    expect(unsummarized([def('source')], new Set(), ['source'])).toEqual([]);
  });
});
