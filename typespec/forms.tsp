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
  job_search_stage?: string | null;
  biggest_challenges?: string[] | null;

  /** Kept only while `other` is among the challenges. */
  biggest_challenge_note?: string | null;

  current_income_amount?: int64 | null;
  current_income_currency?: string | null;
  current_income_period?: string | null;
}

model OnboardingCompleted {
  onboarding_complete: boolean;
}

/** `data` is null before the first save. */
@tag("Account")
@route("/me/screening-answers")
@get
op getScreeningAnswers(): {
  data: ScreeningAnswers | null;
} | Problem;

@tag("Account")
@route("/me/screening-answers")
@put
op updateScreeningAnswers(@body body: ScreeningAnswersUpdate): Data<ScreeningAnswers> | Problem;

@tag("Account")
@route("/me/survey")
@get
op getSurvey(): Data<SurveyResponses> | Problem;

@tag("Account")
@route("/me/survey")
@put
op updateSurvey(@body body: SurveyResponses): Data<SurveyResponses> | Problem;

@tag("Account")
@route("/me/onboarding/complete")
@post
op completeOnboarding(): Data<OnboardingCompleted> | Problem;
