// A filter collection is the second kind of collection: a curated card that maps
// to an arbitrary /jobs facet filter rather than company membership. Unlike
// COLLECTIONS it is frontend-only — no Go registry, no `collections` search-facet
// value, no company/job membership. Adding one is a single entry below.
export type FilterCollection = {
  slug: string;
  // Which section of the /collections hub the card sits in. Explicit rather than
  // derived from `params`: a `skills` pin alone cannot say whether it names a
  // language, a framework or a piece of infrastructure.
  group: FilterCollectionGroup;
  title: string;
  description: string;
  // Job-search facet params this collection maps to — the same param names the
  // /jobs feed accepts (see search.StringFacets). A value may be a single string
  // or a list; a list expands into repeated query keys (OR semantics), matching
  // the /jobs filter contract.
  params: Record<string, string | string[]>;
  // ISO 3166-1 alpha-2 code whose flag marks the card, for a collection whose params
  // name a place without pinning its country (a city). Display only — never a filter.
  flag?: string;
};

export type FilterCollectionGroup =
  | 'remote'
  | 'language'
  | 'framework'
  | 'infra'
  | 'field'
  | 'seniority'
  | 'role'
  | 'industry'
  | 'city'
  | 'job-type'
  | 'company-type'
  | 'ai';

export const FILTER_COLLECTIONS: FilterCollection[] = [
  {
    slug: 'remote-worldwide',
    group: 'remote',
    title: 'Remote Worldwide',
    description: 'Fully remote roles open to candidates anywhere in the world, not tied to a country or region.',
    params: { work_mode: 'remote', regions: 'global' },
  },
  // Regional remote landings. Params use the canonical facet vocabulary: regions
  // from REGION_LABELS (there is no `us` region — the US is country-level), and
  // countries as ISO 3166-1 alpha-2. Each was confirmed to have a healthy,
  // non-empty live count before shipping.
  {
    slug: 'remote-latam',
    group: 'remote',
    title: 'Remote Latam',
    description: 'Fully remote roles open to candidates across Latin America.',
    params: { work_mode: 'remote', regions: 'latam' },
  },
  {
    slug: 'remote-brasil',
    group: 'remote',
    title: 'Remote Brasil',
    description: 'Fully remote roles open to candidates in Brazil.',
    params: { work_mode: 'remote', countries: 'br' },
  },
  {
    slug: 'remote-us',
    group: 'remote',
    title: 'Remote US',
    description: 'Fully remote roles open to candidates in the United States.',
    params: { work_mode: 'remote', countries: 'us' },
  },
  {
    slug: 'remote-europe',
    group: 'remote',
    title: 'Remote Europe',
    description: 'Fully remote roles open to candidates across Europe.',
    params: { work_mode: 'remote', regions: 'eu' },
  },
  {
    slug: 'remote-apac',
    group: 'remote',
    title: 'Remote APAC',
    description: 'Fully remote roles open to candidates across Asia-Pacific.',
    params: { work_mode: 'remote', regions: 'apac' },
  },
  // Single-country remote landings — the "remote jobs in <country>" pattern,
  // alongside the regional ones above. Countries are ISO 3166-1 alpha-2. Each
  // was confirmed to have a healthy live count (thousands) before shipping.
  {
    slug: 'remote-canada',
    group: 'remote',
    title: 'Remote Canada',
    description: 'Fully remote roles open to candidates in Canada.',
    params: { work_mode: 'remote', countries: 'ca' },
  },
  {
    slug: 'remote-uk',
    group: 'remote',
    title: 'Remote UK',
    description: 'Fully remote roles open to candidates in the United Kingdom.',
    params: { work_mode: 'remote', countries: 'gb' },
  },
  {
    slug: 'remote-india',
    group: 'remote',
    title: 'Remote India',
    description: 'Fully remote roles open to candidates in India.',
    params: { work_mode: 'remote', countries: 'in' },
  },
  {
    slug: 'remote-poland',
    group: 'remote',
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
    group: 'language',
    title: 'Python',
    description: 'Open roles that use Python — backend, data, ML and automation.',
    params: { skills: 'python' },
  },
  {
    slug: 'javascript',
    group: 'language',
    title: 'JavaScript',
    description: 'Open roles that use JavaScript across web and backend.',
    params: { skills: 'javascript' },
  },
  {
    slug: 'typescript',
    group: 'language',
    title: 'TypeScript',
    description: 'Open roles that use TypeScript for typed JavaScript at scale.',
    params: { skills: 'typescript' },
  },
  {
    slug: 'java',
    group: 'language',
    title: 'Java',
    description: 'Open roles that use Java — enterprise backends, Android and big data.',
    params: { skills: 'java' },
  },
  {
    slug: 'csharp',
    group: 'language',
    title: 'C#',
    description: 'Open roles that use C# and the .NET ecosystem.',
    params: { skills: 'csharp' },
  },
  {
    slug: 'cpp',
    group: 'language',
    title: 'C++',
    description: 'Open roles that use C++ — systems, games and performance-critical code.',
    params: { skills: 'cpp' },
  },
  {
    slug: 'go',
    group: 'language',
    title: 'Go',
    description: 'Open roles that use Go for backends, infra and cloud-native services.',
    params: { skills: 'go' },
  },
  {
    slug: 'rust',
    group: 'language',
    title: 'Rust',
    description: 'Open roles that use Rust for safe, high-performance systems.',
    params: { skills: 'rust' },
  },
  {
    slug: 'ruby',
    group: 'language',
    title: 'Ruby',
    description: 'Open roles that use Ruby, from web apps to tooling.',
    params: { skills: 'ruby' },
  },
  {
    slug: 'php',
    group: 'language',
    title: 'PHP',
    description: 'Open roles that use PHP for web backends and platforms.',
    params: { skills: 'php' },
  },
  {
    slug: 'kotlin',
    group: 'language',
    title: 'Kotlin',
    description: 'Open roles that use Kotlin for Android and JVM backends.',
    params: { skills: 'kotlin' },
  },
  {
    slug: 'swift',
    group: 'language',
    title: 'Swift',
    description: 'Open roles that use Swift for iOS, macOS and Apple platforms.',
    params: { skills: 'swift' },
  },
  {
    slug: 'scala',
    group: 'language',
    title: 'Scala',
    description: 'Open roles that use Scala for JVM backends and data engineering.',
    params: { skills: 'scala' },
  },
  {
    slug: 'nodejs',
    group: 'framework',
    title: 'Node.js',
    description: 'Open roles that use Node.js for JavaScript backends and APIs.',
    params: { skills: 'nodejs' },
  },
  {
    slug: 'clojure',
    group: 'language',
    title: 'Clojure',
    description: 'Open roles that use Clojure and functional JVM development.',
    params: { skills: 'clojure' },
  },
  {
    slug: 'elixir',
    group: 'language',
    title: 'Elixir',
    description: 'Open roles that use Elixir and the BEAM for scalable backends.',
    params: { skills: 'elixir' },
  },
  {
    slug: 'react',
    group: 'framework',
    title: 'React',
    description: 'Open roles that use React to build web interfaces.',
    params: { skills: 'react' },
  },
  {
    slug: 'angular',
    group: 'framework',
    title: 'Angular',
    description: 'Open roles that use Angular for web applications.',
    params: { skills: 'angular' },
  },
  {
    slug: 'vue',
    group: 'framework',
    title: 'Vue',
    description: 'Open roles that use Vue.js for web interfaces.',
    params: { skills: 'vue' },
  },
  {
    slug: 'nextjs',
    group: 'framework',
    title: 'Next.js',
    description: 'Open roles that use Next.js for full-stack React apps.',
    params: { skills: 'nextjs' },
  },
  {
    slug: 'spring',
    group: 'framework',
    title: 'Spring',
    description: 'Open roles that use Spring for Java backends.',
    params: { skills: 'spring' },
  },
  {
    slug: 'rails',
    group: 'framework',
    title: 'Rails',
    description: 'Open roles that use Ruby on Rails for web applications.',
    params: { skills: 'rails' },
  },
  {
    slug: 'django',
    group: 'framework',
    title: 'Django',
    description: 'Open roles that use Django for Python web backends.',
    params: { skills: 'django' },
  },
  {
    slug: 'svelte',
    group: 'framework',
    title: 'Svelte',
    description: 'Open roles that use Svelte and SvelteKit for web interfaces.',
    params: { skills: 'svelte' },
  },
  // Tech-category landings — the "<category> jobs" search pattern, one per canonical
  // `category` facet value (see vocab.CategoryValues). The param is `category`; the
  // slug is a readable form of the value — usually its kebab case (data_engineering →
  // data-engineering), but chosen for readability where they differ (ml_ai →
  // machine-learning). Engineering categories plus the two that sit beside an
  // engineering team — data analytics and product management, the second and
  // eleventh most-typed searches on 2026-10-24 — while sales/support/marketing
  // stay off-audience. Each was confirmed to have a healthy live count (≥ 300)
  // before shipping.
  {
    slug: 'backend',
    group: 'field',
    title: 'Backend',
    description: 'Server-side engineering roles — APIs, services, databases and backend systems.',
    params: { category: 'backend' },
  },
  {
    slug: 'frontend',
    group: 'field',
    title: 'Frontend',
    description: 'Frontend engineering roles building web user interfaces and client-side apps.',
    params: { category: 'frontend' },
  },
  {
    slug: 'fullstack',
    group: 'field',
    title: 'Full-Stack',
    description: 'Full-stack roles spanning both frontend and backend development.',
    params: { category: 'fullstack' },
  },
  {
    slug: 'devops',
    group: 'field',
    title: 'DevOps',
    description: 'DevOps roles automating build, deployment and cloud infrastructure.',
    params: { category: 'devops' },
  },
  {
    slug: 'sre',
    group: 'field',
    title: 'SRE',
    description: 'Site reliability roles keeping production systems scalable and resilient.',
    params: { category: 'sre' },
  },
  {
    slug: 'data-engineering',
    group: 'field',
    title: 'Data Engineering',
    description: 'Data engineering roles building pipelines, warehouses and data platforms.',
    params: { category: 'data_engineering' },
  },
  {
    slug: 'data-science',
    group: 'field',
    title: 'Data Science',
    description: 'Data science roles turning data into models, insight and decisions.',
    params: { category: 'data_science' },
  },
  {
    slug: 'data-analytics',
    group: 'field',
    title: 'Data Analytics',
    description:
      'Data and analytics roles — analysts, analytics engineers and BI — turning data into reporting and decisions.',
    params: { category: 'data_analytics' },
  },
  {
    slug: 'machine-learning',
    group: 'field',
    title: 'Machine Learning',
    description: 'Machine learning roles training and shipping ML models to production.',
    params: { category: 'ml_ai' },
  },
  {
    slug: 'ai-engineering',
    group: 'field',
    title: 'AI Engineering',
    description: 'AI engineering roles building LLM and generative-AI powered products.',
    params: { category: 'ai_engineering' },
  },
  {
    slug: 'mobile',
    group: 'field',
    title: 'Mobile',
    description: 'Mobile engineering roles for iOS, Android and cross-platform apps.',
    params: { category: 'mobile' },
  },
  {
    slug: 'security',
    group: 'field',
    title: 'Security',
    description: 'Security roles covering application, cloud and infrastructure security.',
    params: { category: 'security' },
  },
  {
    slug: 'qa',
    group: 'field',
    title: 'QA',
    description: 'Quality assurance and test engineering roles across manual and automated testing.',
    params: { category: 'qa' },
  },
  {
    slug: 'architecture',
    group: 'field',
    title: 'Architecture',
    description: 'Software and solution architecture roles designing systems at scale.',
    params: { category: 'architecture' },
  },
  {
    slug: 'embedded',
    group: 'field',
    title: 'Embedded',
    description: 'Embedded and firmware roles programming devices and low-level systems.',
    params: { category: 'embedded' },
  },
  {
    slug: 'network-engineering',
    group: 'field',
    title: 'Network Engineering',
    description: 'Network engineering roles designing and operating network infrastructure.',
    params: { category: 'network_engineering' },
  },
  {
    slug: 'product-management',
    group: 'field',
    title: 'Product Management',
    description: 'Product management roles — product managers and product owners shaping what gets built.',
    params: { category: 'product' },
  },
  // Seniority landings — the "<level> jobs" pattern, one per canonical `seniority`
  // facet value. Copy leans on the level ("Senior-Level"), not a category it cannot
  // claim. Generic intent, but each has a large live count.
  {
    slug: 'junior',
    group: 'seniority',
    title: 'Junior-Level',
    description: 'Entry-level and junior engineering roles for early-career developers.',
    params: { seniority: 'junior' },
  },
  {
    slug: 'mid-level',
    group: 'seniority',
    title: 'Mid-Level',
    description: 'Mid-level engineering roles for developers with a few years of experience.',
    params: { seniority: 'middle' },
  },
  {
    slug: 'senior',
    group: 'seniority',
    title: 'Senior-Level',
    description: 'Senior engineering roles for experienced developers who lead delivery.',
    params: { seniority: 'senior' },
  },
  {
    slug: 'lead',
    group: 'seniority',
    title: 'Lead',
    description: 'Lead engineering roles owning technical direction and delivery for a team.',
    params: { seniority: 'lead' },
  },
  {
    slug: 'staff',
    group: 'seniority',
    title: 'Staff',
    description: 'Staff engineering roles driving technical strategy across teams.',
    params: { seniority: 'staff' },
  },
  {
    slug: 'principal',
    group: 'seniority',
    title: 'Principal',
    description: 'Principal engineering roles setting technical direction org-wide.',
    params: { seniority: 'principal' },
  },
  {
    slug: 'internship',
    group: 'seniority',
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
    group: 'infra',
    title: 'AWS',
    description: 'Open roles that use Amazon Web Services for cloud infrastructure.',
    params: { skills: 'aws' },
  },
  {
    slug: 'kubernetes',
    group: 'infra',
    title: 'Kubernetes',
    description: 'Open roles that use Kubernetes to orchestrate containerized workloads.',
    params: { skills: 'kubernetes' },
  },
  {
    slug: 'terraform',
    group: 'infra',
    title: 'Terraform',
    description: 'Open roles that use Terraform for infrastructure as code.',
    params: { skills: 'terraform' },
  },
  {
    slug: 'docker',
    group: 'infra',
    title: 'Docker',
    description: 'Open roles that use Docker to build and run containerized apps.',
    params: { skills: 'docker' },
  },
  {
    slug: 'postgresql',
    group: 'infra',
    title: 'PostgreSQL',
    description: 'Open roles that use PostgreSQL as the primary relational database.',
    params: { skills: 'postgresql' },
  },
  {
    slug: 'redis',
    group: 'infra',
    title: 'Redis',
    description: 'Open roles that use Redis for caching, queues and real-time data.',
    params: { skills: 'redis' },
  },
  {
    slug: 'kafka',
    group: 'infra',
    title: 'Kafka',
    description: 'Open roles that use Apache Kafka for event streaming and messaging.',
    params: { skills: 'kafka' },
  },
  {
    slug: 'graphql',
    group: 'infra',
    title: 'GraphQL',
    description: 'Open roles that use GraphQL for API design and data fetching.',
    params: { skills: 'graphql' },
  },
  {
    slug: 'mongodb',
    group: 'infra',
    title: 'MongoDB',
    description: 'Open roles that use MongoDB as the primary document database.',
    params: { skills: 'mongodb' },
  },
  {
    slug: 'mysql',
    group: 'infra',
    title: 'MySQL',
    description: 'Open roles that use MySQL as the primary relational database.',
    params: { skills: 'mysql' },
  },
  {
    slug: 'elasticsearch',
    group: 'infra',
    title: 'Elasticsearch',
    description: 'Open roles that use Elasticsearch for search and log analytics.',
    params: { skills: 'elasticsearch' },
  },
  {
    slug: 'dotnet',
    group: 'framework',
    title: '.NET',
    description: 'Open roles that use .NET for backend and enterprise applications.',
    params: { skills: 'dotnet' },
  },
  {
    slug: 'laravel',
    group: 'framework',
    title: 'Laravel',
    description: 'Open roles that use Laravel for PHP web backends.',
    params: { skills: 'laravel' },
  },
  {
    slug: 'flutter',
    group: 'framework',
    title: 'Flutter',
    description: 'Open roles that use Flutter for cross-platform mobile apps.',
    params: { skills: 'flutter' },
  },
  {
    slug: 'azure',
    group: 'infra',
    title: 'Azure',
    description: 'Open roles that use Microsoft Azure for cloud infrastructure.',
    params: { skills: 'azure' },
  },
  {
    slug: 'gcp',
    group: 'infra',
    title: 'Google Cloud',
    description: 'Open roles that use Google Cloud Platform for cloud infrastructure.',
    params: { skills: 'gcp' },
  },
  {
    slug: 'jenkins',
    group: 'infra',
    title: 'Jenkins',
    description: 'Open roles that use Jenkins for CI/CD automation.',
    params: { skills: 'jenkins' },
  },
  {
    slug: 'ansible',
    group: 'infra',
    title: 'Ansible',
    description: 'Open roles that use Ansible for infrastructure automation.',
    params: { skills: 'ansible' },
  },
  {
    slug: 'pytorch',
    group: 'framework',
    title: 'PyTorch',
    description: 'Open roles that use PyTorch for machine learning.',
    params: { skills: 'pytorch' },
  },
  {
    slug: 'tensorflow',
    group: 'framework',
    title: 'TensorFlow',
    description: 'Open roles that use TensorFlow for machine learning.',
    params: { skills: 'tensorflow' },
  },
  {
    slug: 'solidity',
    group: 'language',
    title: 'Solidity',
    description: 'Open roles that use Solidity for smart-contract and blockchain development.',
    params: { skills: 'solidity' },
  },
  // Job landings, keyed on the two axes a retired `role` slug decomposed into. Every
  // one of these was `role: senior_backend` and friends; the pair says the same thing
  // in the vocabulary the rest of the filter uses, so the URLs and their feeds survive
  // the facet that used to back them.
  //
  // `founding-engineer` is the one that had no mechanical pair — the slug named a
  // stage, not a grade — so it reads as software engineering at a startup, which is
  // what the phrase means. Judged, not derived, and worth saying out loud.
  {
    slug: 'software-engineer',
    group: 'role',
    title: 'Software Engineer',
    description: 'General software engineering roles across the stack and domains.',
    params: { category: 'software_engineering' },
  },
  {
    slug: 'senior-backend',
    group: 'role',
    title: 'Senior Backend',
    description: 'Senior backend engineering roles owning server-side systems and APIs.',
    params: { category: 'backend', seniority: 'senior' },
  },
  {
    slug: 'senior-frontend',
    group: 'role',
    title: 'Senior Frontend',
    description: 'Senior frontend engineering roles owning web UI architecture and delivery.',
    params: { category: 'frontend', seniority: 'senior' },
  },
  {
    slug: 'founding-engineer',
    group: 'role',
    title: 'Founding Engineer',
    description: 'Founding engineer roles building the first product at early-stage startups.',
    params: { category: 'software_engineering', company_type: 'startup' },
  },
  {
    slug: 'senior-devops',
    group: 'role',
    title: 'Senior DevOps',
    description: 'Senior DevOps engineering roles owning build, deployment and infrastructure.',
    params: { category: 'devops', seniority: 'senior' },
  },
  {
    slug: 'senior-fullstack',
    group: 'role',
    title: 'Senior Full-Stack',
    description: 'Senior full-stack engineering roles spanning frontend and backend delivery.',
    params: { category: 'fullstack', seniority: 'senior' },
  },
  {
    slug: 'senior-data-engineering',
    group: 'role',
    title: 'Senior Data Engineer',
    description: 'Senior data engineering roles building pipelines and data platforms.',
    params: { category: 'data_engineering', seniority: 'senior' },
  },
  {
    slug: 'staff-engineer',
    group: 'role',
    title: 'Staff Engineer',
    description: 'Staff engineering roles driving technical strategy for a team or product area.',
    params: { category: 'software_engineering', seniority: 'staff' },
  },
  {
    slug: 'engineering-manager',
    group: 'role',
    title: 'Engineering Manager',
    description: 'Engineering management roles leading and growing a team of engineers.',
    params: { category: 'software_engineering', role_type: 'people_manager' },
  },
  // Industry landings — one per `domains` facet value (the employer's market, as
  // enrichment reads it). `ai` is left out: the `ai` company collection already owns
  // that slug and that subject.
  {
    slug: 'fintech',
    group: 'industry',
    title: 'Fintech',
    description: 'Open tech roles at companies in financial technology — payments, banking, lending and insurance.',
    params: { domains: 'fintech' },
  },
  {
    slug: 'healthcare',
    group: 'industry',
    title: 'Healthcare',
    description: 'Open tech roles at companies in healthcare and health tech.',
    params: { domains: 'healthcare' },
  },
  {
    slug: 'govtech',
    group: 'industry',
    title: 'GovTech',
    description: 'Open tech roles at companies in government and public-sector technology.',
    params: { domains: 'govtech' },
  },
  {
    slug: 'devtools',
    group: 'industry',
    title: 'Developer Tools',
    description: 'Open tech roles at companies in developer tools and platforms.',
    params: { domains: 'devtools' },
  },
  {
    slug: 'cybersecurity',
    group: 'industry',
    title: 'Cybersecurity',
    description: 'Open tech roles at companies in cybersecurity.',
    params: { domains: 'cybersecurity' },
  },
  {
    slug: 'ecommerce',
    group: 'industry',
    title: 'E-commerce',
    description: 'Open tech roles at companies in e-commerce and retail technology.',
    params: { domains: 'ecommerce' },
  },
  {
    slug: 'saas',
    group: 'industry',
    title: 'SaaS',
    description: 'Open tech roles at companies in B2B software-as-a-service.',
    params: { domains: 'saas' },
  },
  {
    slug: 'logistics',
    group: 'industry',
    title: 'Logistics',
    description: 'Open tech roles at companies in logistics and supply chain.',
    params: { domains: 'logistics' },
  },
  {
    slug: 'climate-tech',
    group: 'industry',
    title: 'Climate Tech',
    description: 'Open tech roles at companies in climate and energy technology.',
    params: { domains: 'climatetech' },
  },
  {
    slug: 'mobility',
    group: 'industry',
    title: 'Mobility',
    description: 'Open tech roles at companies in mobility, automotive and transportation.',
    params: { domains: 'mobility' },
  },
  {
    slug: 'media',
    group: 'industry',
    title: 'Media',
    description: 'Open tech roles at companies in media, content and entertainment.',
    params: { domains: 'media' },
  },
  {
    slug: 'hr-tech',
    group: 'industry',
    title: 'HR Tech',
    description: 'Open tech roles at companies in HR and recruiting technology.',
    params: { domains: 'hrtech' },
  },
  {
    slug: 'edtech',
    group: 'industry',
    title: 'EdTech',
    description: 'Open tech roles at companies in education technology.',
    params: { domains: 'edtech' },
  },
  {
    slug: 'gaming',
    group: 'industry',
    title: 'Gaming',
    description: 'Open tech roles at companies in game development.',
    params: { domains: 'gamedev' },
  },
  {
    slug: 'proptech',
    group: 'industry',
    title: 'PropTech',
    description: 'Open tech roles at companies in real-estate technology.',
    params: { domains: 'proptech' },
  },
  {
    slug: 'adtech',
    group: 'industry',
    title: 'AdTech',
    description: 'Open tech roles at companies in advertising and marketing technology.',
    params: { domains: 'adtech' },
  },
  {
    slug: 'travel',
    group: 'industry',
    title: 'Travel',
    description: 'Open tech roles at companies in travel and hospitality technology.',
    params: { domains: 'travel' },
  },
  {
    slug: 'crypto',
    group: 'industry',
    title: 'Crypto & Web3',
    description: 'Open tech roles at companies in crypto, blockchain and web3.',
    params: { domains: 'crypto' },
  },
  // City landings — the "<city> tech jobs" pattern. `cities` carries the facet's exact
  // spelling (`New York City`, `São Paulo`). The country is NOT pinned beside it: the
  // location facets combine with OR, so `cities=Austin&countries=us` is every US job,
  // not the Austin ones. It rides in `flag` instead, which only draws the card's mark.
  {
    slug: 'singapore',
    group: 'city',
    title: 'Singapore',
    description: 'Open tech roles based in Singapore.',
    params: { cities: 'Singapore' },
    flag: 'sg',
  },
  {
    slug: 'new-york',
    group: 'city',
    title: 'New York',
    description: 'Open tech roles based in New York.',
    params: { cities: 'New York City' },
    flag: 'us',
  },
  {
    slug: 'london',
    group: 'city',
    title: 'London',
    description: 'Open tech roles based in London.',
    params: { cities: 'London' },
    flag: 'gb',
  },
  {
    slug: 'san-francisco',
    group: 'city',
    title: 'San Francisco',
    description: 'Open tech roles based in San Francisco.',
    params: { cities: 'San Francisco' },
    flag: 'us',
  },
  {
    slug: 'bengaluru',
    group: 'city',
    title: 'Bengaluru',
    description: 'Open tech roles based in Bengaluru.',
    params: { cities: 'Bengaluru' },
    flag: 'in',
  },
  {
    slug: 'austin',
    group: 'city',
    title: 'Austin',
    description: 'Open tech roles based in Austin.',
    params: { cities: 'Austin' },
    flag: 'us',
  },
  {
    slug: 'toronto',
    group: 'city',
    title: 'Toronto',
    description: 'Open tech roles based in Toronto.',
    params: { cities: 'Toronto' },
    flag: 'ca',
  },
  {
    slug: 'boston',
    group: 'city',
    title: 'Boston',
    description: 'Open tech roles based in Boston.',
    params: { cities: 'Boston' },
    flag: 'us',
  },
  {
    slug: 'paris',
    group: 'city',
    title: 'Paris',
    description: 'Open tech roles based in Paris.',
    params: { cities: 'Paris' },
    flag: 'fr',
  },
  {
    slug: 'berlin',
    group: 'city',
    title: 'Berlin',
    description: 'Open tech roles based in Berlin.',
    params: { cities: 'Berlin' },
    flag: 'de',
  },
  {
    slug: 'seattle',
    group: 'city',
    title: 'Seattle',
    description: 'Open tech roles based in Seattle.',
    params: { cities: 'Seattle' },
    flag: 'us',
  },
  {
    slug: 'hyderabad',
    group: 'city',
    title: 'Hyderabad',
    description: 'Open tech roles based in Hyderabad.',
    params: { cities: 'Hyderabad' },
    flag: 'in',
  },
  {
    slug: 'sydney',
    group: 'city',
    title: 'Sydney',
    description: 'Open tech roles based in Sydney.',
    params: { cities: 'Sydney' },
    flag: 'au',
  },
  {
    slug: 'dubai',
    group: 'city',
    title: 'Dubai',
    description: 'Open tech roles based in Dubai.',
    params: { cities: 'Dubai' },
    flag: 'ae',
  },
  {
    slug: 'warsaw',
    group: 'city',
    title: 'Warsaw',
    description: 'Open tech roles based in Warsaw.',
    params: { cities: 'Warsaw' },
    flag: 'pl',
  },
  {
    slug: 'sao-paulo',
    group: 'city',
    title: 'São Paulo',
    description: 'Open tech roles based in São Paulo.',
    params: { cities: 'São Paulo' },
    flag: 'br',
  },
  {
    slug: 'madrid',
    group: 'city',
    title: 'Madrid',
    description: 'Open tech roles based in Madrid.',
    params: { cities: 'Madrid' },
    flag: 'es',
  },
  {
    slug: 'amsterdam',
    group: 'city',
    title: 'Amsterdam',
    description: 'Open tech roles based in Amsterdam.',
    params: { cities: 'Amsterdam' },
    flag: 'nl',
  },
  {
    slug: 'munich',
    group: 'city',
    title: 'Munich',
    description: 'Open tech roles based in Munich.',
    params: { cities: 'Munich' },
    flag: 'de',
  },
  {
    slug: 'dublin',
    group: 'city',
    title: 'Dublin',
    description: 'Open tech roles based in Dublin.',
    params: { cities: 'Dublin' },
    flag: 'ie',
  },
  {
    slug: 'tokyo',
    group: 'city',
    title: 'Tokyo',
    description: 'Open tech roles based in Tokyo.',
    params: { cities: 'Tokyo' },
    flag: 'jp',
  },
  // Job-type landings — how the role is held rather than what it does. Internships live
  // under seniority, which already owns the `internship` slug.
  {
    slug: 'contract',
    group: 'job-type',
    title: 'Contract',
    description: 'Open tech roles on a contract or freelance basis.',
    params: { employment_type: 'contract' },
  },
  {
    slug: 'part-time',
    group: 'job-type',
    title: 'Part-time',
    description: 'Open part-time tech roles.',
    params: { employment_type: 'part_time' },
  },
  {
    slug: 'relocation',
    group: 'job-type',
    title: 'Relocation supported',
    description: 'Open tech roles where the employer offers to help you relocate.',
    params: { relocation: 'supported' },
  },
  {
    slug: 'fellowship',
    group: 'job-type',
    title: 'Fellowships',
    description: 'Open tech fellowships and residency programmes.',
    params: { employment_type: 'fellowship' },
  },
  // Company-type landings — the employer's business model, as enrichment reads
  // it from the posting.
  {
    slug: 'startups',
    group: 'company-type',
    title: 'Startups',
    description: 'Open tech roles at startups.',
    params: { company_type: 'startup' },
  },
  {
    slug: 'product-companies',
    group: 'company-type',
    title: 'Product companies',
    description: 'Open tech roles at companies building their own product.',
    params: { company_type: 'product' },
  },
  {
    slug: 'outsourcing',
    group: 'company-type',
    title: 'Outsourcing',
    description: 'Open tech roles at outsourcing and IT-services companies.',
    params: { company_type: 'outsource' },
  },
  {
    slug: 'outstaffing',
    group: 'company-type',
    title: 'Outstaffing',
    description: 'Open tech roles at outstaffing companies that place engineers with clients.',
    params: { company_type: 'outstaff' },
  },
  {
    slug: 'agencies',
    group: 'company-type',
    title: 'Agencies',
    description: 'Open tech roles at digital and creative agencies.',
    params: { company_type: 'agency' },
  },
  {
    slug: 'government',
    group: 'company-type',
    title: 'Government',
    description: 'Open tech roles in government and the public sector.',
    params: { company_type: 'government' },
  },
  // AI-career landings — one per `ai_archetype` value: what kind of AI work the role
  // actually is, which the `ml_ai`/`ai_engineering` categories cannot tell apart.
  {
    slug: 'ml-research',
    group: 'ai',
    title: 'ML Research & Training',
    description: 'Open roles training and researching machine-learning models.',
    params: { ai_archetype: 'ml_trainer_researcher' },
  },
  {
    slug: 'rag-engineer',
    group: 'ai',
    title: 'RAG Engineer',
    description: 'Open roles building retrieval-augmented LLM applications.',
    params: { ai_archetype: 'rag_app_builder' },
  },
  {
    slug: 'ai-agent-engineer',
    group: 'ai',
    title: 'AI Agent Engineer',
    description: 'Open roles building LLM agents and agentic workflows.',
    params: { ai_archetype: 'agent_builder' },
  },
  {
    slug: 'fullstack-ai-engineer',
    group: 'ai',
    title: 'Full-Stack AI Engineer',
    description: 'Open roles shipping AI features end to end, from model to product.',
    params: { ai_archetype: 'fullstack_ai_engineer' },
  },
  {
    slug: 'ml-platform-engineer',
    group: 'ai',
    title: 'ML Platform Engineer',
    description: 'Open roles building the cloud platforms ML models are trained and served on.',
    params: { ai_archetype: 'cloud_ml_platform_engineer' },
  },
  {
    slug: 'ai-infra-engineer',
    group: 'ai',
    title: 'AI Infrastructure',
    description: 'Open DevOps and infrastructure roles on AI systems.',
    params: { ai_archetype: 'devops_infra_engineer' },
  },
];
