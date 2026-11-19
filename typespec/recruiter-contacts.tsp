import "@typespec/http";
import "@typespec/openapi";

using Http;
using TypeSpec.OpenAPI;

namespace Freehire;

/** One person to write to about a job, found from public sources. */
model RecruiterContact {
  name: string;
  title: string;
  email: string;

  /** `high` for a verified address, `medium` for a likely one. */
  confidence: "high" | "medium";
}

// The recruiter contacts for a job's company. Paid: the gate in front answers its own 402.
/**
 * Up to three people at the job's company who recruit, with their work email. The first view
 * of a job draws one from your daily `recruiter-contact` allowance; looking again at a job you
 * already opened is free, and an empty result or a failure costs nothing. `[]` when nobody
 * usable is known. 402 without a paid plan or with the allowance spent; 404 for an unknown job
 * or when the feature is off; 422 when the company has no website on record; 503 when the
 * lookup is unavailable (try later, nothing was charged).
 */
@summary("Find who recruits for a job.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #[
      #{
        name: "Anna Lee",
        title: "Senior Technical Recruiter",
        email: "anna@acme.io",
        confidence: "high",
      },
    ],
  },
})
@tag("Jobs")
@route("/jobs/{slug}/recruiter-contacts")
@post
op findRecruiterContacts(/** The job `public_slug`. */ @path slug: string):
  | Data<RecruiterContact[]>
  | {
      @statusCode _: 402;
      @body body: CvPlanRefusal;
    }
  | Problem;
