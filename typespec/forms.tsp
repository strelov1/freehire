import "@typespec/http";

using Http;

namespace Freehire;

// Two saved forms of a signed-in account: the standing answers to screening questions an
// application form asks, and the onboarding survey. A save is a patch: an absent field
// keeps its value, and `null` reads the same as absent. Readable with a session or an API
// key; written with the session cookie only.

/** Answers an application form asks for, saved once and reused. Each field is optional. */
model ScreeningAnswers {
  /** ISO 3166-1 alpha-2 codes the candidate may work in. */
  authorized_countries?: string[] | null;

  visa_sponsorship_needed?: boolean | null;
  desired_salary_amount?: int64 | null;

  /** ISO 4217, upper case. */
  desired_salary_currency?: string | null;

  desired_salary_period?: string | null;
  notice_period_days?: int64 | null;
  willing_to_relocate?: boolean | null;
  age_18_or_older?: boolean | null;

  /** A standing agreement to an employer form's privacy terms; false asks each time. */
  privacy_consent?: boolean | null;
}

model ScreeningAnswersUpdate {
  ...ScreeningAnswers;

  /** Field names to clear. An unknown name is a 400. */
  clear?: string[] | null;
}

/** The onboarding survey. Each field is optional. */
model SurveyResponses {
  /**
   * One of `not_started`, `searching`, `employed_looking`, `exploring`. (e.g.
   * `searching`)
   */
  job_search_stage?: string | null;
  /**
   * Any of `recruiter_contact`, `job_fit`, `remote_flexible`, `working_abroad`,
   * `visa_sponsorship`, `technical_interviews`, `english`, `first_job`, `just_starting`,
   * `other`. `just_starting` stands alone. Replaces the stored list. (e.g.
   * `["technical_interviews"]`)
   */
  biggest_challenges?: string[] | null;

  /** Kept only while `other` is among the challenges. */
  biggest_challenge_note?: string | null;

  /** What you earn today, in the currency and period below. (e.g. `5000`) */
  current_income_amount?: int64 | null;
  /** Three-letter ISO 4217 currency code. (e.g. `USD`) */
  current_income_currency?: string | null;
  /** Income period, e.g. `year`, `month`. (e.g. `month`) */
  current_income_period?: string | null;
}

model OnboardingCompleted {
  onboarding_complete: boolean;
}

/** `data` is null before the first save. */
@summary("Your stored screening answers, or `null` if you have stated none.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      authorized_countries: #["DE", "NL"],
      visa_sponsorship_needed: false,
      desired_salary_amount: 90000,
      desired_salary_currency: "EUR",
      desired_salary_period: "year",
      notice_period_days: 30,
      willing_to_relocate: true,
      age_18_or_older: true,
      privacy_consent: true,
    },
  },
})
@tag("Account")
@route("/me/screening-answers")
@get
op getScreeningAnswers(): {
  data: ScreeningAnswers | null;
} | Problem;

/**
 * A field the body omits is left unchanged; to withdraw an answer, name it in `clear`. An
 * unrecognized country code, a currency that is not a three-letter ISO 4217 code, a
 * period outside the vocabulary, a non-positive salary, a negative notice period, an
 * unknown name in `clear`, or a field both given and cleared is a `400` naming it.
 */
@summary("Update one or more screening answers.")
@opExample(#{
  parameters: #{
    body: #{
      willing_to_relocate: true,
      notice_period_days: 30,
    },
  },
  returnType: #{
    data: #{
      authorized_countries: #["DE", "NL"],
      visa_sponsorship_needed: false,
      desired_salary_amount: 90000,
      desired_salary_currency: "EUR",
      desired_salary_period: "year",
      notice_period_days: 30,
      willing_to_relocate: true,
      age_18_or_older: true,
      privacy_consent: true,
    },
  },
})
@tag("Account")
@route("/me/screening-answers")
@put
op updateScreeningAnswers(@body body: ScreeningAnswersUpdate): Data<ScreeningAnswers> | Problem;

@summary("Your stored survey answers. Every field is absent until you answer it.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      job_search_stage: "searching",
      biggest_challenges: #["technical_interviews", "visa_sponsorship"],
      current_income_amount: 5000,
      current_income_currency: "USD",
      current_income_period: "month",
    },
  },
})
@tag("Account")
@route("/me/survey")
@get
op getSurvey(): Data<SurveyResponses> | Problem;

/**
 * A field the body omits is left unchanged. A stage or challenge outside its vocabulary,
 * `just_starting` beside another challenge, a note when the challenges do not include
 * `other`, a currency that is not a three-letter ISO 4217 code, a period outside the
 * vocabulary, or a non-positive income is a `400` naming the offending field.
 */
@summary("Update one or more survey answers.")
@opExample(#{
  parameters: #{
    body: #{
      job_search_stage: "searching",
    },
  },
  returnType: #{
    data: #{
      job_search_stage: "searching",
    },
  },
})
@tag("Account")
@route("/me/survey")
@put
op updateSurvey(@body body: SurveyResponses): Data<SurveyResponses> | Problem;

/**
 * Sets `onboarding_completed_at` on your account, which is what stops the app routing you
 * into the wizard again. Idempotent: calling it a second time keeps the original
 * timestamp and still answers `200`.
 */
@summary("Record that you have been through the onboarding wizard.")
@opExample(#{
  returnType: #{
    data: #{
      onboarding_complete: true,
    },
  },
})
@tag("Account")
@route("/me/onboarding/complete")
@post
op completeOnboarding(): Data<OnboardingCompleted> | Problem;
