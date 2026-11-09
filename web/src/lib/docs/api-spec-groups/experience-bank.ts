import { BASE_URL, type Group } from '../api-spec-types';

export const experienceBankGroup: Group = {
  title: 'Experience bank',
  intro:
    'The durable record a CV is built from: employments, and the evidence atoms ' +
    'under them. Every atom carries who asserted it — you, or a model that ' +
    'inferred it — and only your own assertions may be written into a CV. ' +
    'Reading accepts an API key; editing is session-only.',
  endpoints: [
    {
      method: 'GET',
      path: '/me/experience',
      auth: 'cookie-or-key',
      summary: 'Your employments and their evidence atoms.',
      curl: `curl "${BASE_URL}/me/experience" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": { "employments": [ { "id": 12, "company": "Acme", "title": "Senior Backend Engineer", "started_at": "2023-02-01", "ended_at": null, "atoms": [ { "id": 88, "text": "Cut p99 checkout latency 40%", "source": "cv_import" } ] } ] } }`,
    },
    {
      method: 'PUT',
      path: '/me/experience/employments/{id}',
      auth: 'cookie',
      summary: 'Edit one employment.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The employment id.' }],
      curl: `curl -X PUT "${BASE_URL}/me/experience/employments/3fa8…" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"company":"Acme","role":"Staff Engineer"}'`,
      responseExample: `{ "data": { "id": "3fa8…", "kind": "job", "company": "Acme", "role": "Staff Engineer" } }`,
    },
    {
      method: 'DELETE',
      path: '/me/experience/employments/{id}',
      auth: 'cookie',
      summary: 'Remove an employment and its atoms.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The employment id.' }],
      curl: `curl -X DELETE "${BASE_URL}/me/experience/employments/3fa8…" -b cookies.txt`,
    },
    {
      method: 'PUT',
      path: '/me/experience/atoms/{id}',
      auth: 'cookie',
      summary: 'Edit one evidence atom.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The atom id.' }],
      curl: `curl -X PUT "${BASE_URL}/me/experience/atoms/88b1…" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"claim":"Cut p99 checkout latency 40% (12k rps)"}'`,
      responseExample: `{ "data": { "id": "88b1…", "claim": "Cut p99 checkout latency 40% (12k rps)", "provenance": "manual" } }`,
    },
    {
      method: 'DELETE',
      path: '/me/experience/atoms/{id}',
      auth: 'cookie',
      summary: 'Remove an evidence atom.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The atom id.' }],
      curl: `curl -X DELETE "${BASE_URL}/me/experience/atoms/88b1…" -b cookies.txt`,
    },
    {
      method: 'POST',
      path: '/me/experience/employments',
      auth: 'cookie-or-key',
      summary: 'Record a new place — a job or a project.',
      description:
        'An employment carries no claim of its own (only the atoms attached to it ' +
        'do), so nothing about provenance applies here. `kind` is `job` or ' +
        '`project`; a project’s name arrives as `name` on the wire (legacy ' +
        '`company` accepted as a fallback), a job’s as `company`.',
      body: [
        { name: 'kind', type: 'string', required: true, description: '`job` or `project`.', example: 'job' },
        {
          name: 'company',
          type: 'string',
          description: 'The employer (jobs) — or the project name via `name` (see description).',
          example: 'Acme',
        },
        { name: 'role', type: 'string', description: 'Your title.', example: 'Senior Backend Engineer' },
        { name: 'location', type: 'string', description: 'Where you worked.' },
        { name: 'start', type: 'string', description: 'Start date.', example: '2023-02' },
        { name: 'end', type: 'string', description: 'End date; omit with `current`.' },
        { name: 'current', type: 'boolean', description: 'Still ongoing.' },
        { name: 'summary', type: 'string', description: 'A short description of the role.' },
        { name: 'link', type: 'string', description: 'Outbound URL, for a project.' },
        {
          name: 'stack',
          type: 'string[]',
          description: 'Technologies used, printed on the CV’s per-role stack line.',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/me/experience/employments" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" -H 'Content-Type: application/json' \\
  -d '{"kind":"job","company":"Acme","role":"Senior Backend Engineer","start":"2023-02","current":true}'`,
      responseExample: `{ "data": { "id": "3fa8…", "kind": "job", "company": "Acme", "role": "Senior Backend Engineer", "start": "2023-02", "current": true } }`,
    },
    {
      method: 'POST',
      path: '/me/experience/atoms',
      auth: 'cookie-or-key',
      summary: 'Record a new achievement.',
      description:
        'The only route besides the assistant’s own tool that can add evidence ' +
        'outside a chat session, which is why `provenance` is forced to `manual` ' +
        'regardless of what the body sends — an authenticated POST with no chat ' +
        'transcript behind it can only honestly be the owner’s own words. When ' +
        'the account has opted into requiring it, an empty `context` is a 400.',
      body: [
        {
          name: 'claim',
          type: 'string',
          required: true,
          description: 'The sentence a CV bullet would carry.',
          example: 'Cut p99 checkout latency 40%',
        },
        {
          name: 'employment_id',
          type: 'string (uuid)',
          description: 'The place this achievement belongs under; omit to leave it unplaced.',
        },
        {
          name: 'context',
          type: 'string',
          description: 'How it was done — raw material for reframing toward a vacancy.',
        },
        {
          name: 'metrics',
          type: 'string[]',
          description: 'Numbers backing the claim.',
          example: '["12k rps", "40%"]',
        },
        { name: 'skills', type: 'string[]', description: 'Skills this achievement demonstrates.' },
        { name: 'source_ref', type: 'string', description: 'Where this came from, for your own reference.' },
      ],
      curl: `curl -X POST "${BASE_URL}/me/experience/atoms" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" -H 'Content-Type: application/json' \\
  -d '{"claim":"Cut p99 checkout latency 40%","employment_id":"3fa8…","metrics":["40%","12k rps"]}'`,
      responseExample: `{ "data": { "id": "88b1…", "employment_id": "3fa8…", "claim": "Cut p99 checkout latency 40%", "metrics": ["40%", "12k rps"], "provenance": "manual" } }`,
    },
    {
      method: 'POST',
      path: '/me/experience/atoms/merge',
      auth: 'cookie',
      summary: 'Fold two of your atoms into one richer keep.',
      description:
        'Session-only, unlike its sibling create/edit/delete routes — the merge ' +
        'folds the discarded atom’s metrics and skills into the kept one while ' +
        'the kept one’s own provenance stands, letting a `manual` atom absorb ' +
        'numbers a model had inferred, so it cannot be widened to a key the way ' +
        'the others were.',
      body: [
        {
          name: 'ids',
          type: 'string[] (2 uuids)',
          required: true,
          description: 'Exactly two atom ids; the first is kept.',
          example: '["88b1…","91c2…"]',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/me/experience/atoms/merge" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"ids":["88b1…","91c2…"]}'`,
      responseExample: `{ "data": { "id": "88b1…", "claim": "Cut p99 checkout latency 40% (12k rps)", "metrics": ["40%", "12k rps"], "provenance": "manual" } }`,
    },
  ],
};
