import "@typespec/http";

using Http;

namespace Freehire;

enum DirectApplyQuestionType {
  text,
  boolean,
  select,
}

model DirectApplyQuestion {
  id: string;
  label: string;
  type: DirectApplyQuestionType;
  required: boolean;
  options?: string[];
}

model DirectApplyTailoredCV {
  id: string;
  title: string;
}

/** What the signed-in candidate can send. */
model DirectApplyCVOptions {
  /** The CV tailored for this vacancy, null when none. */
  tailored: DirectApplyTailoredCV | null;

  has_resume: boolean;
}

model DirectApplyForm {
  company_name: string;
  questions: DirectApplyQuestion[];
  open: boolean;
  applied: boolean;
  prefill: Record<string>;
  cvs: DirectApplyCVOptions;
}

/** Names the CV to send: `resume: true` for the uploaded one, or `cv_id`, exactly one. */
model DirectApplyRequest {
  /**
   * Send the CV file uploaded to your profile. Exactly one of `resume` and `cv_id`. (e.g.
   * `true`)
   */
  resume?: boolean;
  /**
   * The CV you tailored for THIS vacancy (`cvs.tailored.id` from the form). Any other CV
   * is a `404`. (e.g. `6f1c0c3e-8a51-4c1e-9d55-6a4c1f2b0a01`)
   */
  cv_id?: string;
  /**
   * Question id → answer; a yes/no answer is `"true"` or `"false"`, a list answer one of
   * its options. (e.g. `{"q1":"true"}`)
   */
  answers?: Record<string>;
  /** Up to 2000 characters to the employer. (e.g. `Happy to talk this week.`) */
  note?: string;
}

model DirectApplySent {
  sent: boolean;
}

// The vacancy's questions and, for a signed-in candidate, their prefill. 501 when the
// deployment cannot deliver applications.
/**
 * Only for a posting whose `direct_apply` is true — its employer published it here and
 * receives applications by email. Any other posting is a `404`. Signed in with the
 * browser session, the response also carries `applied`, a `prefill` of your earlier
 * answers to the same questions, and `cvs` — the CV you tailored for this vacancy and
 * whether you uploaded one; an API key never sees those. `open` is false once the vacancy
 * closed. `501` when this deployment cannot deliver applications.
 */
@summary("The application form of a vacancy that takes applications on freehire.")
@useAuth(NoAuth)
@tag("DirectApply")
@route("/jobs/{slug}/direct-apply")
@get
op getDirectApplyForm(/** The job `public_slug`. (e.g. `platform-engineer-acme-9f3c`) */ @path slug: string): Data<DirectApplyForm> | Problem;

// Sends the caller's application. Cookie session only.
/**
 * Needs a verified email. The employer gets the chosen CV as a PDF, the answers, the note
 * and your address as Reply-To. One application per vacancy: `409` once it was sent (or
 * while it is being sent), `400` naming the first unanswered or invalid question, `403`
 * unverified, `410` closed, `429` past the daily limit, `502` when delivery failed —
 * applying again retries it.
 */
@summary("Apply on freehire: the employer receives your CV and answers by email.")
@opExample(#{
  parameters: #{
    slug: "platform-engineer-acme-9f3c",
    body: #{
      cv_id: "6f1c0c3e-8a51-4c1e-9d55-6a4c1f2b0a01",
      answers: #{
        q1: "true",
      },
    },
  },
  returnType: #{
    _: 201,
    body: #{
      data: #{
        sent: true,
      },
    },
  },
})
@tag("DirectApply")
@route("/jobs/{slug}/direct-apply")
@post
op sendDirectApply(/** The job `public_slug`. (e.g. `platform-engineer-acme-9f3c`) */ @path slug: string, @body body: DirectApplyRequest): {
  @statusCode _: 201;
  @body body: Data<DirectApplySent>;
} | Problem;
