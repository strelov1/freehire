import { BASE_URL, type Group } from '../api-spec-types';

export const screeningAnswersGroup: Group = {
  title: 'Screening answers',
  intro:
    'The six candidate-stated facts that repeat across ATS application forms ' +
    'and no CV can supply. A singleton per user — no id in the path. `PUT` is ' +
    'a partial update: a field the body omits keeps its stored value.',
  endpoints: [
    {
      method: 'GET',
      path: '/me/screening-answers',
      auth: 'cookie-or-key',
      summary: 'Your stored screening answers, or `null` if you have stated none.',
      curl: `curl "${BASE_URL}/me/screening-answers" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{
  "data": {
    "authorized_countries": ["DE", "NL"],
    "visa_sponsorship_needed": false,
    "desired_salary_amount": 90000,
    "desired_salary_currency": "EUR",
    "desired_salary_period": "year",
    "notice_period_days": 30,
    "willing_to_relocate": true,
    "age_18_or_older": true,
    "privacy_consent": true
  }
}`,
    },
    {
      method: 'PUT',
      path: '/me/screening-answers',
      auth: 'cookie',
      summary: 'Update one or more screening answers.',
      description:
        'A field the body omits is left unchanged; to withdraw an answer, name it in ' +
        '`clear`. An unrecognized country code, a currency that is not a three-letter ' +
        'ISO 4217 code, a period outside the vocabulary, a non-positive salary, a ' +
        'negative notice period, an unknown name in `clear`, or a field both given and ' +
        'cleared is a `400` naming it.',
      body: [
        {
          name: 'authorized_countries',
          type: 'string[]',
          description: 'ISO alpha-2 country codes you are authorized to work in.',
          example: '["DE","NL"]',
        },
        { name: 'visa_sponsorship_needed', type: 'boolean', description: 'Whether you need visa sponsorship.' },
        { name: 'desired_salary_amount', type: 'integer', description: 'Desired salary figure.', example: '90000' },
        {
          name: 'desired_salary_currency',
          type: 'string',
          description: 'Three-letter ISO 4217 currency code.',
          example: 'EUR',
        },
        {
          name: 'desired_salary_period',
          type: 'string',
          description: 'Salary period, e.g. `year`, `month`.',
          example: 'year',
        },
        { name: 'notice_period_days', type: 'integer', description: 'Notice period in days.', example: '30' },
        { name: 'willing_to_relocate', type: 'boolean', description: 'Whether you are willing to relocate.' },
        { name: 'age_18_or_older', type: 'boolean', description: 'Whether you are 18 or older.' },
        {
          name: 'privacy_consent',
          type: 'boolean',
          description:
            "Whether auto-apply may accept an employer's privacy / personal-data terms for you. Never covers marketing, SMS, future-openings or diversity opt-ins.",
        },
        {
          name: 'clear',
          type: 'string[]',
          description:
            'Answers to withdraw back to unstated, by field name. Auto-apply then asks you instead of answering.',
          example: '["desired_salary_amount"]',
        },
      ],
      curl: `curl -X PUT "${BASE_URL}/me/screening-answers" \\
  -b cookies.txt -H 'Content-Type: application/json' \\
  -d '{"willing_to_relocate":true,"notice_period_days":30}'`,
      responseExample: `{
  "data": {
    "authorized_countries": ["DE", "NL"],
    "visa_sponsorship_needed": false,
    "desired_salary_amount": 90000,
    "desired_salary_currency": "EUR",
    "desired_salary_period": "year",
    "notice_period_days": 30,
    "willing_to_relocate": true,
    "age_18_or_older": true,
    "privacy_consent": true
  }
}`,
    },
  ],
};
