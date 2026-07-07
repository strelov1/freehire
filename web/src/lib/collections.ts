// Curated collections shown on /collections. This mirrors the Go registry in
// internal/collections (the source of truth for membership + the search facet).
// It is a hand-kept mirror for now — only the display copy lives here; if the set
// grows, fold it into the generated contracts (gen-contracts) so the two can't
// drift. Keep `slug` identical to the Go registry slugs and the `collections`
// search-facet values.
export type Collection = {
  slug: string;
  title: string;
  description: string;
};

// A filter collection is the second kind of collection: a curated card that maps
// to an arbitrary /jobs facet filter rather than company membership. Unlike
// COLLECTIONS it is frontend-only — no Go registry, no `collections` search-facet
// value, no company/job membership. Adding one is a single entry below.
export type FilterCollection = {
  slug: string;
  title: string;
  description: string;
  // Job-search facet params this collection maps to — the same param names the
  // /jobs feed accepts (see search.StringFacets). A value may be a single string
  // or a list; a list expands into repeated query keys (OR semantics), matching
  // the /jobs filter contract.
  params: Record<string, string | string[]>;
};

export const FILTER_COLLECTIONS: FilterCollection[] = [
  {
    slug: 'remote-worldwide',
    title: 'Remote Worldwide',
    description:
      'Fully remote roles open to candidates anywhere in the world, not tied to a country or region.',
    params: { work_mode: 'remote', regions: 'global' },
  },
  // Regional remote landings. Params use the canonical facet vocabulary: regions
  // from REGION_LABELS (there is no `us` region — the US is country-level), and
  // countries as ISO 3166-1 alpha-2. Each was confirmed to have a healthy,
  // non-empty live count before shipping.
  {
    slug: 'remote-latam',
    title: 'Remote Latam',
    description: 'Fully remote roles open to candidates across Latin America.',
    params: { work_mode: 'remote', regions: 'latam' },
  },
  {
    slug: 'remote-brasil',
    title: 'Remote Brasil',
    description: 'Fully remote roles open to candidates in Brazil.',
    params: { work_mode: 'remote', countries: 'br' },
  },
  {
    slug: 'remote-us',
    title: 'Remote US',
    description: 'Fully remote roles open to candidates in the United States.',
    params: { work_mode: 'remote', countries: 'us' },
  },
  {
    slug: 'remote-europe',
    title: 'Remote Europe',
    description: 'Fully remote roles open to candidates across Europe.',
    params: { work_mode: 'remote', regions: 'eu' },
  },
  {
    slug: 'remote-apac',
    title: 'Remote APAC',
    description: 'Fully remote roles open to candidates across Asia-Pacific.',
    params: { work_mode: 'remote', regions: 'apac' },
  },
  // Language & framework landings — the classic "<lang> jobs" search pattern, one
  // per canonical `skills` facet value. `slug`/`params.skills` MUST be the exact
  // skilltag canonical (e.g. `go` not `golang`, `nodejs` not `node`, `cpp`/`csharp`
  // not `c++`/`c#`) or the feed comes back empty. Each was confirmed to have a live
  // count before shipping; the few low-count ones (clojure/elixir/svelte) are kept
  // deliberately — low-competition "<lang> jobs" terms with hundreds of real roles.
  {
    slug: 'python',
    title: 'Python',
    description: 'Open roles that use Python — backend, data, ML and automation.',
    params: { skills: 'python' },
  },
  {
    slug: 'javascript',
    title: 'JavaScript',
    description: 'Open roles that use JavaScript across web and backend.',
    params: { skills: 'javascript' },
  },
  {
    slug: 'typescript',
    title: 'TypeScript',
    description: 'Open roles that use TypeScript for typed JavaScript at scale.',
    params: { skills: 'typescript' },
  },
  {
    slug: 'java',
    title: 'Java',
    description: 'Open roles that use Java — enterprise backends, Android and big data.',
    params: { skills: 'java' },
  },
  {
    slug: 'csharp',
    title: 'C#',
    description: 'Open roles that use C# and the .NET ecosystem.',
    params: { skills: 'csharp' },
  },
  {
    slug: 'cpp',
    title: 'C++',
    description: 'Open roles that use C++ — systems, games and performance-critical code.',
    params: { skills: 'cpp' },
  },
  {
    slug: 'go',
    title: 'Go',
    description: 'Open roles that use Go for backends, infra and cloud-native services.',
    params: { skills: 'go' },
  },
  {
    slug: 'rust',
    title: 'Rust',
    description: 'Open roles that use Rust for safe, high-performance systems.',
    params: { skills: 'rust' },
  },
  {
    slug: 'ruby',
    title: 'Ruby',
    description: 'Open roles that use Ruby, from web apps to tooling.',
    params: { skills: 'ruby' },
  },
  {
    slug: 'php',
    title: 'PHP',
    description: 'Open roles that use PHP for web backends and platforms.',
    params: { skills: 'php' },
  },
  {
    slug: 'kotlin',
    title: 'Kotlin',
    description: 'Open roles that use Kotlin for Android and JVM backends.',
    params: { skills: 'kotlin' },
  },
  {
    slug: 'swift',
    title: 'Swift',
    description: 'Open roles that use Swift for iOS, macOS and Apple platforms.',
    params: { skills: 'swift' },
  },
  {
    slug: 'scala',
    title: 'Scala',
    description: 'Open roles that use Scala for JVM backends and data engineering.',
    params: { skills: 'scala' },
  },
  {
    slug: 'nodejs',
    title: 'Node.js',
    description: 'Open roles that use Node.js for JavaScript backends and APIs.',
    params: { skills: 'nodejs' },
  },
  {
    slug: 'clojure',
    title: 'Clojure',
    description: 'Open roles that use Clojure and functional JVM development.',
    params: { skills: 'clojure' },
  },
  {
    slug: 'elixir',
    title: 'Elixir',
    description: 'Open roles that use Elixir and the BEAM for scalable backends.',
    params: { skills: 'elixir' },
  },
  {
    slug: 'react',
    title: 'React',
    description: 'Open roles that use React to build web interfaces.',
    params: { skills: 'react' },
  },
  {
    slug: 'angular',
    title: 'Angular',
    description: 'Open roles that use Angular for web applications.',
    params: { skills: 'angular' },
  },
  {
    slug: 'vue',
    title: 'Vue',
    description: 'Open roles that use Vue.js for web interfaces.',
    params: { skills: 'vue' },
  },
  {
    slug: 'nextjs',
    title: 'Next.js',
    description: 'Open roles that use Next.js for full-stack React apps.',
    params: { skills: 'nextjs' },
  },
  {
    slug: 'spring',
    title: 'Spring',
    description: 'Open roles that use Spring for Java backends.',
    params: { skills: 'spring' },
  },
  {
    slug: 'rails',
    title: 'Rails',
    description: 'Open roles that use Ruby on Rails for web applications.',
    params: { skills: 'rails' },
  },
  {
    slug: 'django',
    title: 'Django',
    description: 'Open roles that use Django for Python web backends.',
    params: { skills: 'django' },
  },
  {
    slug: 'svelte',
    title: 'Svelte',
    description: 'Open roles that use Svelte and SvelteKit for web interfaces.',
    params: { skills: 'svelte' },
  },
];

