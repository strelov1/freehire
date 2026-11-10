import { BASE_URL, type Group } from '../api-spec-types';

export const jobReportsGroup: Group = {
  title: 'Job reports',
  intro:
    'Any signed-in user can flag a problem with a live vacancy. Review actions ' +
    'are moderator-only; resolving may soft-close the reported job.',
  endpoints: [
    {
      method: 'POST',
      path: '/jobs/{slug}/reports',
      auth: 'cookie-or-key',
      summary: 'Report a problem with a job.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      body: [
        {
          name: 'reason',
          type: 'string',
          required: true,
          description: 'Short reason code/category.',
          example: 'expired',
        },
        { name: 'details', type: 'string', description: 'Free-text details.' },
        { name: 'contact_telegram', type: 'string', description: 'Optional contact handle.' },
      ],
      curl: `curl -X POST "${BASE_URL}/jobs/<slug>/reports" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" \\
  -H 'Content-Type: application/json' \\
  -d '{"reason":"expired","details":"posting returns 404"}'`,
      responseExample: `{ "data": { "id": 3, "status": "pending", "reason": "expired" } }`,
    },
    {
      method: 'GET',
      path: '/reports',
      auth: 'moderator',
      summary: 'The pending report queue (moderators).',
      curl: `curl "${BASE_URL}/reports" -b cookies.txt`,
      responseExample: `{ "data": [ { "id": 3, "status": "pending", "job_slug": "...", "job_title": "..." } ] }`,
    },
    {
      method: 'POST',
      path: '/reports/{id}/resolve',
      auth: 'moderator',
      summary: 'Resolve a report, optionally closing the job and telling the reporter.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The report id.', example: '3' }],
      body: [
        { name: 'close_job', type: 'boolean', description: 'Soft-close the reported job.', example: 'true' },
        {
          name: 'note',
          type: 'string',
          description:
            'What you did about the report. Emailed to the reporter verbatim when notify_reporter is set, and stored as the review reason either way.',
          example: 'Fixed — the job is now marked hybrid',
        },
        {
          name: 'notify_reporter',
          type: 'boolean',
          description: 'Email the reporter this decision. Defaults to false when omitted.',
          example: 'true',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/reports/3/resolve" \\
  -b cookies.txt -H 'Content-Type: application/json' \\
  -d '{"close_job":true,"note":"Fixed — the job is now marked hybrid","notify_reporter":true}'`,
      responseExample: `{ "data": { "id": 3, "status": "resolved", "notified": true } }`,
    },
    {
      method: 'POST',
      path: '/reports/{id}/dismiss',
      auth: 'moderator',
      summary: 'Dismiss a report with a reason, optionally telling the reporter.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The report id.', example: '3' }],
      body: [
        {
          name: 'reason',
          type: 'string',
          description: 'Why nothing changed. Emailed to the reporter verbatim when notify_reporter is set.',
          example: 'not an issue',
        },
        {
          name: 'notify_reporter',
          type: 'boolean',
          description: 'Email the reporter this decision. Defaults to false when omitted.',
          example: 'true',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/reports/3/dismiss" \\
  -b cookies.txt -H 'Content-Type: application/json' \\
  -d '{"reason":"not an issue"}'`,
      responseExample: `{ "data": { "id": 3, "status": "dismissed", "review_reason": "not an issue" } }`,
    },
  ],
};
