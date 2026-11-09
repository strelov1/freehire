import { BASE_URL, type Group } from '../api-spec-types';

export const jobInteractionsGroup: Group = {
  title: 'Job interactions',
  intro:
    'Per-user tracking, addressed by the job slug. All accept the session ' +
    'cookie or an API key and are idempotent. The response is the interaction ' +
    'record for that job.',
  endpoints: [
    {
      method: 'POST',
      path: '/jobs/{slug}/view',
      auth: 'cookie-or-key',
      summary: 'Record that you viewed the job.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      curl: `curl -X POST "${BASE_URL}/jobs/<slug>/view" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": { "job_id": 42, "viewed_at": "2026-06-19T10:00:00Z" } }`,
    },
    {
      method: 'POST',
      path: '/jobs/{slug}/apply',
      auth: 'cookie-or-key',
      summary: 'Mark the job as applied to.',
      description:
        'Send `applied_on` to record an application on the day it was actually sent — ' +
        'importing a history, or correcting a date already stored. A date you state ' +
        'overrides one we inferred. The day is stored at noon UTC, because you are stating ' +
        'a day and midnight reads as the previous date west of Greenwich. A date in the ' +
        'future, older than a year, or not a calendar date, is a 400. Without a body the ' +
        'application is stamped now, as before.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      body: [
        {
          name: 'applied_on',
          type: 'string',
          description: 'The day the application was sent (`YYYY-MM-DD`). Defaults to today.',
          example: '2026-07-27',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/jobs/<slug>/apply" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" -H 'Content-Type: application/json' \\
  -d '{"applied_on":"2026-07-27"}'`,
      responseExample: `{ "data": { "job_id": 42, "applied_at": "2026-07-27T12:00:00Z" } }`,
    },
    {
      method: 'POST',
      path: '/jobs/{slug}/save',
      auth: 'cookie-or-key',
      summary: 'Save (bookmark) the job.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      curl: `curl -X POST "${BASE_URL}/jobs/<slug>/save" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": { "job_id": 42, "saved_at": "2026-06-19T10:00:00Z" } }`,
    },
    {
      method: 'DELETE',
      path: '/jobs/{slug}/save',
      auth: 'cookie-or-key',
      summary: 'Unsave the job (no-op if not saved).',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      curl: `curl -X DELETE "${BASE_URL}/jobs/<slug>/save" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": { "job_id": 42, "saved_at": null } }`,
    },
    {
      method: 'PATCH',
      path: '/jobs/{slug}/track',
      auth: 'cookie-or-key',
      summary: 'Set the application stage and/or notes.',
      description:
        'A null field is left unchanged. `stage` is a controlled vocabulary: ' +
        '`preparing`, `applied`, `screening`, `responded`, `interview`, `offer`, `accepted`, ' +
        '`rejected`, `withdrawn`, `expired` (an unknown value is a 400). `expired` is the ' +
        'outcome for an application nobody answered; nothing sets it for you.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      body: [
        {
          name: 'stage',
          type: 'string',
          description: 'Application stage from the vocabulary above.',
          example: 'interview',
        },
        { name: 'notes', type: 'string', description: 'Free-text notes.' },
      ],
      curl: `curl -X PATCH "${BASE_URL}/jobs/<slug>/track" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" \\
  -H 'Content-Type: application/json' \\
  -d '{"stage":"interview","notes":"call on Friday"}'`,
      responseExample: `{ "data": { "job_id": 42, "stage": "interview", "notes": "call on Friday" } }`,
    },
    {
      method: 'DELETE',
      path: '/jobs/{slug}/stage',
      auth: 'cookie-or-key',
      summary: 'Clear the application stage.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      curl: `curl -X DELETE "${BASE_URL}/jobs/<slug>/stage" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": { "job_id": 42, "stage": null } }`,
    },
    {
      method: 'DELETE',
      path: '/jobs/{slug}/track',
      auth: 'cookie-or-key',
      summary: 'Remove the interaction record entirely.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      curl: `curl -X DELETE "${BASE_URL}/jobs/<slug>/track" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": { "ok": true } }`,
    },
    {
      method: 'PATCH',
      path: '/me/applications/{id}',
      auth: 'cookie-or-key',
      summary: 'Set the application stage and/or notes, addressed by row id.',
      description:
        'Same as `PATCH /jobs/{slug}/track`, but addressed by the ' +
        'tracking-board row id instead of the job slug — needed when a ' +
        'tracked application’s posting was later pruned and has no slug left ' +
        'to address it by. Same `stage` vocabulary and body shape.',
      pathParams: [
        {
          name: 'id',
          type: 'integer',
          required: true,
          description: 'The tracking row id (from `/me/tracking`).',
          example: '42',
        },
      ],
      body: [
        { name: 'stage', type: 'string', description: 'Application stage.', example: 'interview' },
        { name: 'notes', type: 'string', description: 'Free-text notes.' },
      ],
      curl: `curl -X PATCH "${BASE_URL}/me/applications/42" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" \\
  -H 'Content-Type: application/json' \\
  -d '{"stage":"interview"}'`,
      responseExample: `{ "data": { "job_id": 42, "stage": "interview", "notes": null } }`,
    },
    {
      method: 'DELETE',
      path: '/me/applications/{id}',
      auth: 'cookie-or-key',
      summary: 'Remove the interaction record entirely, addressed by row id.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The tracking row id.', example: '42' }],
      curl: `curl -X DELETE "${BASE_URL}/me/applications/42" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": { "ok": true } }`,
    },
    {
      method: 'DELETE',
      path: '/me/applications/{id}/stage',
      auth: 'cookie-or-key',
      summary: 'Clear the application stage, addressed by row id.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The tracking row id.', example: '42' }],
      curl: `curl -X DELETE "${BASE_URL}/me/applications/42/stage" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": { "job_id": 42, "stage": null } }`,
    },
    {
      method: 'POST',
      path: '/jobs/{slug}/dismiss',
      auth: 'cookie-or-key',
      summary: 'Dismiss (swipe away) the job.',
      description:
        'Only keeps the job out of the swipe deck; it stays visible in the public ' +
        '`/jobs` list and search. Idempotent.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      curl: `curl -X POST "${BASE_URL}/jobs/<slug>/dismiss" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": { "job_id": 42, "dismissed_at": "2026-06-19T10:00:00Z", "saved_at": null, "stage": null } }`,
    },
    {
      method: 'DELETE',
      path: '/jobs/{slug}/dismiss',
      auth: 'cookie-or-key',
      summary: 'Undismiss the job (no-op if not dismissed).',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      curl: `curl -X DELETE "${BASE_URL}/jobs/<slug>/dismiss" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": { "job_id": 42, "dismissed_at": null } }`,
    },
    {
      method: 'GET',
      path: '/me/tracking',
      auth: 'cookie-or-key',
      summary: 'Your tracked jobs joined with the job data.',
      description:
        'Each item carries a card of the job with your interaction timestamps alongside ' +
        'it — what a list row draws: slug, title, company, closed_at, the stated facets, ' +
        'skills, collections, posted_at, and a `blurb` already cut to length. It does NOT ' +
        'carry the description; the full job view is on `GET /me/tracking/:slug`. ' +
        '`meta.counts` gives the per-filter totals for tab badges. Closed jobs stay ' +
        'listed so your history never shrinks.',
      query: [
        {
          name: 'filter',
          type: 'string',
          description:
            'Subset to return: `all`, `viewed`, `saved`, `applied`, or `board` (default `all`; an unknown value is a 400).',
          example: 'applied',
        },
        { name: 'limit', type: 'integer', description: 'Page size, 1–100.', example: '20' },
        { name: 'offset', type: 'integer', description: 'Rows to skip.', example: '0' },
      ],
      curl: `curl "${BASE_URL}/me/tracking?filter=applied" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{
  "data": [
    {
      "job": { "public_slug": "senior-go-engineer-acme-1a2b", "title": "Senior Go Engineer", "...": "..." },
      "viewed_at": "2026-06-19T10:00:00Z",
      "saved_at": null,
      "applied_at": "2026-06-19T11:00:00Z",
      "stage": "interview",
      "notes": "call on Friday"
    }
  ],
  "meta": {
    "total": 5,
    "limit": 20,
    "offset": 0,
    "counts": { "all": 12, "viewed": 12, "saved": 3, "applied": 5, "board": 7 }
  }
}`,
    },
    {
      method: 'GET',
      path: '/me/tracking/viewed',
      auth: 'cookie-or-key',
      summary: 'Slugs of jobs you have viewed.',
      curl: `curl "${BASE_URL}/me/tracking/viewed" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": ["senior-go-engineer-acme-1a2b", "..."] }`,
    },
    {
      method: 'GET',
      path: '/me/tracking/analyses',
      auth: 'cookie-or-key',
      summary: 'Jobs you have run the AI match analysis on.',
      description:
        'Newest first, closed jobs included (with `closed: true`). Each item carries the ' +
        'overall score and verdict; `stale` marks an analysis whose CV, job, or model has ' +
        'changed since. `meta.allowance` reports where you stand on the day’s analyses. Never runs the LLM.',
      curl: `curl "${BASE_URL}/me/tracking/analyses" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{
  "data": [
    {
      "slug": "senior-go-engineer-acme-1a2b",
      "title": "Senior Go Engineer",
      "company": "Acme",
      "closed": false,
      "overall_score": 82,
      "verdict": "Strong Fit",
      "analysed_at": "2026-07-11T10:00:00Z",
      "stale": false
    }
  ],
  "meta": { "allowance": { "feature": "match", "used": 1, "limit": 3, "unlimited": false, "enforced": false, "resets_at": "2026-09-01T00:00:00Z" } }
}`,
    },
    {
      method: 'GET',
      path: '/me/plan',
      auth: 'cookie-or-key',
      summary: 'Your plan and what it allows today.',
      description:
        'Which plan you are on and, for every metered AI feature, how much of today you ' +
        'have used against what the day allows. Every plan offers every feature; what ' +
        'differs is the daily amount, and it resets at `resets_at`. A pro caller reads as ' +
        '`unlimited` rather than as a number. `enforced` says whether that ceiling turns ' +
        'anybody away yet — while it is `false` a spent allowance is counted and the ' +
        'action still runs, so do not refuse on `used >= limit` alone. Never runs the LLM. ' +
        'A pro caller also carries `pro_until` and `pro_source` — `stripe`, `revenuecat` ' +
        'or `granted` — both absent on the free plan and on a plan that has ended. ' +
        '`pro_source` is behavioural: a client offering an in-app purchase must not offer ' +
        'one to a `stripe` subscriber, who would be charged twice for one plan, and a ' +
        '`revenuecat` subscriber must be sent to their store to cancel rather than to a ' +
        'web page. A tie resolves in the order `stripe`, `revenuecat`, `granted`.',
      curl: `curl "${BASE_URL}/me/plan" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{
  "data": {
    "plan": "free",
    "resets_at": "2026-09-01T00:00:00Z",
    "allowances": [
      { "feature": "tailor", "used": 1, "limit": 2, "unlimited": false, "enforced": false, "resets_at": "2026-09-01T00:00:00Z" },
      { "feature": "match", "used": 0, "limit": 3, "unlimited": false, "enforced": false, "resets_at": "2026-09-01T00:00:00Z" }
    ]
  }
}`,
    },
    {
      method: 'POST',
      path: '/billing/revenuecat/sync',
      auth: 'cookie',
      summary: 'Re-read your own store subscription now.',
      description:
        'For a mobile client that has just completed an App Store or Google Play ' +
        'purchase. The purchase finishes on the device before the provider’s webhook ' +
        'reaches us, and if that delivery is lost the provider stops retrying after 80 ' +
        'minutes — so without this route a paid subscriber can be left looking at a ' +
        'paywall. It names nobody: the account is the session’s, and a user id in the ' +
        'request is ignored. Absent entirely where the store ' +
        'provider is unconfigured. `status` is `synced` when the plan was written, or ' +
        '`no_subscription` when RevenueCat holds nothing for this account; a 503 means ' +
        'the provider could not be reached and the hourly reconciler will finish the job.',
      curl: `curl -X POST "${BASE_URL}/billing/revenuecat/sync" -b cookies.txt`,
      responseExample: `{ "data": { "status": "synced" } }`,
    },
    {
      method: 'GET',
      path: '/me/tracking/saved',
      auth: 'cookie-or-key',
      summary: 'Slugs of jobs you have saved.',
      description: 'Lets the SPA render the save toggle as filled without authenticating the public job reads.',
      curl: `curl "${BASE_URL}/me/tracking/saved" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": ["senior-go-engineer-acme-1a2b", "..."] }`,
    },
    {
      method: 'GET',
      path: '/me/tracking/applied-companies',
      auth: 'cookie-or-key',
      summary: 'Company slugs of the employers you have applied to.',
      description:
        'Lets the SPA mark "you already applied to this company" on any posting of theirs ' +
        "without authenticating the public job reads: compare a posting's `company_slug` " +
        'against this set. Counts applications actually made (not ones still being ' +
        "prepared), keyed by the posting's current company slug and resolved through " +
        'company merges. Newest first, drawn from your 1000 most recent applications.',
      curl: `curl "${BASE_URL}/me/tracking/applied-companies" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": ["acme", "globex"] }`,
    },
    {
      method: 'GET',
      path: '/me/tracking/pipeline',
      auth: 'cookie-or-key',
      summary: 'Your application-pipeline snapshot (counts per stage).',
      description:
        'The total application count and the count at each stage, aggregated server-side ' +
        'over all of your applications. Every stage of the vocabulary is present, zero ' +
        'included, and the counts always sum to `applications`. An application with ' +
        '`applied_at` set but no explicit stage counts as `applied`. `reply_rate` compares ' +
        'your own employer-reply rate against the rate across every candidate — both sides ' +
        'counted only from applications whose owner has a connected mailbox, so a reply ' +
        'would have been seen. It is present only once both your own count and the global ' +
        'count reach a minimum sample size; absent otherwise, never a zero or an estimate.',
      curl: `curl "${BASE_URL}/me/tracking/pipeline" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{
  "data": {
    "applications": 12,
    "stages": {
      "applied": 4,
      "screening": 2,
      "responded": 1,
      "interview": 2,
      "offer": 1,
      "accepted": 1,
      "rejected": 1,
      "withdrawn": 0
    },
    "reply_rate": {
      "you": { "applications": 12, "answered": 4 },
      "global": { "applications": 287, "answered": 97 }
    }
  }
}`,
    },
    {
      method: 'GET',
      path: '/me/tracking/swipe',
      auth: 'cookie-or-key',
      summary: 'A batch of open jobs for the swipe triage deck.',
      description:
        'Runs the same query as search (same facets, `q`, and sort), then excludes ' +
        'the jobs you have already saved or dismissed. A param no filter reads is ' +
        'ignored rather than refused, and listed in `meta.ignored_params`, exactly ' +
        'as in search. `503` when search is unavailable.',
      query: [
        { name: 'q', type: 'string', description: 'Optional full-text query (as in search).', example: 'golang' },
        { name: 'limit', type: 'integer', description: 'Page size, 1–100.', example: '20' },
        { name: 'offset', type: 'integer', description: 'Rows to skip; `offset + limit` ≤ 10000.', example: '0' },
      ],
      curl: `curl "${BASE_URL}/me/tracking/swipe" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{
  "data": [ { "public_slug": "...", "title": "Senior Go Engineer", "...": "..." } ],
  "meta": { "total": 137, "limit": 20, "offset": 0 }
}`,
    },
    {
      method: 'GET',
      path: '/me/timeline',
      auth: 'cookie-or-key',
      summary: 'What happened to your applications over a date range.',
      description:
        'The application-event ledger as a dated series, oldest first: applications ' +
        'sent, employer replies, follow-ups and stage changes. `occurred_at` is an ' +
        'instant, not a day — which day it falls on depends on your timezone, so ' +
        'group it client-side. `observed` says whether the date came from a source ' +
        'other than you: mail-derived events carry a date the employer set, while a ' +
        'stage you set yourself is dated from when you recorded it. `email_id` and ' +
        '`email_subject` are present only while the message exists; deleting it hides ' +
        'the content and leaves the event standing. Both bounds are required and may ' +
        'span at most 366 days.',
      query: [
        {
          name: 'from',
          type: 'string (RFC3339)',
          required: true,
          description:
            'Lower bound, inclusive. Use `Z`, or percent-encode a numeric offset — a bare `+` decodes as a space in a query string and will be rejected.',
          example: '2026-08-01T00:00:00Z',
        },
        {
          name: 'to',
          type: 'string (RFC3339)',
          required: true,
          description: 'Upper bound, inclusive.',
          example: '2026-08-31T23:59:59Z',
        },
      ],
      curl: `curl "${BASE_URL}/me/timeline?from=2026-08-01T00:00:00Z&to=2026-08-31T23:59:59Z" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{
  "data": [
    {
      "id": 7,
      "kind": "employer_reply",
      "signal": "interview_invitation",
      "source": "mail_gmail",
      "observed": true,
      "occurred_at": "2026-08-13T09:41:00Z",
      "company_slug": "derq",
      "role_title": "Senior Go Engineer",
      "application_id": 31,
      "job_slug": "senior-go-engineer-derq-1a2b",
      "email_id": 42,
      "email_subject": "Invitation to interview"
    },
    {
      "id": 8,
      "kind": "stage_set",
      "signal": "screening",
      "source": "user",
      "observed": false,
      "occurred_at": "2026-08-13T21:15:00Z",
      "company_slug": "linear",
      "application_id": 33
    }
  ],
  "meta": { "from": "2026-08-01T00:00:00Z", "to": "2026-08-31T23:59:59Z", "count": 2 }
}`,
    },
    {
      method: 'GET',
      path: '/me/interviews',
      auth: 'cookie-or-key',
      summary: 'Arranged meetings whose start falls in the date range.',
      description:
        'The calendar’s second layer beside `/me/timeline`: a meeting is ' +
        'arranged and can move or be cancelled, unlike a ledger event which ' +
        'happened and cannot change. `status` is `suggested`, `confirmed`, or ' +
        '`cancelled` — a cancelled meeting is still served, not withheld. Both ' +
        'bounds are required, RFC3339.',
      query: [
        {
          name: 'from',
          type: 'string (RFC3339)',
          required: true,
          description: 'Lower bound, inclusive.',
          example: '2026-08-01T00:00:00Z',
        },
        {
          name: 'to',
          type: 'string (RFC3339)',
          required: true,
          description: 'Upper bound, inclusive.',
          example: '2026-08-31T23:59:59Z',
        },
      ],
      curl: `curl "${BASE_URL}/me/interviews?from=2026-08-01T00:00:00Z&to=2026-08-31T23:59:59Z" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{
  "data": [
    {
      "id": 3,
      "application_id": 31,
      "starts_at": "2026-08-20T13:00:00Z",
      "ends_at": "2026-08-20T13:30:00Z",
      "title": "Interview with Acme",
      "join_url": "https://meet.example.com/abc",
      "status": "confirmed",
      "company_slug": "acme",
      "role_title": "Senior Go Engineer",
      "job_slug": "senior-go-engineer-acme-1a2b"
    }
  ],
  "meta": { "from": "2026-08-01T00:00:00Z", "to": "2026-08-31T23:59:59Z", "count": 1 }
}`,
    },
  ],
};