// toQuery expands a filter collection's params into a URL query string, repeating a
// key once per value for list params (OR semantics). It is the single source for
// both a card's link (`/jobs?<query>`) and its open-job count request, so the two
// can never disagree.
export function toQuery(params: Record<string, string | string[]>): string {
  const q = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    for (const v of Array.isArray(value) ? value : [value]) {
      q.append(key, v);
    }
  }
  return q.toString();
}

export const COLLECTIONS: Collection[] = [
  {
    slug: 'yc',
    title: 'Y Combinator',
    description:
      'Open roles at Y Combinator–backed companies, from current batches to graduated unicorns.',
  },
  {
    slug: 'techstars',
    title: 'Techstars',
    description: 'Open roles at Techstars-backed companies.',
  },
  {
    slug: 'european',
    title: 'European Startups',
    description: "Open roles at European startups across the continent's tech hubs.",
  },
  {
    slug: 'ai',
    title: 'AI Companies',
    description:
      'Open roles at AI-native companies — foundation-model labs, ML platforms and applied-AI products.',
  },
  {
    slug: 'mag7',
    title: 'Magnificent Seven',
    description:
      'Open roles at the Magnificent Seven — Apple, Microsoft, Alphabet, Amazon, Meta, Nvidia and Tesla.',
  },
  {
    slug: 'bigtech',
    title: 'Big Tech',
    description: 'Open roles at the largest, most established technology companies.',
  },
  {
    slug: 'unicorn',
    title: 'Unicorns',
    description: 'Open roles at unicorns — private companies valued at over $1 billion.',
  },
  {
    slug: 'fortune500',
    title: 'Fortune 500',
    description: 'Open roles at Fortune 500 companies — the largest US corporations by revenue.',
  },
  {
    slug: 'eastern-roots',
    title: 'Eastern Roots',
    description:
      'Open roles at globally distributed companies founded by Eastern European (incl. Russian-speaking) founders or with Eastern European engineering roots.',
  },
];

// A resolved collection: the display copy plus the fixed job-search facet params
// that scope its feed. `params` is single-valued (the shape JobsView's `scope`
// pins) — every current collection maps to single values; a multi-value filter
// collection would need `scope` widened first (see design's array seam).
export type ResolvedCollection = {
  title: string;
  description: string;
  params: Record<string, string>;
};

// Flatten a filter collection's params to the single-valued scope shape. A
// single-element list collapses to its element; a genuine multi-value param is
// unsupported by `scope` today and takes its first value (no such data exists).
function scopeParams(params: Record<string, string | string[]>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(params)) {
    if (Array.isArray(value)) {
      const [first] = value;
      if (first !== undefined) out[key] = first;
    } else {
      out[key] = value;
    }
  }
  return out;
}

// Resolve a slug to its collection, checking filter collections first, then
// company-membership collections (which map to the `collections` facet). Returns
// undefined for an unknown slug. The single source used by the /collections/:slug
// landing route, the hub card links, and the sitemap, so they cannot drift.
export function collectionBySlug(slug: string): ResolvedCollection | undefined {
  const filter = FILTER_COLLECTIONS.find((c) => c.slug === slug);
  if (filter) {
    return { title: filter.title, description: filter.description, params: scopeParams(filter.params) };
  }
  const company = COLLECTIONS.find((c) => c.slug === slug);
  if (company) {
    return { title: company.title, description: company.description, params: { collections: company.slug } };
  }
  return undefined;
}

// Every collection slug across both registries — the sitemap's source for the
// collection landing URLs. Slugs are unique across the two sets.
export function collectionSlugs(): string[] {
  return [...FILTER_COLLECTIONS.map((c) => c.slug), ...COLLECTIONS.map((c) => c.slug)];
}
