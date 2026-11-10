import { BASE_URL, type Group } from '../api-spec-types';

export const jobSubmissionsGroup: Group = {
  title: 'Job submissions',
  intro:
    'Any signed-in user can submit a vacancy for moderation and read their own ' +
    'queue. The review actions are moderator-only; approval mints a live job.',
  endpoints: [
    {
      method: 'POST',
      path: '/submissions',
      auth: 'cookie-or-key',
      summary: 'Submit a vacancy for review.',
      description:
        'A URL already awaiting review is a `409`, and so is a URL the catalogue ' +
        'already carries — matched the way `/jobs/find` matches it, so a link differing ' +
        'only by scheme, `www.`, tracking query or trailing slash counts as the same ' +
        'posting. That `409` names the existing job (`/jobs/<public_slug>`) in its error.',
      body: [
        {
          name: 'url',
          type: 'string',
          required: true,
          description: 'Link to the original posting.',
          example: 'https://acme.com/careers/123',
        },
        { name: 'title', type: 'string', required: true, description: 'Job title.', example: 'Senior Go Engineer' },
        { name: 'company', type: 'string', required: true, description: 'Company name.', example: 'Acme' },
        { name: 'location', type: 'string', description: 'Free-text location.', example: 'Remote — EU' },
        { name: 'remote', type: 'boolean', description: 'Whether the role is remote.', example: 'true' },
        { name: 'description', type: 'string', description: 'Job description.' },
        { name: 'source', type: 'string', description: 'Origin hint (optional).' },
        { name: 'posted_at', type: 'string (RFC3339)', description: 'Original posting date (optional).' },
      ],
      curl: `curl -X POST "${BASE_URL}/submissions" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" \\
  -H 'Content-Type: application/json' \\
  -d '{"url":"https://acme.com/careers/123","title":"Senior Go Engineer","company":"Acme","remote":true}'`,
      responseExample: `{ "data": { "id": 9, "status": "pending", "title": "Senior Go Engineer", "company": "Acme", "url": "https://acme.com/careers/123" } }`,
    },
    {
      method: 'POST',
      path: '/submissions/prefill',
      auth: 'cookie-or-key',
      summary: 'Parse a job URL into a draft submission for review before posting.',
      description:
        'Uses the same ATS-recognition registry as `/jobs/resolve`, but ' +
        'writes nothing — no job, no submission, no allowance spent. An unrecognized ' +
        'URL returns an empty object rather than an error. A URL the catalogue already ' +
        'carries returns only `existing_job_slug`, and `POST /submissions` would refuse ' +
        'it.',
      body: [{ name: 'url', type: 'string', required: true, description: 'The job posting URL to parse.' }],
      curl: `curl -X POST "${BASE_URL}/submissions/prefill" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" -H 'Content-Type: application/json' \\
  -d '{"url":"https://boards.greenhouse.io/acme/jobs/123"}'`,
      responseExample: `{ "data": { "title": "Senior Go Engineer", "company": "Acme", "location": "Remote — EU", "description": "...", "work_mode": "remote", "employment_type": "full_time", "seniority": "senior", "skills": ["go"], "source": "greenhouse" } }`,
    },
    {
      method: 'GET',
      path: '/me/submissions',
      auth: 'cookie-or-key',
      summary: 'Your own submission queue.',
      curl: `curl "${BASE_URL}/me/submissions" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": [ { "id": 9, "status": "pending", "title": "Senior Go Engineer" } ] }`,
    },
    {
      method: 'GET',
      path: '/submissions',
      auth: 'moderator',
      summary: 'The pending submission queue (moderators).',
      curl: `curl "${BASE_URL}/submissions" -b cookies.txt`,
      responseExample: `{ "data": [ { "id": 9, "status": "pending", "submitter_email": "me@example.com" } ] }`,
    },
    {
      method: 'POST',
      path: '/submissions/{id}/approve',
      auth: 'moderator',
      summary: 'Approve a submission, minting a live job.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The submission id.', example: '9' }],
      curl: `curl -X POST "${BASE_URL}/submissions/9/approve" -b cookies.txt`,
      responseExample: `{ "data": { "id": 9, "status": "approved", "job_slug": "senior-go-engineer-acme-1a2b" } }`,
    },
    {
      method: 'POST',
      path: '/submissions/{id}/reject',
      auth: 'moderator',
      summary: 'Reject a submission with a reason.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The submission id.', example: '9' }],
      body: [{ name: 'reason', type: 'string', description: 'Why it was rejected.', example: 'duplicate' }],
      curl: `curl -X POST "${BASE_URL}/submissions/9/reject" \\
  -b cookies.txt -H 'Content-Type: application/json' \\
  -d '{"reason":"duplicate"}'`,
      responseExample: `{ "data": { "id": 9, "status": "rejected", "review_reason": "duplicate" } }`,
    },
  ],
};
