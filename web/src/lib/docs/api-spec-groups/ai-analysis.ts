import { BASE_URL, type Group } from '../api-spec-types';

export const aiAnalysisGroup: Group = {
  title: 'AI analysis',
  intro:
    'Personalized signals computed against the caller’s profile or stored CV. ' +
    'All accept the session cookie or an API key. The skill-match endpoint is ' +
    'deterministic (no LLM); the match-analysis endpoints run the LLM chain and draw on ' +
    'your daily allowance. All take the same facet filter params as search where they narrow a ' +
    'market or candidate set.',
  endpoints: [
    {
      method: 'GET',
      path: '/jobs/{slug}/match',
      auth: 'cookie-or-key',
      summary: 'Deterministic skill match of the job against your profile (no LLM).',
      description:
        'How well the job’s skills are covered by your profile skills — exact, ' +
        'adjacent, and missing, plus a coverage percent. A caller without a saved ' +
        'profile is a 404.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      curl: `curl "${BASE_URL}/jobs/<slug>/match" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{
  "data": {
    "total": 12,
    "exact_count": 7,
    "adjacent_count": 2,
    "coverage_percent": 75,
    "matched": ["go", "postgresql"],
    "adjacent": [ { "name": "typescript", "via": "javascript" } ],
    "missing": ["kubernetes"]
  }
}`,
    },
    {
      method: 'GET',
      path: '/jobs/{slug}/match-analysis',
      auth: 'cookie-or-key',
      summary: 'The cached AI match analysis for the job (never runs the LLM).',
      description:
        'Returns the cached analysis, flagged `stale` when your CV or the job ' +
        'changed since it was computed, or a null analysis when none is cached. ' +
        '`has_cv` is false when you have no stored CV. `allowance` reports how much of ' +
        'today you have used against what the day allows, and when it resets. ' +
        '`tailor_allowance` reports the same for CV tailoring, which carries its own ' +
        'daily ceiling, so a caller deciding between the two has both figures here.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      curl: `curl "${BASE_URL}/jobs/<slug>/match-analysis" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{
  "data": {
    "has_cv": true,
    "stale": false,
    "analysis": {
      "overall_score": 82,
      "verdict": "Strong Fit",
      "dimensions": { "...": "..." },
      "requirement_match": [ { "text": "5+ years Go", "priority": "required", "status": "covered", "evidence": "..." } ],
      "strengths": ["..."],
      "gaps": ["..."],
      "recommendation": "..."
    },
    "allowance": { "feature": "match", "used": 1, "limit": 3, "unlimited": false, "enforced": false, "resets_at": "2026-09-01T00:00:00Z" },
    "tailor_allowance": { "feature": "tailor", "used": 0, "limit": 2, "unlimited": false, "enforced": false, "resets_at": "2026-09-01T00:00:00Z" }
  }
}`,
    },
    {
      method: 'POST',
      path: '/jobs/{slug}/match-analysis',
      auth: 'cookie-or-key',
      summary: 'Run the three-stage AI match analysis and cache it.',
      description:
        'Runs the match prompt-chain over your stored CV and the job, caches the ' +
        'result, and returns it fresh (no `allowance` on this response). Analysing a new ' +
        'job draws on your daily analysis allowance; with none left it is a `402`, and recomputing ' +
        'an already-analyzed job is free. `has_cv` is false when no CV is stored; a ' +
        'failing or unconfigured LLM returns a null analysis (200).',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      curl: `curl -X POST "${BASE_URL}/jobs/<slug>/match-analysis" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{
  "data": {
    "has_cv": true,
    "stale": false,
    "analysis": { "overall_score": 82, "verdict": "Strong Fit", "...": "..." }
  }
}`,
    },
    {
      method: 'GET',
      path: '/jobs/{slug}/match-analysis/stream',
      auth: 'cookie-or-key',
      summary: 'Run the match analysis over Server-Sent Events.',
      description:
        'The same three-stage chain as `POST /jobs/{slug}/match-analysis`, streamed as SSE ' +
        '(`text/event-stream`) rather than a single JSON body. Each event’s `kind` ' +
        'is one of `stage_start`, `stage_done`, `thinking`, `requirements`, ' +
        '`dimensions`, `final`; the `final` event carries the completed `analysis` ' +
        '(the same shape as the match-analysis endpoints). Not a JSON endpoint.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      curl: `curl -N "${BASE_URL}/jobs/<slug>/match-analysis/stream" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `data: {"kind":"stage_start","stage":1,"label":"Extracting requirements"}

data: {"kind":"requirements","requirements":[ { "...": "..." } ]}

data: {"kind":"final","analysis":{"overall_score":82,"verdict":"Strong Fit","...":"..."}}`,
    },
    {
      method: 'POST',
      path: '/market/coverage',
      auth: 'cookie-or-key',
      summary: 'Score a supplied skill list against the filtered job market.',
      description:
        'Stateless sibling of the CV verdict: skills come from the request body, the ' +
        'market from the facet query params (same vocabulary as search; the `skills` ' +
        'facet is ignored as a filter). Reports how many of the role’s vacancies ' +
        'your skills cover and which missing skill unlocks the most. `400` on empty ' +
        'skills, `503` when search is unavailable.',
      query: [
        {
          name: '(any search filter)',
          type: 'string',
          description: 'Any search facet param scopes the market (the `skills` facet is ignored here).',
          example: 'category=backend',
        },
      ],
      body: [
        {
          name: 'skills',
          type: 'string[]',
          required: true,
          description: 'The skill list to score (max 100).',
          example: '["go","postgresql"]',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/market/coverage?category=backend" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" \\
  -H 'Content-Type: application/json' \\
  -d '{"skills":["go","postgresql"]}'`,
      responseExample: `{
  "data": {
    "total": 1820,
    "covered": 1400,
    "coverage_percent": 77,
    "gaps": [ { "name": "kubernetes", "new_vacancies": 120, "unlock_percent": 7 } ],
    "skills": [ { "name": "go", "market_frequency": 61, "must_have": true, "status": "strong", "advice": "" } ],
    "must_have_total": 8,
    "must_have_covered": 6,
    "stack_match_percent": 75,
    "bundles": [ { "...": "..." } ]
  }
}`,
    },
  ],
};
