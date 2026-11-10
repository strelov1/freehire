import { BASE_URL, type Group } from '../api-spec-types';

export const ghostJobReportsGroup: Group = {
  title: 'Ghost job reports',
  intro:
    'One person states they applied to a posting and were never answered, ' +
    'feeding the ghost-signal evidence used elsewhere in the API (see the ' +
    '`reality` field on a job). Distinct from Job reports above: nothing here ' +
    'reaches a moderator and nothing here closes the job directly.',
  endpoints: [
    {
      method: 'POST',
      path: '/jobs/{slug}/ghost-report',
      auth: 'cookie-or-key',
      summary: 'File a claim that you applied and got no response.',
      description:
        'An unproven (unverified) address is a `403`; a closed job or a claim ' +
        'you already have on this job is a `409`.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      body: [
        {
          name: 'applied_on',
          type: 'string',
          required: true,
          description: 'The day you applied (`YYYY-MM-DD`).',
          example: '2026-07-29',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/jobs/<slug>/ghost-report" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" -H 'Content-Type: application/json' \\
  -d '{"applied_on":"2026-07-29"}'`,
      responseExample: `{ "data": { "job_id": 42, "applied_on": "2026-07-29", "created_at": "2026-07-29T10:00:00Z" } }`,
    },
    {
      method: 'DELETE',
      path: '/jobs/{slug}/ghost-report',
      auth: 'cookie-or-key',
      summary: 'Withdraw your claim about this job.',
      description: 'A claim that is absent or already withdrawn is a `404`.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      curl: `curl -X DELETE "${BASE_URL}/jobs/<slug>/ghost-report" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `(204 No Content)`,
    },
  ],
};
