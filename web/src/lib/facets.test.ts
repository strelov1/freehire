import { describe, expect, it } from 'vitest';
import { COLLECTIONS, AI_ARCHETYPE_VALUES, ROLE_TYPE_VALUES } from './generated/contracts';
import {
  cityOption,
  collapseCities,
  countryFromSlug,
  countryLabel,
  countrySlug,
  dynamicLabel,
  FACETS,
  slugifiedCountries,
} from './facets';

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

describe('the country slug index', () => {
  it('slugifies the English name, not the ISO code — the code is not what anyone searches', () => {
    expect(countrySlug('de')).toBe('germany');
    expect(countrySlug('us')).toBe('united-states');
    expect(countrySlug('gb')).toBe('united-kingdom');
  });

  it('resolves a slug back to its ISO code', () => {
    expect(countryFromSlug('germany')).toBe('de');
    expect(countryFromSlug('united-states')).toBe('us');
  });

  it('strips diacritics rather than emitting them into a URL', () => {
    // 'Åland Islands' and 'Côte d’Ivoire' both carry marks Intl returns verbatim.
    expect(countrySlug('ax')).toBe('aland-islands');
    expect(countrySlug('ci')).toMatch(/^[a-z0-9-]+$/);
  });

  it('never publishes a slug that is merely an ISO code', () => {
    // countryLabel ECHOES the uppercased code when Intl has no name for it. A full-ICU
    // Node resolves every code in the list, so nothing echoes here — but a small-icu
    // build resolves few, and each miss would otherwise mint a URL out of a
    // two-letter non-word. The invariant holds either way, so it is what we assert
    // rather than the echo of any one code.
    const codes = new Set(slugifiedCountries().map((c) => c.code));
    for (const { slug } of slugifiedCountries()) {
      expect(codes.has(slug)).toBe(false);
    }
  });

  it('names no country Intl could not name', () => {
    for (const { code } of slugifiedCountries()) {
      expect(countryLabel(code)).not.toBe(code.toUpperCase());
    }
  });

  it('is injective — no two countries may claim one slug', () => {
    const slugs = slugifiedCountries().map((c) => c.slug);
    expect(new Set(slugs).size).toBe(slugs.length);
  });

  it('round-trips every country it publishes', () => {
    for (const { code, slug } of slugifiedCountries()) {
      expect(countryFromSlug(slug)).toBe(code);
      expect(countrySlug(code)).toBe(slug);
    }
  });

  it('emits URL-safe slugs only', () => {
    // Asserted by the property rather than by a shape regex: a slug is URL-safe
    // exactly when encoding it changes nothing, which is the thing that matters and
    // not a pattern that happens to describe today's names. (It also keeps a
    // character class out of this file — the design-system token check reads
    // `-[a-z0-9]` inside a regex as a Tailwind arbitrary value and fails the commit.)
    for (const { slug } of slugifiedCountries()) {
      expect(encodeURIComponent(slug)).toBe(slug);
      expect(slug).toBe(slug.toLowerCase());
      expect(slug.startsWith('-')).toBe(false);
      expect(slug.endsWith('-')).toBe(false);
      expect(slug).not.toContain('--');
    }
  });
});

describe('dynamicLabel', () => {
  // The skills facet has no static option list, so every surface that names a selected
  // skill — the filter summary chips above all — falls through to here. Without a
  // branch it printed the raw slug, so one skill was spelled two ways on one screen:
  // "ci-cd" in the summary, "CI/CD" in the panel beside it.
  it('labels a skill the way the rest of the product does', () => {
    expect(dynamicLabel('skills', 'ci-cd')).toBe('CI/CD');
    expect(dynamicLabel('skills', 'nodejs')).toBe('Node.js');
    expect(dynamicLabel('skills', 'data-engineering')).toBe('Data Engineering');
  });

  it('still passes through a facet with no label map of its own', () => {
    expect(dynamicLabel('some_other_facet', 'raw-value')).toBe('raw-value');
  });
});
