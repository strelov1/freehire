import "@typespec/http";

using Http;

namespace Freehire;

/** Exactly one of `job_slug`, `url` and `text`; anything else is a 400. */
model JobDescriptionSource {
  job_slug?: string;
  url?: string;
  text?: string;

  /** Hints used only with `text`. */
  title?: string;

  company?: string;
}

model ResolvedJob {
  job_slug: string;
}

/**
 * Turns a catalogue job, a vacancy URL or pasted text into a job the tailoring workspace
 * can open. 404 for an unknown slug, 422 for a URL with no readable vacancy. Session
 * cookie only.
 */
@tag("Tailoring")
@route("/me/jd/resolve")
@post
op resolveJobDescription(@body body: JobDescriptionSource): Data<ResolvedJob> | Problem;
