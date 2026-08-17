import type { FamilyIconName } from './familymarks';
import type { Job } from './types';

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
  // Single-country remote landings — the "remote jobs in <country>" pattern,
  // alongside the regional ones above. Countries are ISO 3166-1 alpha-2. Each
  // was confirmed to have a healthy live count (thousands) before shipping.
  {
    slug: 'remote-canada',
    title: 'Remote Canada',
    description: 'Fully remote roles open to candidates in Canada.',
    params: { work_mode: 'remote', countries: 'ca' },
  },
  {
    slug: 'remote-uk',
    title: 'Remote UK',
    description: 'Fully remote roles open to candidates in the United Kingdom.',
    params: { work_mode: 'remote', countries: 'gb' },
  },
  {
    slug: 'remote-india',
    title: 'Remote India',
    description: 'Fully remote roles open to candidates in India.',
    params: { work_mode: 'remote', countries: 'in' },
  },
  {
    slug: 'remote-poland',
    title: 'Remote Poland',
    description: 'Fully remote roles open to candidates in Poland.',
    params: { work_mode: 'remote', countries: 'pl' },
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
  // Tech-category landings — the "<category> jobs" search pattern, one per canonical
  // `category` facet value (see vocab.CategoryValues). The param is `category`; the
  // slug is a readable form of the value — usually its kebab case (data_engineering →
  // data-engineering), but chosen for readability where they differ (ml_ai →
  // machine-learning). Only technical categories are listed — non-tech ones
  // (sales/management/support/marketing) are off-audience. Each was confirmed to have
  // a healthy live count (≥ 300) before shipping.
  {
    slug: 'backend',
    title: 'Backend',
    description: 'Server-side engineering roles — APIs, services, databases and backend systems.',
    params: { category: 'backend' },
  },
  {
    slug: 'frontend',
    title: 'Frontend',
    description: 'Frontend engineering roles building web user interfaces and client-side apps.',
    params: { category: 'frontend' },
  },
  {
    slug: 'fullstack',
    title: 'Full-Stack',
    description: 'Full-stack roles spanning both frontend and backend development.',
    params: { category: 'fullstack' },
  },
  {
    slug: 'devops',
    title: 'DevOps',
    description: 'DevOps roles automating build, deployment and cloud infrastructure.',
    params: { category: 'devops' },
  },
  {
    slug: 'sre',
    title: 'SRE',
    description: 'Site reliability roles keeping production systems scalable and resilient.',
    params: { category: 'sre' },
  },
  {
    slug: 'data-engineering',
    title: 'Data Engineering',
    description: 'Data engineering roles building pipelines, warehouses and data platforms.',
    params: { category: 'data_engineering' },
  },
  {
    slug: 'data-science',
    title: 'Data Science',
    description: 'Data science roles turning data into models, insight and decisions.',
    params: { category: 'data_science' },
  },
  {
    slug: 'machine-learning',
    title: 'Machine Learning',
    description: 'Machine learning roles training and shipping ML models to production.',
    params: { category: 'ml_ai' },
  },
  {
    slug: 'ai-engineering',
    title: 'AI Engineering',
    description: 'AI engineering roles building LLM and generative-AI powered products.',
    params: { category: 'ai_engineering' },
  },
  {
    slug: 'mobile',
    title: 'Mobile',
    description: 'Mobile engineering roles for iOS, Android and cross-platform apps.',
    params: { category: 'mobile' },
  },
  {
    slug: 'security',
    title: 'Security',
    description: 'Security roles covering application, cloud and infrastructure security.',
    params: { category: 'security' },
  },
  {
    slug: 'qa',
    title: 'QA',
    description: 'Quality assurance and test engineering roles across manual and automated testing.',
    params: { category: 'qa' },
  },
  {
    slug: 'architecture',
    title: 'Architecture',
    description: 'Software and solution architecture roles designing systems at scale.',
    params: { category: 'architecture' },
  },
  {
    slug: 'embedded',
    title: 'Embedded',
    description: 'Embedded and firmware roles programming devices and low-level systems.',
    params: { category: 'embedded' },
  },
  {
    slug: 'network-engineering',
    title: 'Network Engineering',
    description: 'Network engineering roles designing and operating network infrastructure.',
    params: { category: 'network_engineering' },
  },
  // Seniority landings — the "<level> jobs" pattern, one per canonical `seniority`
  // facet value. Copy leans on the level ("Senior-Level"), not a category it cannot
  // claim. Generic intent, but each has a large live count.
  {
    slug: 'junior',
    title: 'Junior-Level',
    description: 'Entry-level and junior engineering roles for early-career developers.',
    params: { seniority: 'junior' },
  },
  {
    slug: 'mid-level',
    title: 'Mid-Level',
    description: 'Mid-level engineering roles for developers with a few years of experience.',
    params: { seniority: 'middle' },
  },
  {
    slug: 'senior',
    title: 'Senior-Level',
    description: 'Senior engineering roles for experienced developers who lead delivery.',
    params: { seniority: 'senior' },
  },
  {
    slug: 'lead',
    title: 'Lead',
    description: 'Lead engineering roles owning technical direction and delivery for a team.',
    params: { seniority: 'lead' },
  },
  {
    slug: 'staff',
    title: 'Staff',
    description: 'Staff engineering roles driving technical strategy across teams.',
    params: { seniority: 'staff' },
  },
  {
    slug: 'principal',
    title: 'Principal',
    description: 'Principal engineering roles setting technical direction org-wide.',
    params: { seniority: 'principal' },
  },
  {
    slug: 'internship',
    title: 'Internship',
    description: 'Internship and trainee roles for students and new graduates in tech.',
    params: { seniority: 'intern' },
  },
  // Infra & ecosystem skill landings — the same "<skill> jobs" pattern as the
  // language/framework set above, extended to any other skilltag canonical:
  // cloud/platform tools, databases, CI/CD, ML frameworks, blockchain.
  // `slug`/`params.skills` MUST be the exact skilltag canonical. Each was
  // confirmed to have a live count before shipping.
  {
    slug: 'aws',
    title: 'AWS',
    description: 'Open roles that use Amazon Web Services for cloud infrastructure.',
    params: { skills: 'aws' },
  },
  {
    slug: 'kubernetes',
    title: 'Kubernetes',
    description: 'Open roles that use Kubernetes to orchestrate containerized workloads.',
    params: { skills: 'kubernetes' },
  },
  {
    slug: 'terraform',
    title: 'Terraform',
    description: 'Open roles that use Terraform for infrastructure as code.',
    params: { skills: 'terraform' },
  },
  {
    slug: 'docker',
    title: 'Docker',
    description: 'Open roles that use Docker to build and run containerized apps.',
    params: { skills: 'docker' },
  },
  {
    slug: 'postgresql',
    title: 'PostgreSQL',
    description: 'Open roles that use PostgreSQL as the primary relational database.',
    params: { skills: 'postgresql' },
  },
  {
    slug: 'redis',
    title: 'Redis',
    description: 'Open roles that use Redis for caching, queues and real-time data.',
    params: { skills: 'redis' },
  },
  {
    slug: 'kafka',
    title: 'Kafka',
    description: 'Open roles that use Apache Kafka for event streaming and messaging.',
    params: { skills: 'kafka' },
  },
  {
    slug: 'graphql',
    title: 'GraphQL',
    description: 'Open roles that use GraphQL for API design and data fetching.',
    params: { skills: 'graphql' },
  },
  {
    slug: 'mongodb',
    title: 'MongoDB',
    description: 'Open roles that use MongoDB as the primary document database.',
    params: { skills: 'mongodb' },
  },
  {
    slug: 'mysql',
    title: 'MySQL',
    description: 'Open roles that use MySQL as the primary relational database.',
    params: { skills: 'mysql' },
  },
  {
    slug: 'elasticsearch',
    title: 'Elasticsearch',
    description: 'Open roles that use Elasticsearch for search and log analytics.',
    params: { skills: 'elasticsearch' },
  },
  {
    slug: 'dotnet',
    title: '.NET',
    description: 'Open roles that use .NET for backend and enterprise applications.',
    params: { skills: 'dotnet' },
  },
  {
    slug: 'laravel',
    title: 'Laravel',
    description: 'Open roles that use Laravel for PHP web backends.',
    params: { skills: 'laravel' },
  },
  {
    slug: 'flutter',
    title: 'Flutter',
    description: 'Open roles that use Flutter for cross-platform mobile apps.',
    params: { skills: 'flutter' },
  },
  {
    slug: 'azure',
    title: 'Azure',
    description: 'Open roles that use Microsoft Azure for cloud infrastructure.',
    params: { skills: 'azure' },
  },
  {
    slug: 'gcp',
    title: 'Google Cloud',
    description: 'Open roles that use Google Cloud Platform for cloud infrastructure.',
    params: { skills: 'gcp' },
  },
  {
    slug: 'jenkins',
    title: 'Jenkins',
    description: 'Open roles that use Jenkins for CI/CD automation.',
    params: { skills: 'jenkins' },
  },
  {
    slug: 'ansible',
    title: 'Ansible',
    description: 'Open roles that use Ansible for infrastructure automation.',
    params: { skills: 'ansible' },
  },
  {
    slug: 'pytorch',
    title: 'PyTorch',
    description: 'Open roles that use PyTorch for machine learning.',
    params: { skills: 'pytorch' },
  },
  {
    slug: 'tensorflow',
    title: 'TensorFlow',
    description: 'Open roles that use TensorFlow for machine learning.',
    params: { skills: 'tensorflow' },
  },
  {
    slug: 'solidity',
    title: 'Solidity',
    description: 'Open roles that use Solidity for smart-contract and blockchain development.',
    params: { skills: 'solidity' },
  },
  // Named-role landings — the `role` facet (roletag: named roles + skill×seniority
  // combos). Only clearly-technical roles with an individually-verified live count
  // are listed; the facet also carries non-tech and seniority-only values, which are
  // covered by the category/seniority axes instead.
  {
    slug: 'software-engineer',
    title: 'Software Engineer',
    description: 'General software engineering roles across the stack and domains.',
    params: { role: 'software_engineer' },
  },
  {
    slug: 'senior-backend',
    title: 'Senior Backend',
    description: 'Senior backend engineering roles owning server-side systems and APIs.',
    params: { role: 'senior_backend' },
  },
  {
    slug: 'senior-frontend',
    title: 'Senior Frontend',
    description: 'Senior frontend engineering roles owning web UI architecture and delivery.',
    params: { role: 'senior_frontend' },
  },
  {
    slug: 'founding-engineer',
    title: 'Founding Engineer',
    description: 'Founding engineer roles building the first product at early-stage startups.',
    params: { role: 'founding_engineer' },
  },
  {
    slug: 'senior-devops',
    title: 'Senior DevOps',
    description: 'Senior DevOps engineering roles owning build, deployment and infrastructure.',
    params: { role: 'senior_devops' },
  },
  {
    slug: 'senior-fullstack',
    title: 'Senior Full-Stack',
    description: 'Senior full-stack engineering roles spanning frontend and backend delivery.',
    params: { role: 'senior_fullstack' },
  },
  {
    slug: 'senior-data-engineering',
    title: 'Senior Data Engineer',
    description: 'Senior data engineering roles building pipelines and data platforms.',
    params: { role: 'senior_data_engineering' },
  },
  {
    slug: 'staff-engineer',
    title: 'Staff Engineer',
    description: 'Staff engineering roles driving technical strategy for a team or product area.',
    params: { role: 'staff_engineer' },
  },
  {
    slug: 'engineering-manager',
    title: 'Engineering Manager',
    description: 'Engineering management roles leading and growing a team of engineers.',
    params: { role: 'engineering_manager' },
  },
];

