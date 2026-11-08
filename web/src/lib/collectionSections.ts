import type { CollectionKind, FilterCollectionGroup, SeeAlsoMark } from './collections';

// A section of the /collections hub: one card there, and its own page at
// /collections/group/:slug listing every collection in it. A filter collection
// names its section (`group`); a company collection's section follows from its
// `kind`, so a new tag lands in the right section without anyone listing it here.
//
// The page lives under `group/` rather than beside the collections themselves
// because /collections/:slug is already the collection namespace, and a section
// slug sharing it would one day shadow a collection of the same name.
export type CollectionSectionDef = {
  key: FilterCollectionGroup | CollectionKind;
  slug: string;
  title: string;
  description: string;
};

// One collection inside a section, normalized across both kinds of collection so
// the pages render them uniformly. `count` is the open-job count, or null when it
// could not be fetched — counts are decorative, so a failed fetch degrades to no count.
export type CollectionRow = {
  slug: string;
  title: string;
  description: string;
  count: number | null;
  mark: SeeAlsoMark;
  // Leading companies hiring in it, biggest first — drawn as an overlapping logo
  // stack in place of `mark`. Empty when the snapshot has none for it: `mark` is
  // the fallback.
  companies: string[];
  // Leading countries, for a regional remote collection — a flag stack, same rule.
  flags: string[];
};

// In display order.
export const COLLECTION_SECTIONS: CollectionSectionDef[] = [
  {
    key: 'remote',
    slug: 'remote',
    title: 'Remote',
    description: 'Fully remote roles, by the region or country they are open to.',
  },
  {
    key: 'city',
    slug: 'cities',
    title: 'Cities',
    description: 'Open tech roles in the cities with the most of them.',
  },
  {
    key: 'language',
    slug: 'languages',
    title: 'Languages',
    description: 'Open roles by the programming language they are written in.',
  },
  {
    key: 'framework',
    slug: 'frameworks',
    title: 'Frameworks',
    description: 'Open roles by the framework or runtime they build on.',
  },
  {
    key: 'infra',
    slug: 'cloud-infrastructure',
    title: 'Cloud & Infrastructure',
    description: 'Open roles by the cloud, database and tooling they run on.',
  },
  {
    key: 'field',
    slug: 'fields',
    title: 'Fields',
    description: 'Open roles by field — backend, DevOps, data, product, security and more.',
  },
  {
    key: 'ai',
    slug: 'ai-careers',
    title: 'AI careers',
    description: 'Open AI roles by the kind of AI work they actually are.',
  },
  {
    key: 'seniority',
    slug: 'seniority',
    title: 'Seniority',
    description: 'Open roles by level, from internships to principal engineers.',
  },
  {
    key: 'role',
    slug: 'roles',
    title: 'Roles',
    description: 'Open roles by job title, a field and a level together.',
  },
  {
    key: 'industry',
    slug: 'industries',
    title: 'Industries',
    description: 'Open tech roles by the market the employer is in.',
  },
  {
    key: 'job-type',
    slug: 'job-types',
    title: 'Job types',
    description: 'Contract, part-time and relocation roles.',
  },
  {
    key: 'company-type',
    slug: 'company-types',
    title: 'Company types',
    description: 'Open tech roles by the kind of company hiring — startups, product companies, agencies and more.',
  },
  {
    key: 'editorial',
    slug: 'company-lists',
    title: 'Company lists',
    description: 'Open roles at hand-picked groups of companies — Big Tech, unicorns, AI labs and more.',
  },
  {
    key: 'backer',
    slug: 'investors',
    title: 'Investors',
    description: 'Open roles at companies backed by a given accelerator or venture firm.',
  },
  {
    key: 'credential',
    slug: 'visa-sponsors',
    title: 'Visa sponsors',
    description: 'Open roles at employers on a public register of visa sponsors.',
  },
];

export function sectionBySlug(slug: string): CollectionSectionDef | undefined {
  return COLLECTION_SECTIONS.find((s) => s.slug === slug);
}
