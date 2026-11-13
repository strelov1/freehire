namespace Freehire;

// The wire shapes the operations pass through as `unknown`: the job (internal/job/jobview,
// shared with the MCP tools), the facet distribution and the company detail. Typing them in
// an operation would make ogen build them, so no operation names these models. They
// describe the answers for readers: web/scripts/gen-api-docs.mjs puts them under `data` in
// the public schema at web/static/openapi.yaml (the ChatGPT Actions schema).

/** One posting, as every job endpoint serves it. */
model Job {
  /** Stable public identifier. The freehire page is https://freehire.me/jobs/{public_slug}. */
  public_slug: string;

  title: string;
  company: string;
  company_slug?: string;

  /**
   * The employer's own application URL. This is where an applicant goes. Empty when
   * `direct_apply` is true: that vacancy has no page elsewhere.
   */
  url?: string;

  /**
   * True for a vacancy its employer published on freehire that takes applications only on
   * its freehire page (`https://freehire.me/jobs/{slug}`). Absent for every other posting.
   */
  direct_apply?: boolean;

  /** The posting's own location string, verbatim and unnormalized. Filter on the geography facets instead. */
  location?: string;

  /** Origin slug — the ATS or board this posting was crawled from. */
  source?: string;

  /** The source's own identifier for the posting. */
  external_id?: string;

  /** Source-stated last application day, inclusive. Absent when the source states no deadline. */
  application_deadline?: plainDate;

  /** True for a hand-curated posting rather than an automated crawl. */
  manually_added?: boolean;

  /**
   * Stored verbatim HTML by default. `searchJobs` serves a truncated preview;
   * `agentSearchJobs` and `getJob` serve the full body, and `agentSearchJobs` can convert
   * it via `description_format`.
   */
  description?: string;

  regions?: string[];
  countries?: string[];
  cities?: string[];
  metros?: string[];

  /** Absent when the posting's format could not be resolved. */
  work_mode?: "remote" | "hybrid" | "onsite";

  skills?: string[];

  /** Curated collection slugs of the posting's company. */
  collections?: string[];

  /** Absent when unknown. */
  is_tech?: "tech" | "non_tech";

  /**
   * Present and `true` only when the posting states a government security-clearance
   * requirement. Absent otherwise — there is no `false`, because "no requirement was
   * detected" is not the same claim as "this posting needs no clearance".
   */
  requires_clearance?: boolean;

  /**
   * Present and `true` only when the posting states that it is published by an
   * intermediary on a client's behalf. Absent otherwise — a recruiter who never says so is
   * not detected, so absence does not mean the employer posted it.
   */
  via_recruiter?: boolean;

  /**
   * What the posting states about equity compensation: `salary_and_equity` (offered beside
   * cash pay) or `equity_only` (equity is the whole pay). Absent when the posting states
   * neither — "not stated", not "no equity".
   */
  equity?: "salary_and_equity" | "equity_only";

  /**
   * The security-clearance levels the posting states. Present exactly when
   * `requires_clearance` is, absent otherwise. `other` is a non-US scheme or a clearance
   * stated with no level.
   */
  clearance_levels?: ("confidential" | "secret" | "top_secret" | "ts_sci" | "public_trust" | "interim" | "other")[];

  /**
   * Benefits the description states it offers (see the `perks` filter for the
   * vocabulary). Empty when it names none of them — not a claim that the job offers none.
   */
  perks?: string[];

  /**
   * The legal terms the posting states it engages its worker on (see the `engagement`
   * filter for the vocabulary). Empty when it does not say.
   */
  engagement?: string[];

  /**
   * Present and `true` only when the posting's ATS provider is one freehire's auto-apply
   * feature can currently attempt to fill and submit for. Absent otherwise — a best-effort
   * provider-eligibility signal, never a guarantee of successful submission.
   */
  auto_apply_available?: boolean;

  /**
   * How many questions the posting's application form REQUIRES beyond the standard fields
   * every form asks for (name, contact details, CV, cover letter), counted from the form
   * freehire captured from the employer's ATS. Optional questions, the equal-opportunity
   * survey and consent boxes are not counted. `0` means no REQUIRED question beyond the
   * basics. Absent when freehire holds no form for the posting — "not known", never
   * "long". Served by search hits and the job detail; a list read straight from the
   * database may omit it.
   */
  @minValue(0)
  apply_questions?: integer;

  posted_at?: utcDateTime | null;

  /** When freehire first saw the posting. */
  created_at?: utcDateTime | null;

  updated_at?: utcDateTime | null;

  /** When a re-crawl last confirmed the posting was still live. */
  last_seen_at?: utcDateTime | null;

  /** Non-null once the posting is no longer open. Only `getJob` ever serves such a row. */
  closed_at?: utcDateTime | null;

  enrichment?: JobEnrichment;
  enriched_at?: utcDateTime | null;

  /** Bumped when the enrichment contract changes — use it to detect re-enriched rows. */
  enrichment_version?: integer | null;

  /** The calling user's vote; 0 or null without a signed-in caller. */
  my_vote?: integer | null;

  reality?: JobReality;
  ghost?: JobGhost;
  view_count?: integer;
  applied_count?: integer;
  upvote_count?: integer;
  downvote_count?: integer;
}

