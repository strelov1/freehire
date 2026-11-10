import { BASE_URL, type Group } from '../api-spec-types';

export const linkIntakeDiscoveryGroup: Group = {
  title: 'Link intake & discovery',
  intro:
    'Turning a URL in the wild into something freehire knows about. One intake ' +
    'sequence serves every surface — the website, the bot, the extension and the ' +
    'CLI — deliberately, so a pasted link gets the same answer everywhere.',
  endpoints: [
    {
      method: 'GET',
      path: '/jobs/find',
      auth: 'none',
      summary: 'Resolve a posting URL to a catalogue slug.',
      description:
        'Answers `{"data": null}` when the posting cannot be identified, rather ' +
        'than 404 — "we do not carry this" is a normal answer here. Used by the ' +
        'extension to tell whether the page you are on is a job we already have. ' +
        'A miss also carries `meta.ats_board`: true when the URL sits on an ATS board ' +
        'freehire knows how to crawl, so a client can tell a posting worth adding ' +
        'from an arbitrary page.',
      query: [
        {
          name: 'url',
          type: 'string',
          required: true,
          description: "The posting's URL in the wild.",
          example: 'https://boards.greenhouse.io/acme/jobs/123',
        },
      ],
      curl: `curl "${BASE_URL}/jobs/find?url=https%3A%2F%2Fboards.greenhouse.io%2Facme%2Fjobs%2F123"`,
      responseExample: `{ "data": { "public_slug": "senior-backend-engineer-acme-1a2b" } }`,
    },
    {
      method: 'POST',
      path: '/jobs/resolve',
      auth: 'cookie-or-key',
      summary: 'Hand freehire a link: import the posting, and the board behind it.',
      description:
        'Five outcomes in one shape, distinguished by status: **200 found** — the ' +
        'catalogue already carries the vacancy. Either the URL itself is stored (nothing ' +
        'is fetched or written), or the page turned out to be a second copy of a posting ' +
        'we already carry under another source, in which case the answer names the one we ' +
        'had; **201 tracked** ' +
        '— we crawl that board already and the posting just had not landed, so it ' +
        'was imported now; **201 imported** — imported, and its board queued for ' +
        'onboarding; **201 review** — imported, but its careers site names no board ' +
        'we can crawl, so the link went to manual triage; **202 queued** — nothing ' +
        'could read the page, so the link went to manual triage. `company_slug` is ' +
        'returned whenever the catalogue already carries the employer, through any ' +
        'source — independent of what became of the board. A URL that is not http(s) is a 422.',
      body: [
        { name: 'url', type: 'string', required: true, description: 'The job page to resolve.' },
        {
          name: 'surface',
          type: 'string',
          description:
            'Where the link came from (website, extension, cli, bot) — recorded, not validated against a whitelist.',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/jobs/resolve" \\
  -H "Authorization: Bearer fhk_…" -H 'Content-Type: application/json' \\
  -d '{"url":"https://boards.greenhouse.io/acme/jobs/123","surface":"cli"}'`,
      responseExample: `{ "data": { "status": "tracked", "public_slug": "senior-backend-engineer-acme-1a2b", "company_slug": "acme" } }`,
    },
    {
      method: 'GET',
      path: '/me/contributions',
      auth: 'cookie-or-key',
      summary: 'The boards you contributed, and their onboarding state.',
      curl: `curl "${BASE_URL}/me/contributions" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": [ { "url": "https://boards.greenhouse.io/acme", "source": "greenhouse", "board": "acme", "state": "active", "created_at": "2026-07-20T12:00:00Z" } ] }`,
    },
    {
      method: 'POST',
      path: '/me/jd/resolve',
      auth: 'cookie',
      summary: 'Turn a URL, pasted text, or an existing job into one usable by CV tailoring.',
      description:
        'Exactly one of `job_slug`, `url`, or `text` is required. A recognized ' +
        'URL resolves through the same ATS-aware registry `/jobs/resolve` uses ' +
        '(a network fetch); an ' +
        'unrecognized URL falls back to a generic scrape; pasted text is ' +
        'stored as a private, unlisted job. Returns the slug the tailor ' +
        'workspace should open — an existing job, or a freshly created ' +
        'private one. `404` for an unknown `job_slug`, `422` for a URL ' +
        'nothing could read.',
      body: [
        { name: 'job_slug', type: 'string', description: 'An existing job’s slug.' },
        { name: 'url', type: 'string', description: 'A job posting URL.' },
        { name: 'text', type: 'string', description: 'Pasted job description text.' },
        { name: 'title', type: 'string', description: 'Optional hint, used only alongside `text`.' },
        { name: 'company', type: 'string', description: 'Optional hint, used only alongside `text`.' },
      ],
      curl: `curl -X POST "${BASE_URL}/me/jd/resolve" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"url":"https://boards.greenhouse.io/acme/jobs/123"}'`,
      responseExample: `{ "data": { "job_slug": "senior-go-engineer-acme-1a2b" } }`,
    },
  ],
};
