import { BASE_URL, type Group } from '../api-spec-types';

export const accountPlanExtensionGroup: Group = {
  title: 'Account, plan & extension',
  intro:
    'The rest of the account surface: the password, deleting the account, the ' +
    'plan and what it allows today, and the two endpoints the browser extension runs on. ' +
    'Password and deletion are session-only — an API key must not be able to ' +
    'change or destroy the credential it would outlive.',
  endpoints: [
    {
      method: 'POST',
      path: '/me/password',
      auth: 'cookie',
      summary: 'Change a known password.',
      description:
        'Revokes every session; your own cookie is re-issued at the new ' +
        'generation, so you stay signed in and everyone else is signed out.',
      body: [
        { name: 'current_password', type: 'string', required: true, description: 'Your existing password.' },
        { name: 'new_password', type: 'string', required: true, description: 'The replacement.' },
      ],
      curl: `curl -X POST "${BASE_URL}/me/password" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"current_password":"…","new_password":"…"}'`,
      responseExample: `{ "data": { "ok": true } }`,
    },
    {
      method: 'DELETE',
      path: '/me',
      auth: 'cookie',
      summary: 'Delete your account and everything under it.',
      curl: `curl -X DELETE "${BASE_URL}/me" -b cookies.txt`,
    },
    {
      method: 'GET',
      path: '/me/plan/history',
      auth: 'cookie-or-key',
      summary: 'What you spent your allowances on, newest first.',
      description:
        'Each entry is labelled with what it bought — the job an analysis was run on, the vacancy a CV editing session was opened for — rather than an opaque reference. A `release` entry is one that was given back because the work produced nothing.',
      curl: `curl "${BASE_URL}/me/plan/history" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": [ { "feature": "match", "day": "2026-08-31", "kind": "consume", "label": "Job analysis", "subtitle": "Senior Backend Engineer at Acme", "created_at": "2026-08-31T09:00:00Z" } ] }`,
    },
    {
      method: 'GET',
      path: '/me/usage',
      auth: 'cookie-or-key',
      summary: 'Your AI request activity today.',
      description:
        'Counts and token usage, not cost — the gateway’s dollar figure is a ' +
        'list price against a mixed upstream pool, not what you pay; what you ' +
        'spend is a plan allowance, reported by `/me/plan` over this same day. ' +
        'Never fails for a reason you could act on: no usage yet, or an ' +
        'unreachable gateway, both answer 200 with zeroes.',
      curl: `curl "${BASE_URL}/me/usage" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": { "requests": 42, "failed": 1, "tokens": 118000, "period": "2026-08-31", "resets_at": "2026-09-01T00:00:00Z" } }`,
    },
    {
      method: 'GET',
      path: '/me/tracking/{slug}',
      auth: 'cookie-or-key',
      summary: 'One tracked application, with the mail linked to it and its history.',
      description:
        "`events` is the application's ledger, newest first — the apply, employer " +
        'replies, follow-ups, stage changes, scheduled interviews — in the shape ' +
        '`GET /me/timeline` serves, bounded at 100 and empty when nothing has happened ' +
        'yet. This read also carries the full job view; the listing carries only a card. ' +
        'A slug you do not track is a 404.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      curl: `curl "${BASE_URL}/me/tracking/senior-backend-engineer-acme-1a2b" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{
  "data": {
    "slug": "senior-backend-engineer-acme-1a2b",
    "stage": "interview",
    "applied_at": "2026-07-24T09:00:00Z",
    "emails": [ { "id": 4821, "subject": "Interview for …", "status_signal": "interview_invitation" } ],
    "events": [
      { "id": 991, "kind": "employer_reply", "signal": "interview_invitation", "source": "mail_gmail", "observed": true, "occurred_at": "2026-07-29T11:04:00Z" },
      { "id": 802, "kind": "applied", "source": "user", "observed": false, "occurred_at": "2026-07-24T09:00:00Z" }
    ]
  }
}`,
    },
    {
      method: 'GET',
      path: '/me/tracking/dismissed',
      auth: 'cookie-or-key',
      summary: 'The slugs you dismissed, so a client can hide them.',
      curl: `curl "${BASE_URL}/me/tracking/dismissed" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": [ "junior-php-developer-acme-9z8y" ] }`,
    },
    {
      method: 'POST',
      path: '/me/match-text',
      auth: 'cookie-or-key',
      summary: 'Score any job text against your profile.',
      description:
        'The same deterministic skill coverage as `GET /jobs/{slug}/match`, but ' +
        'for a page that need not be in the catalogue — this is what lets the ' +
        'extension show a match on any job page. A caller with no profile is a 404. ' +
        'No LLM, and it draws on no allowance.',
      body: [
        { name: 'title', type: 'string', required: true, description: 'The posting title.' },
        { name: 'text', type: 'string', required: true, description: 'The scraped posting text.' },
      ],
      curl: `curl -X POST "${BASE_URL}/me/match-text" \\
  -H "Authorization: Bearer fhk_…" -H 'Content-Type: application/json' \\
  -d '{"title":"Senior Backend Engineer","text":"We are looking for …"}'`,
      responseExample: `{ "data": { "score": 68, "matched": ["go","postgresql"], "missing": ["kubernetes"] } }`,
    },
    {
      method: 'GET',
      path: '/me/autofill-profile',
      auth: 'cookie-or-key',
      summary: 'Your contact details in canonical autofill fields.',
      description:
        'Projected from your CV header, with the account email as a fallback — what the extension writes into application forms.',
      curl: `curl "${BASE_URL}/me/autofill-profile" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": { "first_name": "Ilya", "last_name": "Strelov", "email": "you@example.com", "phone": "+1…", "linkedin": "https://linkedin.com/in/you", "portfolio": "https://ilya.dev" } }`,
    },
    {
      method: 'POST',
      path: '/me/autofill/run',
      auth: 'extension',
      summary: 'Drive your own browser to fill an application form.',
      description:
        'Runs the autofill agent over the browser-tool wire — your extension on ' +
        'one end, the agent on the other, routed strictly within your own channel. ' +
        'Requires the extension to be connected, and authenticates only as the ' +
        'extension itself — a session cookie or an API key gets a 403, since this ' +
        'call writes into whatever form your browser has open, not just reads it.',
      curl: `curl -X POST "${BASE_URL}/me/autofill/run" -H "Authorization: Bearer <extension session token>"`,
      responseExample: `{ "data": { "filled": 11, "skipped": 2 } }`,
    },
    {
      method: 'PATCH',
      path: '/me/timezone',
      auth: 'cookie',
      summary: 'Set your account’s IANA timezone.',
      body: [
        {
          name: 'timezone',
          type: 'string',
          required: true,
          description: 'IANA timezone name.',
          example: 'Europe/Berlin',
        },
        {
          name: 'only_if_unset',
          type: 'boolean',
          description:
            'Automatically adopt the zone only when no timezone is stored. Defaults to false; explicit edits replace the stored zone.',
        },
      ],
      curl: `curl -X PATCH "${BASE_URL}/me/timezone" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"timezone":"Europe/Berlin"}'`,
      responseExample: `{ "data": { "id": 1, "email": "me@example.com", "role": "user", "timezone": "Europe/Berlin", "...": "..." } }`,
    },
    {
      method: 'PATCH',
      path: '/me/language',
      auth: 'cookie',
      summary: 'Set your preferred interface language.',
      body: [
        { name: 'language', type: 'string', required: true, description: 'Supported language code.', example: 'en' },
      ],
      curl: `curl -X PATCH "${BASE_URL}/me/language" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"language":"en"}'`,
      responseExample: `{ "data": { "id": 1, "email": "me@example.com", "role": "user", "language": "en", "...": "..." } }`,
    },
  ],
};