// toQuery expands a filter collection's params into a URL query string, repeating a
// key once per value for list params (OR semantics). It is the single source for
// both the feed filter URL (`/?<query>`) and its open-job count request, so the two
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

// The company-tag registry is generated from the Go source of truth
// (internal/collections) by cmd/gen-contracts and re-exported here so every import
// site keeps its `$lib/collections` path. It carries `kind`, which decides whether
// a tag renders as an editorial collection or as a credential — a field a hand-kept
// mirror could get wrong silently, which is why this stopped being hand-kept.
export { COLLECTIONS } from './generated/contracts';
import { COLLECTIONS as COMPANY_COLLECTIONS } from './generated/contracts';
export type { Collection, CollectionKind } from './generated/contracts';

// A resolved collection: the display copy plus the fixed job-search facet params
// that scope its feed. `params` is single-valued (the shape JobsView's `scope`
// pins) — every current collection maps to single values; a multi-value filter
// collection would need `scope` widened first (see design's array seam).
export type ResolvedCollection = {
  title: string;
  description: string;
  params: Record<string, string>;
};

// One card in a "see also" style block (JobSeeAlso), mirroring the /collections
// hub's CollectionCard: count is the collection's live open-job total, or null
// when it couldn't be fetched — decorative, so a failure degrades to no count
// rather than breaking the block. mark is always present — resolved by
// resolveSeeAlsoMark (seeAlsoMark.ts) to whichever is most specific for the
// card's underlying collection: a backer's brand image (Y Combinator,
// Techstars, …), a technology's brand logo, a country's flag, or a
// color-coded family icon. The type lives here (not in a route file) so both
// the server load that computes it and the component that renders it import
// from one place.
export type SeeAlsoMark =
  | { kind: 'image'; src: string }
  | { kind: 'logo'; title: string; path: string; hex: string }
  | { kind: 'flag'; countryCode: string }
  | { kind: 'family'; icon: FamilyIconName; color: string };

