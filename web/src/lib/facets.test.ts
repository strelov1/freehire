import { describe, expect, it } from 'vitest';
import { COLLECTIONS, AI_ARCHETYPE_VALUES, ROLE_TYPE_VALUES } from './generated/contracts';
import { cityOption, collapseCities, countryLabel, FACETS } from './facets';

const collectionOptions = () => FACETS.find((f) => f.param === 'collections')?.options ?? [];

describe('the ai_archetype facet', () => {
  it('is registered right after category', () => {
    const categoryIndex = FACETS.findIndex((f) => f.param === 'category');
    const archetypeIndex = FACETS.findIndex((f) => f.param === 'ai_archetype');
    expect(archetypeIndex).toBe(categoryIndex + 1);
  });

  it('offers every generated archetype value as a static select option, with counts', () => {
    const facet = FACETS.find((f) => f.param === 'ai_archetype');
    expect(facet?.control).toBe('select');
    expect(facet?.dynamic).toBeFalsy();
    const offered = (facet?.options ?? []).map((o) => o.value).toSorted();
    expect(offered).toEqual(AI_ARCHETYPE_VALUES.toSorted());
  });
});

describe('the role_type facet', () => {
  it('offers every generated role-type value as an excludable pill', () => {
    const facet = FACETS.find((f) => f.param === 'role_type');
    expect(facet?.control).toBe('pills');
    expect(facet?.excludable).toBe(true);
    const offered = (facet?.options ?? []).map((o) => o.value).toSorted();
    expect(offered).toEqual(ROLE_TYPE_VALUES.toSorted());
  });

  // The vocabulary is one-sided: we can show a posting IS a management role, never
  // that it is not. Labelling the excluded state "individual contributor" would turn
  // a known unknown into a false claim, so no label here may say it.
  it('never labels any value as individual contributor', () => {
    const labels = (FACETS.find((f) => f.param === 'role_type')?.options ?? []).map((o) =>
      o.label.toLowerCase(),
    );
    for (const label of labels) {
      expect(label).not.toContain('individual contributor');
      expect(label).not.toMatch(/\bic\b/);
    }
  });
});

describe('the collections facet', () => {
  it('offers every collection in the registry as a filter option', () => {
    // The options were built from `kind === 'editorial'` plus `kind === 'credential'`.
    // The day a third kind appeared, its collections silently vanished from the
    // filters — filterable in the API, unreachable in the UI.
    const registry = COLLECTIONS.map((c) => c.slug).sort();
    const offered = collectionOptions()
      .map((o) => o.value)
      .sort();
    expect(offered).toEqual(registry);
  });

  it('offers the backer collections', () => {
    const offered = collectionOptions().map((o) => o.value);
    for (const slug of ['yc', 'techstars', 'a16z-portfolio', 'a16z-speedrun']) {
      expect(offered).toContain(slug);
    }
  });

  it('carries the brand mark on a backer collection, and none on the others', () => {
    // The chip is how most readers meet these collections; the orange Y is
    // recognised well before the words beside it are read.
    const byValue = new Map(collectionOptions().map((o) => [o.value, o]));
    expect(byValue.get('yc')?.icon).toMatch(/\.png$/);
    expect(byValue.get('a16z-speedrun')?.icon).toMatch(/\.png$/);
    expect(byValue.get('bigtech')?.icon).toBeUndefined();
    expect(byValue.get('uk-skilled-worker-sponsor')?.icon).toBeUndefined();
  });

  it('groups credentials under their own heading, and leaves the rest ungrouped', () => {
    // A licence from a public register is not one of our curated picks; running the
    // two together would read as though we vouched for both the same way.
    for (const option of collectionOptions()) {
      const kind = COLLECTIONS.find((c) => c.slug === option.value)?.kind;
      expect(option.group).toBe(kind === 'credential' ? 'Employer credentials' : undefined);
    }
  });
});

describe('cityOption', () => {
  it('composes a display label naming the country, keeping the bare city as the value', () => {
    // The backend sends a raw ISO code (no hand-maintained country-name table in
    // Go); the client already has an Intl.DisplayNames-backed resolver, so the
    // label is composed here, not by the server.
    const opt = cityOption({ value: 'Florianópolis', country: 'br' });
    expect(opt.value).toBe('Florianópolis');
    expect(opt.label).toBe('Florianópolis, Brazil');
  });

  it('delegates country display entirely to the shared countryLabel resolver', () => {
    // Same resolver COUNTRY_OPTIONS already uses (Intl.DisplayNames-backed) — this
    // just locks that cityOption doesn't hand-roll its own country formatting.
    const opt = cityOption({ value: 'Reykjavik', country: 'is' });
    expect(opt.label).toBe(`Reykjavik, ${countryLabel('is')}`);
  });
});

describe('collapseCities', () => {
  // A location preference stores the bare city name, so London/gb and London/ca are
  // the same stored value — two rows offering one outcome. Worse, the picker keys its
  // {#each} by value, and a duplicate key is a Svelte error that unmounts the entire
  // list, so typing "london" emptied the dropdown instead of showing a wrong entry.
  it('offers one option per city name, keeping the highest-population row', () => {
    const options = collapseCities([
      { value: 'London', country: 'gb' },
      { value: 'London', country: 'ca' },
      { value: 'Londonderry County Borough', country: 'gb' },
    ]);

    expect(options.map((o) => o.value)).toEqual(['London', 'Londonderry County Borough']);
    // /geo/cities ranks by descending population, so the first London is the label kept.
    expect(options[0]?.label).toBe(`London, ${countryLabel('gb')}`);
  });

  it('keeps distinct city names that merely share a prefix', () => {
    const options = collapseCities([
      { value: 'San Francisco', country: 'us' },
      { value: 'San Diego', country: 'us' },
    ]);

    expect(options).toHaveLength(2);
  });
});
