import "@typespec/http";

using Http;

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
  url?: string;
  source?: string;
  title?: string;
  company?: string;
  location?: string;
  remote?: boolean;
  description?: string;
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

/** The newest open jobs. It reads no filter: a param other than the page ones is reported in `meta.ignored_params`. */
@tag("Jobs")
@route("/jobs")
@get
op listJobs(
  @query limit?: string,
  @query offset?: string,
): List<unknown> | Problem;

/** Resolves the URL of a posting in the wild to a catalogue slug. */
@tag("Jobs")
@route("/jobs/find")
@get
op findJob(@query url?: string): JobFindResult | Problem;

/**
 * One job by public slug. A signed-in caller also gets their own vote and auto-apply
 * status. A slug that no longer exists still answers with its posting when its shortcode
 * names exactly one.
 */
@tag("Jobs")
@route("/jobs/{slug}")
@get
op getJob(@path slug: string): Data<unknown> | Problem;

/** The open postings a collapsed job stands for, one per city. */
@tag("Jobs")
@route("/jobs/{slug}/copies")
@get
op jobCopies(
  @path slug: string,
  @query limit?: string,
  @query offset?: string,
): JobCopiesPage | Problem;

/** The application form captured for the job. 404 when none was captured. */
@tag("Jobs")
@route("/jobs/{slug}/apply-form")
@get
op jobApplyForm(@path slug: string): Data<unknown> | Problem;

/** Moderators only. Creates a hand-curated vacancy. */
@tag("Jobs")
@route("/jobs")
@post
op createJob(@body body: JobCreate): {
  @statusCode _: 201;
  @body body: Data<unknown>;
} | Problem;

/** Moderators only. Edits a manual vacancy by public slug. */
@tag("Jobs")
@route("/jobs/{slug}")
@patch
op updateJob(@path slug: string, @body body: JobUpdate): Data<unknown> | Problem;