export type SeeAlsoCard = {
  slug: string;
  title: string;
  count: number | null;
  mark: SeeAlsoMark;
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
  const company = COMPANY_COLLECTIONS.find((c) => c.slug === slug);
  if (company) {
    return { title: company.title, description: company.description, params: { collections: company.slug } };
  }
  return undefined;
}

// Every collection slug across both registries — the sitemap's source for the
// collection landing URLs. Slugs are unique across the two sets.
export function collectionSlugs(): string[] {
  return [...FILTER_COLLECTIONS.map((c) => c.slug), ...COMPANY_COLLECTIONS.map((c) => c.slug)];
}

// The facets of a viewed job that relatedCollectionSlugs matches against —
// deliberately only what the Job wire object actually carries (see
// web/src/lib/generated/contracts.ts): `category`/`seniority` come from
// `job.enrichment.category`/`job.enrichment.seniority` (not top-level fields),
// the rest map straight to `job.skills`/`work_mode`/`countries`/`regions`.
// There is no `role` here: roletag's seniority×category/named-role
// composition lives server-side only (internal/roletag), and duplicating it
// client-side would break the dict-only single-source-of-truth convention. A
// `role`-keyed FILTER_COLLECTIONS entry (e.g. senior-backend) therefore never
// matches via facets — it's only reachable through
// POPULAR_COLLECTION_FALLBACK.
export type JobFacets = {
  category?: string;
  seniority?: string;
  skills: string[];
  workMode?: string;
  countries: string[];
  regions: string[];
  collections: string[];
};

