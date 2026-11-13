import "@typespec/http";
import "@typespec/openapi";

using Http;
using TypeSpec.OpenAPI;

namespace Freehire;

// The public job catalogue reads and the moderator-authored job writes. A job is the
// jobview wire shape (internal/job/jobview), shared with search and the MCP tools, so it
// is passed through as `unknown` rather than modelled here.

/** The posting a page URL is, when the catalogue carries it. */
model JobFoundPosting {
  public_slug: string;
}

model JobFindMeta {
  /** Whether the URL sits on an ATS board freehire knows how to crawl. */
  ats_board: boolean;
}

/** `data` is null when the posting cannot be identified, and then `meta` says whether the board is crawlable. */
model JobFindResult {
  data: JobFoundPosting | null;
  meta?: JobFindMeta;
}

/** One per-city opening folded under a canonical job. */
model JobCopy {
  public_slug: string;
  location: string;
  apply_url: string;
  posted_at: utcDateTime | null;
}

model JobCopiesMeta {
  /** The whole closure's open size, even when the list is a capped page. */
  total: int64;
}

model JobCopiesPage {
  data: JobCopy[];
  meta: JobCopiesMeta;
}

/** The moderator create body. url, title and company are required, and the service refuses a request without them. */
model JobCreate {
  /** Link to the posting. (e.g. `https://acme.com/careers/123`) */
  url?: string;
  /** Source label. (e.g. `manual`) */
  source?: string;
  /** Job title. (e.g. `Senior Go Engineer`) */
  title?: string;
  /** Company name. (e.g. `Acme`) */
  company?: string;
  /** Free-text location. */
  location?: string;
  /** Whether the role is remote. */
  remote?: boolean;
  /** Job description. */
  description?: string;
  /** Posting date. */
  posted_at?: utcDateTime | null;
  skills?: string[];
  regions?: string[];
  cities?: string[];
  work_mode?: string;
  employment_type?: string;
  seniority?: string;
  salary_min?: int64 | null;
  salary_max?: int64 | null;
  salary_currency?: string;
  salary_period?: string;
}

/** The moderator edit body: a field left out or null is unchanged. */
model JobUpdate {
  title?: string | null;
  company?: string | null;
  location?: string | null;
  remote?: boolean | null;
  description?: string | null;
  posted_at?: utcDateTime | null;
}

// The newest open jobs. It reads no filter: a param other than the page ones is
// reported in `meta.ignored_params`.
/**
 * Takes no filters at all — `limit` and `offset` are the whole vocabulary. Anything else
 * is ignored rather than refused, and listed in `meta.ignored_params`: a filter param
 * sent here returns the entire catalogue, which is indistinguishable from a narrow result
 * unless you check. Use `/jobs/search` to filter.
 */
@summary("List jobs, newest first, with limit/offset pagination.")
@useAuth(NoAuth)
@opExample(#{
  returnType: #{
    data: #[
      #{
        public_slug: "senior-go-engineer-acme-1a2b",
        source: "greenhouse",
        manually_added: false,
        external_id: "123",
        url: "https://boards.greenhouse.io/acme/jobs/123",
        title: "Senior Go Engineer",
        company: "Acme",
        company_slug: "acme",
        location: "Remote — EU",
        description: "...",
        countries: #["DE"],
        regions: #["eu"],
        work_mode: "remote",
        skills: #["go", "postgresql"],
        cities: #["Berlin"],
        collections: #["yc"],
        is_tech: "tech",
        posted_at: utcDateTime.fromISO("2026-06-18T00:00:00Z"),
        created_at: utcDateTime.fromISO("2026-06-18T09:12:00Z"),
        updated_at: utcDateTime.fromISO("2026-06-18T09:12:00Z"),
        closed_at: null,
        enrichment: #{
          summary: "...",
          seniority: "senior",
          category: "backend",
          employment_type: "full_time",
          relocation: "not_supported",
          visa_sponsorship: false,
          experience_years_min: 5,
          english_level: "b2",
          education_level: "bachelor",
          domains: #["fintech"],
          posting_language: "en",
          company_type: "product",
          company_size: "51-200",
          salary_min: 90000,
          salary_max: 130000,
          salary_currency: "EUR",
          salary_period: "year",
        },
        enriched_at: utcDateTime.fromISO("2026-06-18T09:20:00Z"),
        enrichment_version: 7,
        view_count: 42,
        applied_count: 8,
        reality: #{
          class: "fresh",
          age_days: 3,
          repost_count: 0,
          mass_posting_count: 0,
          fake_freshness: false,
        },
      },
    ],
    meta: #{
      total: 4213,
      limit: 20,
      offset: 0,
    },
  },
})
@tag("Jobs")
@route("/jobs")
@get
op listJobs(
  /** Page size, 1–100. (e.g. `20`) */
  @query limit?: string,
  /** Rows to skip. (e.g. `0`) */
  @query offset?: string,
): List<unknown> | Problem;

