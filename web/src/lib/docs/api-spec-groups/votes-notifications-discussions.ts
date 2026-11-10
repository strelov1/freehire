import { BASE_URL, type Group } from '../api-spec-types';

export const votesNotificationsDiscussionsGroup: Group = {
  title: 'Votes, notifications & discussions',
  intro:
    'The lighter per-user surfaces: a vote on a job or company, the account-level ' +
    'notification rule (gates the saved-job apply reminder and both lifecycle nudges), ' +
    'and the public discussion threads. Votes and notification settings take a key; ' +
    'posting to a thread is browser-owned.',
  endpoints: [
    {
      method: 'POST',
      path: '/jobs/{slug}/vote',
      auth: 'cookie-or-key',
      summary: 'Vote on a job.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      body: [{ name: 'value', type: 'integer', required: true, description: '`1` or `-1`.' }],
      curl: `curl -X POST "${BASE_URL}/jobs/senior-backend-engineer-acme-1a2b/vote" \\
  -H "Authorization: Bearer fhk_…" -H 'Content-Type: application/json' -d '{"value":1}'`,
      responseExample: `{ "data": { "score": 14, "my_vote": 1 } }`,
    },
    {
      method: 'DELETE',
      path: '/jobs/{slug}/vote',
      auth: 'cookie-or-key',
      summary: 'Clear your vote on a job.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      curl: `curl -X DELETE "${BASE_URL}/jobs/senior-backend-engineer-acme-1a2b/vote" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": { "score": 13, "my_vote": 0 } }`,
    },
    {
      method: 'POST',
      path: '/companies/{slug}/vote',
      auth: 'cookie-or-key',
      summary: 'Vote on a company.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The company slug.' }],
      body: [{ name: 'value', type: 'integer', required: true, description: '`1` or `-1`.' }],
      curl: `curl -X POST "${BASE_URL}/companies/acme/vote" \\
  -H "Authorization: Bearer fhk_…" -H 'Content-Type: application/json' -d '{"value":1}'`,
      responseExample: `{ "data": { "score": 42, "my_vote": 1 } }`,
    },
    {
      method: 'DELETE',
      path: '/companies/{slug}/vote',
      auth: 'cookie-or-key',
      summary: 'Clear your vote on a company.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The company slug.' }],
      curl: `curl -X DELETE "${BASE_URL}/companies/acme/vote" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": { "score": 41, "my_vote": 0 } }`,
    },
    {
      method: 'GET',
      path: '/me/notification-settings',
      auth: 'cookie',
      summary: 'Your notification rule (gates saved-job reminders and both lifecycle nudges).',
      curl: `curl "${BASE_URL}/me/notification-settings" -b cookies.txt`,
      responseExample: `{ "data": { "enabled": true, "channels": ["email"] } }`,
    },
    {
      method: 'PUT',
      path: '/me/notification-settings',
      auth: 'cookie',
      summary: 'Change your notification rule.',
      body: [
        { name: 'enabled', type: 'boolean', description: 'Turn notifications on or off.' },
        {
          name: 'channels',
          type: 'string[]',
          description:
            'Delivery channels: `email`, `telegram`, `push`. Adding `push` needs a device the mobile app has registered (409 otherwise).',
        },
      ],
      curl: `curl -X PUT "${BASE_URL}/me/notification-settings" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"enabled":true,"channels":["email"]}'`,
      responseExample: `{ "data": { "enabled": true, "channels": ["email"] } }`,
    },
    {
      method: 'GET',
      path: '/threads',
      auth: 'none',
      summary: 'Public discussion threads.',
      query: [
        { name: 'limit', type: 'integer', description: 'Page size.' },
        { name: 'offset', type: 'integer', description: 'Page offset.' },
      ],
      curl: `curl "${BASE_URL}/threads?limit=20"`,
      responseExample: `{ "data": [ { "id": 31, "title": "Anyone interviewed at Acme lately?", "replies": 12, "created_at": "2026-07-25T10:00:00Z" } ], "meta": { "total": 214 } }`,
    },
    {
      method: 'GET',
      path: '/threads/count',
      auth: 'none',
      summary: 'How many threads there are.',
      curl: `curl "${BASE_URL}/threads/count"`,
      responseExample: `{ "data": { "count": 214 } }`,
    },
    {
      method: 'GET',
      path: '/threads/{id}',
      auth: 'none',
      summary: 'One thread with its replies.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The thread id.' }],
      curl: `curl "${BASE_URL}/threads/31"`,
      responseExample: `{ "data": { "id": 31, "title": "Anyone interviewed at Acme lately?", "body": "…", "replies": [ { "id": 88, "body": "…", "created_at": "2026-07-25T12:00:00Z" } ] } }`,
    },
    {
      method: 'POST',
      path: '/threads',
      auth: 'cookie',
      summary: 'Start a thread.',
      body: [
        { name: 'title', type: 'string', required: true, description: 'Thread title.' },
        { name: 'body', type: 'string', required: true, description: 'Opening post.' },
      ],
      curl: `curl -X POST "${BASE_URL}/threads" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"title":"Anyone interviewed at Acme lately?","body":"…"}'`,
      responseExample: `{ "data": { "id": 31 } }`,
    },
    {
      method: 'POST',
      path: '/threads/{id}/replies',
      auth: 'cookie',
      summary: 'Reply to a thread.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The thread id.' }],
      body: [{ name: 'body', type: 'string', required: true, description: 'Your reply.' }],
      curl: `curl -X POST "${BASE_URL}/threads/31/replies" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"body":"Two rounds, both technical."}'`,
      responseExample: `{ "data": { "id": 88 } }`,
    },
    {
      method: 'POST',
      path: '/threads/{id}/close',
      auth: 'moderator',
      summary: 'Close a thread.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The thread id.' }],
      curl: `curl -X POST "${BASE_URL}/threads/31/close" -b cookies.txt`,
      responseExample: `{ "data": { "id": 31, "closed": true } }`,
    },
  ],
};
