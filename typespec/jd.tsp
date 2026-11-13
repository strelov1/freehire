import "@typespec/http";

using Http;

namespace Freehire;

/** Exactly one of `job_slug`, `url` and `text`; anything else is a 400. */
model JobDescriptionSource {
  /** An existing job’s slug. */
  job_slug?: string;
  /** A job posting URL. */
  url?: string;
  /** Pasted job description text. */
  text?: string;

  /** Hints used only with `text`. */
  title?: string;

  /** Optional hint, used only alongside `text`. */
  company?: string;
}

model ResolvedJob {
  job_slug: string;
}

// Turns a catalogue job, a vacancy URL or pasted text into a job the tailoring
// workspace can open. 404 for an unknown slug, 422 for a URL with no readable vacancy.
// Session cookie only.
/**
 * Exactly one of `job_slug`, `url`, or `text` is required. A recognized URL resolves
 * through the same ATS-aware registry `/jobs/resolve` uses (a network fetch); an
 * unrecognized URL falls back to a generic scrape; pasted text is stored as a private,
 * unlisted job. Returns the slug the tailor workspace should open — an existing job, or a
 * freshly created private one. `404` for an unknown `job_slug`, `422` for a URL nothing
 * could read.
 */
@summary("Turn a URL, pasted text, or an existing job into one usable by CV tailoring.")
@opExample(#{
  parameters: #{
    body: #{
      url: "https://boards.greenhouse.io/acme/jobs/123",
    },
  },
  returnType: #{
    data: #{
      job_slug: "senior-go-engineer-acme-1a2b",
    },
  },
})
@tag("Tailoring")
@route("/me/jd/resolve")
@post
op resolveJobDescription(@body body: JobDescriptionSource): Data<ResolvedJob> | Problem;