/**
 * Model-extracted attributes, every field omitted when the posting did not state it.
 * `work_mode`, `regions`, `countries`, `cities` and `skills` are deliberately NOT here —
 * those are served top-level from the deterministic dictionaries, and the model's own
 * values for them are not published.
 */
model JobEnrichment {
  /** The one synthesized field — a 1-2 sentence synopsis, always present on an enriched posting. */
  summary?: string;

  employment_type?: string;
  relocation?: string;
  visa_sponsorship?: boolean;
  timezone_note?: string;
  salary_min?: integer;
  salary_max?: integer;
  salary_currency?: string;
  salary_period?: string;
  seniority?: string;
  experience_years_min?: integer;
  english_level?: string;
  education_level?: string;
  category?: string;
  domains?: string[];
  posting_language?: string;
  company_type?: string;
  company_size?: string;
}

/**
 * Hedged "is anyone actually hiring for this" verdict, raised only when evidence fires —
 * most postings carry none, so an absent value means "no signal", never "verified real".
 * Attached by `searchJobs`, `getJob` and the plain jobs list. NOT attached by
 * `agentSearchJobs` or `getSimilarJobs`, so its absence on those two is not evidence
 * either way.
 */
model JobGhost {
  /** How strongly the evidence reads, e.g. `possible`. */
  level?: string;

  /** Which criteria fired, e.g. `evergreen_posting`, `ats_absent`. */
  criteria?: string[];

  /** How many criteria exist, so `criteria` reads as N-of-total. */
  criteria_total?: integer;

  /** Number of independent user reports, once past the disclosure gate. */
  contributors?: integer;

  /** When the posting was last looked for on the employer's own ATS. Present only when the `ats_absent` criterion fired. */
  ats_checked_at?: utcDateTime;
}

/** Signals about whether the posting reflects a real, current opening. */
model JobReality {
  class?: "fresh" | "stale" | "likely-evergreen";
  age_days?: integer;

  /** How many times this role has been reposted. */
  repost_count?: integer;

  /** How many near-identical postings the same role cluster holds. */
  mass_posting_count?: integer;

  /** True when the stated posting date looks refreshed rather than real. */
  fake_freshness?: boolean;
}

/** The `/jobs/facets` answer: posting counts per facet value under the request's filters. */
model JobFacetCounts {
  total: integer;

  /** Facet name to a map of value to matching-posting count. */
  facets: Record<Record<integer>>;

  /** Numeric ranges (min/max) for the continuous facets — `salary_min`, `salary_max`, `experience_years_min`. */
  stats?: Record<Record<float>>;

  /**
   * Present only when `facets=` names `company_slug`: the exact number of distinct
   * companies among the matching postings.
   */
  companies_total?: integer;

  /**
   * Present only when `facets=` names `company_slug`: the companies with the most matching
   * postings, in descending count. Replaces a `company_slug` entry in `facets`.
   */
  @maxItems(50)
  top_companies?: TopCompany[];
}

/** A company and how many of the matching postings it holds. */
model TopCompany {
  slug: string;

  /** Display name; the slug when the company has none yet. */
  name: string;

  count: integer;
}

/** The `/companies/{slug}` answer: the company and a page of its open jobs. */
model CompanyPage {
  company?: CompanyDetail;
  jobs?: Job[];

  /** Whether an employee referral offer exists for this company. */
  referral_available?: boolean;

  parent?: CompanyRef;

  /** Companies this one owns. Each is a separate employer with its own jobs. Omitted when there are none. */
  subsidiaries?: CompanyRef[];
}

/**
 * A related company: the parent that owns this one, or a subsidiary it owns. It is a
 * separate company with its own page and jobs, not another name for this one.
 */
model CompanyRef {
  slug: string;
  name: string;

  /** Open jobs the related company has. */
  job_count: integer;
}

/** The company summary plus the profile fields served only on the detail endpoint. */
model CompanyDetail {
  slug: string;
  name: string;
  job_count?: integer;
  tagline?: string | null;

  /** Homepage, description, and other profile fields where known. */
  company_info?: Record<unknown> | null;

  industries?: string[];
  domains?: string[];
  collections?: string[];
  regions?: string[];
  countries?: string[];

  /** Regions the company hires remotely in, derived from its open postings. */
  remote_regions?: string[];

  company_types?: string[];
  company_sizes?: string[];
  hq_country?: string | null;
  year_founded?: integer | null;
  employee_count?: integer | null;
  organization_type?: string | null;
  maturity?: string | null;
  yc_batch?: string[];
  yc_status?: string[];
  yc_stage?: string[];
  yc_flags?: string[];
  feedback_count?: integer;
  feedback_rating_avg?: float | null;
  upvote_count?: integer;
  downvote_count?: integer;

  /** The calling user's vote; 0 or null without a signed-in caller. */
  my_vote?: integer | null;
}