// Builds JobFacets from a wire Job — the one place that knows the
// enrichment.* vs top-level split documented on JobFacets above. Callers
// (currently jobs/[slug]/+page.server.ts) use this instead of hand-mapping
// fields, so a future Job shape change only needs updating here.
export function jobFacetsFromJob(job: Job): JobFacets {
  return {
    category: job.enrichment.category,
    seniority: job.enrichment.seniority,
    skills: job.skills,
    workMode: job.work_mode,
    countries: job.countries,
    regions: job.regions,
    collections: job.collections,
  };
}

// Popular collections padding a sparse "see also" block so it's never empty
// or too thin. Each was verified to have a healthy live count before being
// added; slugs are verified to exist in FILTER_COLLECTIONS by a test.
export const POPULAR_COLLECTION_FALLBACK = [
  'remote-worldwide',
  'javascript',
  'typescript',
  'python',
  'react',
  'go',
  'senior',
  'backend',
  'frontend',
  'aws',
];

/** The popular collections as footer links, in curated order.
 *
 *  Collection landing pages are the site's programmatic-SEO surface, and until this
 *  existed the only internal links they had came from the /collections hub and the
 *  job-detail "see also" block. The homepage — the strongest page on the domain —
 *  linked 20 jobs, 4 feature pages and not one collection. A footer strip fixes that
 *  for every page at once.
 *
 *  Returns the slug and title only, not an href: the caller builds the URL with
 *  `resolve('/collections/[slug]', …)` like every other dynamic route in the app, so
 *  the link stays correct under a non-empty `paths.base` and needs no lint exemption.
 *
 *  Resolved through collectionBySlug, the same resolver the landing route and the
 *  sitemap use, and unknown slugs are dropped rather than linked: a footer on every
 *  page is the worst place to advertise a 404. A test also asserts the pool resolves,
 *  so dropping here is belt-and-braces, not the expected path. */
