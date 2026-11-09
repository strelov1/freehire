import { BASE_URL, type Group } from '../api-spec-types';

export const talentNetworkGroup: Group = {
  title: 'Talent Network',
  intro:
    'A public, anonymised catalogue of candidates who opted into being found. ' +
    'Membership is a single toggle on `users`, distinct from `user_profiles`; ' +
    'a member is addressed by a minted handle, never by their account id or ' +
    'username. Every string a public response carries is a value a dictionary ' +
    'resolved, a number or a date — never text copied from a CV — so no name, ' +
    'employer, institution or free-text field appears anywhere in it.',
  endpoints: [
    {
      method: 'GET',
      path: '/me/talent-network',
      auth: 'cookie-or-key',
      summary: 'Your current Talent Network visibility and shareable id.',
      description:
        'A caller who has never touched the setting reads `"off"` — the column ' +
        'default, not a sentinel. `talent_handle` is absent until the first ' +
        'time they join: it is minted then, and kept forever after, so leaving ' +
        'and rejoining never changes the URL somebody already shared.',
      curl: `curl "${BASE_URL}/me/talent-network" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": { "talent_network_visibility": "anonymous", "talent_handle": "backend-7f2a" } }`,
    },
    {
      method: 'PUT',
      path: '/me/talent-network',
      auth: 'cookie',
      summary: 'Join or leave the Talent Network.',
      description:
        'Two states, not three: `public` was retired with the mode picker ' +
        'itself, and a request asking for it is a 400. Joining for the first ' +
        'time mints the handle the public catalogue addresses you by.',
      body: [
        {
          name: 'visibility',
          type: 'string',
          required: true,
          description: 'Either `off` or `anonymous`.',
          example: 'anonymous',
        },
      ],
      curl: `curl -X PUT "${BASE_URL}/me/talent-network" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"visibility":"anonymous"}'`,
      responseExample: `{ "data": { "talent_network_visibility": "anonymous", "talent_handle": "backend-7f2a" } }`,
    },
    {
      method: 'GET',
      path: '/talent',
      auth: 'none',
      summary: 'The public catalogue of members, filtered and paged.',
      description:
        'Every filter takes values from a closed vocabulary or a number; there ' +
        'is no free-text search, because a card carries no free text to search. ' +
        'Values within one filter are OR, different filters narrow together, ' +
        'and an absent filter is identical to an empty one. A parameter this ' +
        'endpoint does not read — including one it recognises but cannot ' +
        'parse, like `min_years=lots` — is named in `meta.ignored_params` ' +
        'rather than silently widening the answer.',
      query: [
        {
          name: 'categories',
          type: 'string',
          description: 'Comma-separated role categories.',
          example: 'backend,devops',
        },
        { name: 'seniorities', type: 'string', description: 'Comma-separated seniority levels.', example: 'senior' },
        {
          name: 'skills',
          type: 'string',
          description: 'Comma-separated canonical skills.',
          example: 'go,postgresql',
        },
        {
          name: 'tz',
          type: 'string',
          description: 'Comma-separated IANA timezone regions. A member with no timezone is excluded when this is set.',
          example: 'Europe',
        },
        { name: 'cities', type: 'string', description: 'Comma-separated normalised cities.', example: 'berlin' },
        {
          name: 'specializations',
          type: 'string',
          description: 'Comma-separated curated specialisations.',
          example: 'platform',
        },
        { name: 'min_years', type: 'integer', description: 'Least total years of experience.', example: '5' },
        { name: 'limit', type: 'integer', description: 'Page size, 1–100. Defaults to 24.', example: '24' },
        { name: 'offset', type: 'integer', description: 'Rows to skip.', example: '0' },
      ],
      curl: `curl "${BASE_URL}/talent?categories=backend&min_years=5"`,
      responseExample: `{
  "data": [
    {
      "handle": "backend-7f2a",
      "card": {
        "seniority": "senior",
        "category": "backend",
        "total_years": 8,
        "skills": ["go", "postgresql"],
        "roles": [{ "seniority": "senior", "category": "backend", "start": { "year": 2024 }, "current": true, "stack": ["go"] }]
      },
      "timezone": "Europe/Berlin",
      "timezone_region": "Europe",
      "cities": ["berlin"],
      "specializations": ["platform"],
      "updated_at": "2026-09-01T12:00:00Z"
    }
  ],
  "meta": { "total": 1, "limit": 24, "offset": 0 }
}`,
    },
    {
      method: 'GET',
      path: '/talent/facets',
      auth: 'none',
      summary: 'How many members stand behind each filter value.',
      description:
        'Takes the same filters as the list and answers with the count of members ' +
        'behind every value of every facet, plus the total behind the filter itself. ' +
        'Same shape as the job-search facet endpoint, so one control renders either.\n\n' +
        'Each facet is counted with its OWN selection removed, while every other ' +
        'filter still applies — otherwise picking one skill makes every other skill ' +
        'read zero and the control silently becomes single-select.\n\n' +
        'A value nobody carries is absent rather than reported as zero. A value ' +
        'carried by fewer members than the floor is offered with its number withheld: ' +
        'the option stays usable, and the number does not become a way to name ' +
        'somebody. Counts describe the filter you sent and never the whole membership.',
      curl: `curl "${BASE_URL}/talent/facets?categories=backend"`,
      responseExample: `{
  "data": {
    "total": 3,
    "facets": {
      "categories": { "backend": 3 },
      "skills": { "go": 3, "kubernetes": 2, "postgresql": 0 },
      "tz": { "Europe": 2 }
    },
    "stats": {}
  }
}`,
    },
    {
      method: 'GET',
      path: '/talent/{handle}',
      auth: 'none',
      summary: 'One member’s public card.',
      description:
        'Four ways of being absent — a member who left, one whose CV extract ' +
        'has gone stale, a handle nobody holds, and a string that could not be a ' +
        'handle — all answer the same 404, so the route cannot be used to ask ' +
        'whether an account exists. An account’s `username` is not an address ' +
        'here. Membership is re-read from the database on every request, so ' +
        'leaving takes effect immediately rather than when a cache expires.',
      pathParams: [
        {
          name: 'handle',
          type: 'string',
          required: true,
          description: 'The member’s minted catalogue handle.',
          example: 'backend-7f2a',
        },
      ],
      curl: `curl "${BASE_URL}/talent/backend-7f2a"`,
      responseExample: `{
  "data": {
    "handle": "backend-7f2a",
    "card": { "seniority": "senior", "category": "backend", "total_years": 8, "skills": ["go"], "roles": [] },
    "timezone": "Europe/Berlin",
    "timezone_region": "Europe",
    "cities": ["berlin"],
    "specializations": [],
    "updated_at": "2026-09-01T12:00:00Z"
  }
}`,
    },
  ],
};
