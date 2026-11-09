import { BASE_URL, type Group } from '../api-spec-types';

export const moderatorJobsGroup: Group = {
  title: 'Moderator jobs',
  intro:
    'Hand-curate a vacancy directly (moderators only). Approved submissions go ' +
    'through the same minting path, so a curated job is indistinguishable from ' +
    'an approved one.',
  endpoints: [
    {
      method: 'POST',
      path: '/jobs',
      auth: 'moderator',
      summary: 'Create a curated job.',
      body: [
        {
          name: 'url',
          type: 'string',
          required: true,
          description: 'Link to the posting.',
          example: 'https://acme.com/careers/123',
        },
        { name: 'source', type: 'string', description: 'Source label.', example: 'manual' },
        { name: 'title', type: 'string', required: true, description: 'Job title.', example: 'Senior Go Engineer' },
        { name: 'company', type: 'string', required: true, description: 'Company name.', example: 'Acme' },
        { name: 'location', type: 'string', description: 'Free-text location.' },
        { name: 'remote', type: 'boolean', description: 'Whether the role is remote.' },
        { name: 'description', type: 'string', description: 'Job description.' },
        { name: 'posted_at', type: 'string (RFC3339)', description: 'Posting date.' },
      ],
      curl: `curl -X POST "${BASE_URL}/jobs" \\
  -b cookies.txt -H 'Content-Type: application/json' \\
  -d '{"url":"https://acme.com/careers/123","title":"Senior Go Engineer","company":"Acme"}'`,
      responseExample: `{ "data": { "public_slug": "senior-go-engineer-acme-1a2b", "title": "Senior Go Engineer", "manually_added": true } }`,
    },
    {
      method: 'PATCH',
      path: '/jobs/{slug}',
      auth: 'moderator',
      summary: 'Edit a curated job.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      body: [
        {
          name: '(any job field)',
          type: 'varies',
          description: 'Same fields as create; provided fields are updated.',
        },
      ],
      curl: `curl -X PATCH "${BASE_URL}/jobs/<slug>" \\
  -b cookies.txt -H 'Content-Type: application/json' \\
  -d '{"title":"Staff Go Engineer"}'`,
      responseExample: `{ "data": { "public_slug": "...", "title": "Staff Go Engineer" } }`,
    },
  ],
};