export function popularCollectionLinks(): { slug: string; title: string }[] {
  return POPULAR_COLLECTION_FALLBACK.flatMap((slug) => {
    const collection = collectionBySlug(slug);
    if (!collection) return [];
    return [{ slug, title: collection.title }];
  });
}

// Whether a single FILTER_COLLECTIONS param entry is satisfied by the job's
// facets. `role` (and any other key the job carries no data for) never
// matches — see JobFacets' doc comment.
function facetMatches(paramKey: string, paramValue: string | string[], job: JobFacets): boolean {
  const values = Array.isArray(paramValue) ? paramValue : [paramValue];
  switch (paramKey) {
    case 'work_mode':
      return job.workMode !== undefined && values.includes(job.workMode);
    case 'countries':
      return values.some((v) => job.countries.includes(v));
    case 'regions':
      return values.some((v) => job.regions.includes(v));
    case 'skills':
      return values.some((v) => job.skills.includes(v));
    case 'category':
      return job.category !== undefined && values.includes(job.category);
    case 'seniority':
      return job.seniority !== undefined && values.includes(job.seniority);
    default:
      return false;
  }
}

// A collection matches a job when every one of its params is satisfied —
// mirroring how the collection's own landing page pins ALL of its params as
// a fixed scope (see collections/[slug]/+page.server.ts).
function collectionMatchesJob(entry: FilterCollection, job: JobFacets): boolean {
  return Object.entries(entry.params).every(([key, value]) => facetMatches(key, value, job));
}

// The job-detail-page "see also" block's link targets: a bounded, deduped,
// always-full list of collection slugs, built with no additional HTTP
// request from data the job page already has. Source A (the job's own
// facets, against FILTER_COLLECTIONS) is listed before Source B (the job's
// `collections` field — company membership — against the company registry),
// then padded with POPULAR_COLLECTION_FALLBACK up to `target`. Every
// returned slug resolves via collectionBySlug — this never invents a link to
// a collection that doesn't exist.
export function relatedCollectionSlugs(job: JobFacets, target = 10): string[] {
  const slugs: string[] = [];
  const seen = new Set<string>();
  const add = (slug: string) => {
    if (seen.has(slug)) return;
    seen.add(slug);
    slugs.push(slug);
  };

  for (const entry of FILTER_COLLECTIONS) {
    if (slugs.length >= target) break;
    if (collectionMatchesJob(entry, job)) add(entry.slug);
  }

  const companySlugs = new Set<string>(COMPANY_COLLECTIONS.map((c) => c.slug));
  for (const slug of job.collections) {
    if (slugs.length >= target) break;
    if (companySlugs.has(slug)) add(slug);
  }

  for (const slug of POPULAR_COLLECTION_FALLBACK) {
    if (slugs.length >= target) break;
    add(slug);
  }

  return slugs;
}

// relatedCollectionSlugs resolved to {slug, title} pairs via collectionBySlug
// — the same resolver the landing route and sitemap use — so a slug that
// somehow doesn't resolve is dropped here rather than rendered as a dead
// link. This is what JobSeeAlso renders; kept as a pure function (rather than
// resolving inline in the component) so it's unit-testable like the rest of
// this module, matching web/'s no-component-test-harness convention.
export function relatedCollectionLinks(
  job: JobFacets,
  target = 10
): { slug: string; title: string }[] {
  return relatedCollectionSlugs(job, target)
    .map((slug) => {
      const collection = collectionBySlug(slug);
      return collection ? { slug, title: collection.title } : null;
    })
    .filter((link) => link !== null);
}
