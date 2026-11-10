import { BASE_URL, type Group } from '../api-spec-types';

export const companyFeedbackGroup: Group = {
  title: 'Company feedback',
  intro:
    'Signed-in users leave a 1–5 star rating plus category and text about a ' +
    'company, shown under their site-wide pseudonymous persona (never a user ' +
    'id). Reads are public; writes are cookie-only. A reader can report a ' +
    'specific review, and a moderator can hide it.',
  endpoints: [
    {
      method: 'GET',
      path: '/companies/{slug}/feedback',
      auth: 'none',
      summary: "List a company's feedback, newest first.",
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The company slug.', example: 'acme' }],
      query: [
        { name: 'limit', type: 'integer', description: 'Page size, 1–100.', example: '20' },
        { name: 'offset', type: 'integer', description: 'Rows to skip.', example: '0' },
      ],
      curl: `curl "${BASE_URL}/companies/acme/feedback"`,
      responseExample: `{
  "data": [
    {
      "id": 5,
      "author": "quiet-falcon-42",
      "rating": 4,
      "feedback_type": "interview",
      "body": "Fast process, clear communication.",
      "created_at": "2026-06-18T09:12:00Z",
      "updated_at": "2026-06-18T09:12:00Z"
    }
  ],
  "meta": { "total": 12, "limit": 20, "offset": 0 }
}`,
    },
    {
      method: 'GET',
      path: '/companies/{slug}/feedback/mine',
      auth: 'cookie',
      summary: 'Your own feedback on this company, across every category (empty if none).',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The company slug.', example: 'acme' }],
      curl: `curl "${BASE_URL}/companies/acme/feedback/mine" -b cookies.txt`,
      responseExample: `{ "data": [ { "id": 5, "author": "quiet-falcon-42", "rating": 4, "feedback_type": "interview", "body": "...", "created_at": "...", "updated_at": "..." } ] }`,
    },
    {
      method: 'POST',
      path: '/companies/{slug}/feedback',
      auth: 'cookie',
      summary: 'Create or overwrite your feedback in one category on a company.',
      description:
        'One entry per (user, category): posting again in the same ' +
        '`feedback_type` overwrites your existing review rather than adding a ' +
        "second one. The response's nested `company` field is the freshly " +
        'recomputed rating count/average, so you can update your own view of ' +
        'the company without a second fetch. `422` on an empty body.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The company slug.', example: 'acme' }],
      body: [
        { name: 'rating', type: 'integer', required: true, description: '1–5 stars.', example: '4' },
        {
          name: 'feedback_type',
          type: 'string',
          required: true,
          description: 'Review category, e.g. `interview`, `culture`, `compensation`.',
          example: 'interview',
        },
        {
          name: 'body',
          type: 'string',
          required: true,
          description: 'Free-text review.',
          example: 'Fast process, clear communication.',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/companies/acme/feedback" \\
  -b cookies.txt -H 'Content-Type: application/json' \\
  -d '{"rating":4,"feedback_type":"interview","body":"Fast process, clear communication."}'`,
      responseExample: `{
  "data": {
    "id": 5,
    "author": "quiet-falcon-42",
    "rating": 4,
    "feedback_type": "interview",
    "body": "Fast process, clear communication.",
    "created_at": "2026-06-18T09:12:00Z",
    "updated_at": "2026-06-18T09:12:00Z",
    "company": { "feedback_count": 12, "feedback_rating_avg": 4.1 }
  }
}`,
    },
    {
      method: 'DELETE',
      path: '/companies/{slug}/feedback',
      auth: 'cookie',
      summary: 'Delete your feedback in one category (no-op if absent).',
      description:
        "Returns the company's freshly recomputed counters, the same cast/clear shape as the vote endpoints.",
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The company slug.', example: 'acme' }],
      query: [
        {
          name: 'feedback_type',
          type: 'string',
          required: true,
          description: 'Which category to delete.',
          example: 'interview',
        },
      ],
      curl: `curl -X DELETE "${BASE_URL}/companies/acme/feedback?feedback_type=interview" -b cookies.txt`,
      responseExample: `{ "data": { "feedback_count": 11, "feedback_rating_avg": 4.0 } }`,
    },
    {
      method: 'POST',
      path: '/company-feedback/{id}/report',
      auth: 'cookie',
      summary: 'Report a specific review.',
      description: 'A second report of the same review by you is a silent no-op.',
      pathParams: [
        { name: 'id', type: 'integer', required: true, description: 'The feedback entry id.', example: '5' },
      ],
      body: [{ name: 'reason', type: 'string', required: true, description: 'Report reason code.', example: 'spam' }],
      curl: `curl -X POST "${BASE_URL}/company-feedback/5/report" \\
  -b cookies.txt -H 'Content-Type: application/json' \\
  -d '{"reason":"spam"}'`,
      responseExample: `(204 No Content)`,
    },
    {
      method: 'GET',
      path: '/company-feedback/reported',
      auth: 'moderator',
      summary: 'Every review with at least one report, most-reported first.',
      curl: `curl "${BASE_URL}/company-feedback/reported" -b cookies.txt`,
      responseExample: `{
  "data": [
    {
      "id": 5,
      "author": "quiet-falcon-42",
      "rating": 1,
      "feedback_type": "culture",
      "body": "...",
      "created_at": "...",
      "updated_at": "...",
      "company_slug": "acme",
      "report_count": 3,
      "report_reasons": ["spam", "off-topic"]
    }
  ]
}`,
    },
    {
      method: 'POST',
      path: '/company-feedback/{id}/hide',
      auth: 'moderator',
      summary: "Hide a review, dropping it from the company's public list and average.",
      description: 'Idempotent. 404 for an unknown id.',
      pathParams: [
        { name: 'id', type: 'integer', required: true, description: 'The feedback entry id.', example: '5' },
      ],
      curl: `curl -X POST "${BASE_URL}/company-feedback/5/hide" -b cookies.txt`,
      responseExample: `(204 No Content)`,
    },
  ],
};
