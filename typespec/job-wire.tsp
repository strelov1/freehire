import "@typespec/openapi";

using TypeSpec.OpenAPI;

namespace Freehire;

// The wire shapes the job operations answer with: the job (internal/job/jobview, shared
// with the MCP tools), the facet distribution and the company detail. The models are the
// contract — the API reference, the generated clients and Schemathesis all read them — but
// the handlers keep writing them with encoding/json (`x-raw-json`: ogen receives the type as
// raw JSON, so apigen still holds jx.Raw and the response bytes do not move).

/**
 * One posting, as every job endpoint serves it. TestJobSchemaCoversWireShape
 * (internal/job/jobview) fails when jobview.Job serves a key this model lacks.
 */
@extension("x-raw-json", true)
model Job {
  /** Stable public identifier. The freehire page is https://freehire.me/jobs/{public_slug}. */
  public_slug: string;

  title: string;
  company: string;
  company_slug?: string;

  /**
   * The employer's own application URL, as published. Served only by the paid API plans'
   * `/data/jobs` endpoints: every other job read leaves it empty wherever `apply_url` is set.
   * Empty too when `direct_apply` is true: that vacancy has no page elsewhere.
   */
  url?: string;

  /**
   * Where to send an applicant: a freehire redirect (`https://freehire.me/api/v1/go/{public_slug}?src=...`)
   * that counts the click and forwards to the employer's page. Set `src` to the surface handing the link out:
   * `web`, `mcp`, `ojcp`, `assistant`, `email`, `telegram` or `api`. Absent for a vacancy taking applications on freehire.
   */
  apply_url?: string;

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

  /**
   * The original posting when `url` points at a separately verified application
   * destination. Absent when both links are the same, and outside the paid API plans'
   * `/data/jobs` endpoints.
   */
  source_url?: string;

  /** Source-stated last application day, inclusive. Absent when the source states no deadline. */
  application_deadline?: plainDate;

  /**
   * The department or team the posting's ATS files it under, verbatim (Greenhouse, Lever,
   * Ashby, Gem, Recruitee and Employment Hero state one). Absent when the source states none; not a taxonomy, so values differ
   * between employers.
   */
  team?: string;

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
   * How many people reported that the company screens with an AI interviewer. Absent at
   * zero, so "no reports" is a missing field rather than a zero a badge could render.
   */
  @minValue(0)
  ai_interview_reports?: integer;

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

  /**
   * True only on a search hit placed as a paid slot (never on an organic hit). Render a
   * visible "Sponsored" label, and link the card to `sponsored_url`.
   */
  sponsored?: boolean;

  /**
   * Click-tracking redirect (`/api/v1/sponsored/go/<token>`) for a sponsored hit; it always
   * ends at the job. `url` stays the organic link.
   */
  sponsored_url?: string;

  /**
   * The employment type, the same value as `enrichment.employment_type`, lifted to the top
   * level. Served by `getJob` only; absent when not known.
   */
  employment_type?: string;

  /** The pay range, the same values as `enrichment.salary_*`. Served by `getJob` only; absent when no bound is stated. */
  salary?: JobSalary;

  /** The employer's own site as its company record states it. Served by `getJob` only; absent when none is on record. */
  company_website?: string;

  /**
   * The employer's logo through freehire's logo proxy, resolved by the site's domain. The
   * proxy answers 404 when it has no mark. Served by `getJob` only, beside `company_website`.
   */
  company_logo?: string;

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

  /**
   * True when the company has an approved employee referrer. Served by `getJob` only;
   * absent on every list and search row.
   */
  referral_available?: boolean;

  /**
   * True when a recruiter-contact lookup can run for the posting: the feature is configured
   * and the company has a website on record. Served by `getJob` only.
   */
  recruiter_contacts_available?: boolean;

  /**
   * The caller's own auto-apply status for the posting: `queued` (live, undecided),
   * `failed` (given up: dead-lettered, or parked for missing form data) or `declined`
   * (the candidate's permanent decision). Served by `getJob` to a signed-in caller with
   * an attempt; absent for everyone else.
   */
  auto_apply_status?: "queued" | "failed" | "declined";
}

/**
 * The listing card of a posting: what a list row draws, without the description. Served
 * only inside `TrackedJob` (`/me/tracking`), from internal/job/jobview's `Card` — a
 * narrower projection than `Job`, not a subset of its shape: `seniority` and
 * `employment_type` sit at the top level here, not under `enrichment`. TestJobSchemaCoversWireShape
 * (internal/job/jobview) fails when `jobview.Card` serves a key this model lacks.
 */
@extension("x-raw-json", true)
model JobCard {
  public_slug: string;
  title: string;
  company: string;

  /** Source-stated last application day (`YYYY-MM-DD`), inclusive. */
  application_deadline?: plainDate;

  /** Always present: when the posting closed, `null` while it is open. */
  closed_at: utcDateTime | null;

  /** The posting's own location line, verbatim. */
  location?: string;

  /** The stated facets of the tag row; each absent when the posting does not state it. */
  work_mode?: string;
  seniority?: string;
  employment_type?: string;

  countries?: string[];
  regions?: string[];

  /** The row's skills and collections, and the line under the title: the enrichment's
   * summary, or the opening of the description with tags stripped, cut server-side to
   * about 220 characters. These three and `ghost` are dropped by `?compact=1` (the Kanban
   * board's read). */
  skills?: string[];
  collections?: string[];
  blurb?: string;

  /** How many people reported that the company screens with an AI interviewer; absent at zero. */
  @minValue(0)
  ai_interview_reports?: integer;

  /** The effective posting date: the stated date, or the row's creation when the source gave none. */
  posted_at?: utcDateTime;

  /** The ghost signal, computed at read time; absent when there is nothing to say. */
  ghost?: JobGhost;
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

  /**
   * Requirements the posting states (at most 30, each at most 200 characters): the
   * model's list, or the deterministic extractor's when the model has not reached the
   * posting. `priority` is `required` or `preferred`.
   */
  requirements?: JobRequirement[];
}

/** One requirement the posting states. */
model JobRequirement {
  text: string;
  priority: "required" | "preferred";
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
  stats: Record<FacetStatRange>;

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

/** A posting's pay range: either bound may be absent, never both. */
model JobSalary {
  min?: integer;
  max?: integer;

  /** ISO 4217 (e.g. `USD`). */
  currency?: string;

  period?: string;
}

/** The smallest and largest value of one continuous facet among the matching postings. */
model FacetStatRange {
  min: float64;
  max: float64;
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

  /** Whether an employer account holds this company. */
  company_claimed?: boolean;

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
