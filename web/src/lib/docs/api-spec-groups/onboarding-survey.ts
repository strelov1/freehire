import { BASE_URL, type Group } from '../api-spec-types';

export const onboardingSurveyGroup: Group = {
  title: 'Onboarding survey',
  intro:
    'Your stored answers about your job search: how far along it is, the things in ' +
    'its way, and what you earn today. The onboarding tour asks only for the things in ' +
    'its way; the other two are kept from earlier sign-ups. A singleton per ' +
    'user — no id in the path. These answers describe you to us and reach no ' +
    'employer and no job search; what you WANT to be paid is a screening answer ' +
    '(`desired_salary_*`), not one of these. `PUT` is a partial update: a field the ' +
    'body omits keeps its stored value, and there is no way to clear one back to ' +
    'unstated.',
  endpoints: [
    {
      method: 'GET',
      path: '/me/survey',
      auth: 'cookie-or-key',
      summary: 'Your stored survey answers. Every field is absent until you answer it.',
      curl: `curl "${BASE_URL}/me/survey" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{
  "data": {
    "job_search_stage": "searching",
    "biggest_challenges": ["technical_interviews", "visa_sponsorship"],
    "current_income_amount": 5000,
    "current_income_currency": "USD",
    "current_income_period": "month"
  }
}`,
    },
    {
      method: 'PUT',
      path: '/me/survey',
      auth: 'cookie',
      summary: 'Update one or more survey answers.',
      description:
        'A field the body omits is left unchanged. A stage or challenge outside its ' +
        'vocabulary, `just_starting` beside another challenge, a note when the challenges ' +
        'do not include `other`, a currency ' +
        'that is not a three-letter ISO 4217 code, a period outside the vocabulary, ' +
        'or a non-positive income is a `400` naming the offending field.',
      body: [
        {
          name: 'job_search_stage',
          type: 'string',
          description: 'One of `not_started`, `searching`, `employed_looking`, `exploring`.',
          example: 'searching',
        },
        {
          name: 'biggest_challenges',
          type: 'string[]',
          description:
            'Any of `recruiter_contact`, `job_fit`, `remote_flexible`, `working_abroad`, `visa_sponsorship`, `technical_interviews`, `english`, `first_job`, `just_starting`, `other`. `just_starting` stands alone. Replaces the stored list.',
          example: '["technical_interviews"]',
        },
        {
          name: 'biggest_challenge_note',
          type: 'string',
          description: 'Free text. Accepted only while `biggest_challenges` includes `other`.',
        },
        {
          name: 'current_income_amount',
          type: 'integer',
          description: 'What you earn today, in the currency and period below.',
          example: '5000',
        },
        {
          name: 'current_income_currency',
          type: 'string',
          description: 'Three-letter ISO 4217 currency code.',
          example: 'USD',
        },
        {
          name: 'current_income_period',
          type: 'string',
          description: 'Income period, e.g. `year`, `month`.',
          example: 'month',
        },
      ],
      curl: `curl -X PUT "${BASE_URL}/me/survey" \\
  -b cookies.txt -H 'Content-Type: application/json' \\
  -d '{"job_search_stage":"searching"}'`,
      responseExample: `{
  "data": {
    "job_search_stage": "searching"
  }
}`,
    },
    {
      method: 'POST',
      path: '/me/onboarding/complete',
      auth: 'cookie',
      summary: 'Record that you have been through the onboarding wizard.',
      description:
        'Sets `onboarding_completed_at` on your account, which is what stops the app ' +
        'routing you into the wizard again. Idempotent: calling it a second time ' +
        'keeps the original timestamp and still answers `200`.',
      curl: `curl -X POST "${BASE_URL}/me/onboarding/complete" -b cookies.txt`,
      responseExample: `{ "data": { "onboarding_complete": true } }`,
    },
  ],
};
