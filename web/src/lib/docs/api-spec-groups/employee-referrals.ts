import { BASE_URL, type Group } from '../api-spec-types';

export const employeeReferralsGroup: Group = {
  title: 'Employee referrals',
  intro:
    'Two sides of the same marketplace: an employee offers to refer into the ' +
    'company they work at (proof required, moderated), and a candidate asks one ' +
    "of that company's approved referrers for an intro. A candidate never sees " +
    "who the referrer is, and a referrer sees the candidate's CV only through a " +
    'short-lived view. Moderation of offers is moderator-gated.',
  endpoints: [
    {
      method: 'POST',
      path: '/me/referrals/offers',
      auth: 'cookie-or-key',
      summary: 'Offer to refer into a company. Multipart — proof required.',
      description:
        'The proof (an offer letter, badge, anything showing you work there) is ' +
        'stored privately and shown only to a moderator; its storage key is never ' +
        'exposed on the wire. The offer is pending until decided.',
      body: [
        {
          name: 'company_slug',
          type: 'string (form field)',
          required: true,
          description: 'The company you can refer into.',
        },
        {
          name: 'linkedin_url',
          type: 'string (form field)',
          description: 'Your profile, for the moderator to check against.',
        },
        { name: 'proof', type: 'file (form field)', required: true, description: 'Evidence you work there.' },
      ],
      curl: `curl -X POST "${BASE_URL}/me/referrals/offers" \\
  -H "Authorization: Bearer fhk_…" \\
  -F company_slug=acme -F linkedin_url=https://linkedin.com/in/you -F proof=@badge.pdf`,
      responseExample: `{ "data": { "id": "b71e…", "company_slug": "acme", "company_name": "Acme", "status": "pending", "created_at": "2026-07-28T20:00:00Z" } }`,
    },
    {
      method: 'GET',
      path: '/me/referrals/offers',
      auth: 'cookie-or-key',
      summary: 'Your referral offers and their moderation state.',
      curl: `curl "${BASE_URL}/me/referrals/offers" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": [ { "id": "b71e…", "company_slug": "acme", "status": "approved", "decided_at": "2026-07-28T21:00:00Z" } ] }`,
    },
    {
      method: 'DELETE',
      path: '/me/referrals/offers/{id}',
      auth: 'cookie-or-key',
      summary: 'Withdraw an offer.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The offer id.' }],
      curl: `curl -X DELETE "${BASE_URL}/me/referrals/offers/b71e…" -H "Authorization: Bearer fhk_…"`,
    },
    {
      method: 'POST',
      path: '/me/referrals/requests',
      auth: 'cookie-or-key',
      summary: "Ask a company's approved referrers for an intro.",
      description:
        'Validation failures answer 422; a company with no approved referrer ' +
        'answers 409; too many open requests answers 429.',
      body: [
        { name: 'company_slug', type: 'string', required: true, description: 'The company to ask.' },
        {
          name: 'job_slug',
          type: 'string',
          description:
            'The opening the request is made from, when there is one (`job_id` is still read for older clients).',
        },
        {
          name: 'cv_kind',
          type: 'string',
          description: 'Which CV to attach: `original` (your uploaded one) or `built`.',
        },
        {
          name: 'cv_id',
          type: 'string (uuid)',
          description:
            'The built CV, when `cv_kind` is `built`: one you tailored for that opening, or — with no opening — for any opening at the company (see `cv-options`). Any other CV answers 422.',
        },
        { name: 'linkedin_url', type: 'string', required: true, description: 'Your LinkedIn profile URL.' },
        { name: 'contact_telegram', type: 'string', description: 'How the referrer reaches you.' },
        { name: 'contact_email', type: 'string', description: 'How the referrer reaches you.' },
        { name: 'note', type: 'string', description: 'A short message to the referrer.' },
      ],
      curl: `curl -X POST "${BASE_URL}/me/referrals/requests" \\
  -H "Authorization: Bearer fhk_…" -H 'Content-Type: application/json' \\
  -d '{"company_slug":"acme","cv_kind":"original","linkedin_url":"https://www.linkedin.com/in/you","contact_email":"you@example.com"}'`,
      responseExample: `{ "data": { "id": "c92f…", "company_slug": "acme", "status": "open", "created_at": "2026-07-28T20:05:00Z" } }`,
    },
    {
      method: 'GET',
      path: '/me/referrals/cv-options',
      auth: 'cookie-or-key',
      summary: 'The CVs you can attach to a request into a company.',
      description:
        'The CVs you tailored for `job_slug`, or with no `job_slug` for any opening at the ' +
        'company, newest first, and whether you have uploaded a CV file.',
      query: [
        { name: 'company_slug', type: 'string', required: true, description: 'The company to ask.' },
        { name: 'job_slug', type: 'string', description: 'The opening the request is made from.' },
      ],
      curl: `curl "${BASE_URL}/me/referrals/cv-options?company_slug=acme" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": { "tailored": [ { "id": "6f1c…", "title": "Tailored — Backend Engineer" } ], "has_resume": true } }`,
    },
    {
      method: 'GET',
      path: '/me/referrals/requests',
      auth: 'cookie-or-key',
      summary: 'The intros you asked for, and where they stand.',
      curl: `curl "${BASE_URL}/me/referrals/requests" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": [ { "id": "c92f…", "company_slug": "acme", "status": "contacted" } ] }`,
    },
    {
      method: 'GET',
      path: '/me/referrals/incoming',
      auth: 'cookie-or-key',
      summary: 'Requests waiting on you as an approved referrer.',
      curl: `curl "${BASE_URL}/me/referrals/incoming" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": [ { "id": "c92f…", "company_slug": "acme", "cv_kind": "original", "status": "open", "note": "Backend, 8 years Go" } ] }`,
    },
    {
      method: 'GET',
      path: '/me/referrals/incoming/{id}/cv',
      auth: 'cookie-or-key',
      summary: "View the candidate's CV for one incoming request.",
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The request id.' }],
      curl: `curl "${BASE_URL}/me/referrals/incoming/c92f…/cv" -H "Authorization: Bearer fhk_…" -o candidate.pdf`,
    },
    {
      method: 'POST',
      path: '/me/referrals/incoming/{id}/resolve',
      auth: 'cookie-or-key',
      summary: 'Mark an incoming request contacted or declined.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The request id.' }],
      body: [
        {
          name: 'status',
          type: 'string',
          required: true,
          description: '`contacted` or `declined`.',
          example: 'contacted',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/me/referrals/incoming/c92f…/resolve" \\
  -H "Authorization: Bearer fhk_…" -H 'Content-Type: application/json' -d '{"status":"contacted"}'`,
      responseExample: `{ "data": { "id": "c92f…", "status": "contacted" } }`,
    },
    {
      method: 'GET',
      path: '/referrals/offers',
      auth: 'moderator',
      summary: 'The offer-moderation queue.',
      curl: `curl "${BASE_URL}/referrals/offers" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": [ { "id": "b71e…", "company_slug": "acme", "linkedin_url": "https://linkedin.com/in/you", "status": "pending" } ] }`,
    },
    {
      method: 'GET',
      path: '/referrals/offers/{id}/proof',
      auth: 'moderator',
      summary: "View an offer's proof document.",
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The offer id.' }],
      curl: `curl "${BASE_URL}/referrals/offers/b71e…/proof" -H "Authorization: Bearer fhk_…" -o proof.pdf`,
    },
    {
      method: 'POST',
      path: '/referrals/offers/{id}/decide',
      auth: 'moderator',
      summary: 'Approve or reject an offer.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The offer id.' }],
      body: [
        {
          name: 'approve',
          type: 'boolean',
          required: true,
          description: 'True approves the referrer; false rejects the offer.',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/referrals/offers/b71e…/decide" \\
  -H "Authorization: Bearer fhk_…" -H 'Content-Type: application/json' -d '{"approve":true}'`,
      responseExample: `{ "data": { "id": "b71e…", "status": "approved", "decided_at": "2026-07-28T21:00:00Z" } }`,
    },
  ],
};