// Resolves the URL of a posting in the wild to a catalogue slug.
/**
 * Answers `{"data": null}` when the posting cannot be identified, rather than 404 — "we
 * do not carry this" is a normal answer here. Used by the extension to tell whether the
 * page you are on is a job we already have. A miss also carries `meta.ats_board`: true
 * when the URL sits on an ATS board freehire knows how to crawl, so a client can tell a
 * posting worth adding from an arbitrary page.
 */
@summary("Resolve a posting URL to a catalogue slug.")
@useAuth(NoAuth)
@opExample(#{
  returnType: #{
    data: #{
      public_slug: "senior-backend-engineer-acme-1a2b",
    },
  },
})
@tag("Jobs")
@route("/jobs/find")
@get
op findJob(/** The posting's URL in the wild. (e.g. `https://boards.greenhouse.io/acme/jobs/123`) */ @query url?: string): JobFindResult | Problem;

/**
 * One job by public slug. A signed-in caller also gets their own vote and auto-apply
 * status. A slug that no longer exists still answers with its posting when its shortcode
 * names exactly one.
 */
@summary("A single job by its public slug (serves closed jobs too).")
@useAuth(NoAuth)
@opExample(#{
  returnType: #{
    data: #{
      public_slug: "senior-go-engineer-acme-1a2b",
      title: "Senior Go Engineer",
      company: "Acme",
      closed_at: null,
    },
  },
})
@tag("Jobs")
@route("/jobs/{slug}")
@get
op getJob(/** The job `public_slug`. (e.g. `senior-go-engineer-acme-1a2b`) */ @path slug: string): Data<unknown> | Problem;

// The open postings a collapsed job stands for, one per city.
/**
 * The per-city openings folded under one canonical card by content-dedup — each keeps its
 * own `location` and `apply_url` so a seeker picks their city. The anchor job itself is
 * included. `meta.total` is the whole cluster size, so it stays accurate when the list is
 * a capped page.
 */
@summary("Other open postings in the same role cluster (per-city duplicates).")
@useAuth(NoAuth)
@opExample(#{
  returnType: #{
    data: #[
      #{
        public_slug: "senior-go-engineer-acme-1a2b",
        location: "Berlin, Germany",
        apply_url: "https://boards.greenhouse.io/acme/jobs/123",
        posted_at: utcDateTime.fromISO("2026-06-18T00:00:00Z"),
      },
    ],
    meta: #{
      total: 4,
    },
  },
})
@tag("Jobs")
@route("/jobs/{slug}/copies")
@get
op jobCopies(
  /** The job `public_slug`. (e.g. `senior-go-engineer-acme-1a2b`) */
  @path slug: string,
  /** Page size, 1–200 (default 50). (e.g. `50`) */
  @query limit?: string,
  /** Rows to skip. (e.g. `0`) */
  @query offset?: string,
): JobCopiesPage | Problem;

// The application form captured for the job. 404 when none was captured.
/**
 * The questions a candidate will have to answer, shaped for reading. Only captured for a
 * minority of postings (roughly a sixth of technical ATS platforms are readable at all) —
 * no captured form is a `404`, distinguishable from an unknown slug (also `404`, but at
 * the job lookup rather than the form lookup).
 */
@summary("The captured ATS application form for the posting.")
@useAuth(NoAuth)
@opExample(#{
  returnType: #{
    data: #{
      provider: "greenhouse",
      basics: #["resume", "phone", "linkedin"],
      questions: #[
        #{
          text: "Why do you want to work here?",
          required: true,
        },
      ],
    },
  },
})
@tag("Jobs")
@route("/jobs/{slug}/apply-form")
@get
op jobApplyForm(/** The job `public_slug`. (e.g. `senior-go-engineer-acme-1a2b`) */ @path slug: string): Data<unknown> | Problem;

/** Moderators only. Creates a hand-curated vacancy. */
@summary("Create a curated job.")
@useAuth(SessionOrKey)
@extension("x-audience", "moderator")
@opExample(#{
  parameters: #{
    body: #{
      url: "https://acme.com/careers/123",
      title: "Senior Go Engineer",
      company: "Acme",
    },
  },
  returnType: #{
    _: 201,
    body: #{
      data: #{
        public_slug: "senior-go-engineer-acme-1a2b",
        title: "Senior Go Engineer",
        manually_added: true,
      },
    },
  },
})
@tag("Jobs")
@route("/jobs")
@post
op createJob(@body body: JobCreate): {
  @statusCode _: 201;
  @body body: Data<unknown>;
} | Problem;

/** Moderators only. Edits a manual vacancy by public slug. */
@summary("Edit a curated job.")
@useAuth(SessionOrKey)
@extension("x-audience", "moderator")
@opExample(#{
  parameters: #{
    slug: "<slug>",
    body: #{
      title: "Staff Go Engineer",
    },
  },
  returnType: #{
    data: #{
      public_slug: "...",
      title: "Staff Go Engineer",
    },
  },
})
@tag("Jobs")
@route("/jobs/{slug}")
@patch
op updateJob(/** The job `public_slug`. */ @path slug: string, @body body: JobUpdate): Data<unknown> | Problem;
