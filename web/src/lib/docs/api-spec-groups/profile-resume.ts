import { BASE_URL, type Group } from '../api-spec-types';

export const profileResumeGroup: Group = {
  title: 'Profile & résumé',
  intro:
    'Your career profile and stored CV, session-only (a browser feature, like ' +
    'saved searches). The profile is a singleton keyed by your session — no id in ' +
    'the path. The verdict and ATS report are read-only sub-resources computed from ' +
    'the profile and CV; résumé storage degrades to `501` when object storage is ' +
    'unconfigured.',
  endpoints: [
    {
      method: 'GET',
      path: '/me/profile',
      auth: 'cookie',
      summary: 'Your career profile, or null if you have not saved one.',
      curl: `curl "${BASE_URL}/me/profile" -b cookies.txt`,
      responseExample: `{
  "data": {
    "specializations": ["backend"],
    "skills": ["go", "postgresql"],
    "location_preferences": { "work_modes": ["remote"], "base": { "country": "DE" }, "...": "..." },
    "created_at": "2026-06-19T10:00:00Z",
    "updated_at": "2026-06-19T10:00:00Z"
  }
}`,
    },
    {
      method: 'PUT',
      path: '/me/profile',
      auth: 'cookie',
      summary: 'Create or replace your profile.',
      description:
        'The whole profile is replaced on each save. An unknown specialization ' +
        '(must be a job category), empty skills, or an out-of-vocabulary location ' +
        'value is a `400`.',
      body: [
        {
          name: 'specializations',
          type: 'string[]',
          required: true,
          description: 'One or more job categories (max 5).',
          example: '["backend"]',
        },
        {
          name: 'skills',
          type: 'string[]',
          required: true,
          description: 'Canonical skill tokens (non-empty).',
          example: '["go","postgresql"]',
        },
        {
          name: 'job_languages',
          type: 'string[]',
          description:
            'Preferred vacancy languages as two-letter codes (max 200); empty or omitted means any language.',
          example: '["en","ru"]',
        },
        {
          name: 'location_preferences',
          type: 'object',
          description: 'Optional location block (`work_modes`, `remote`, `base`, `relocation`); omit or null to clear.',
        },
      ],
      curl: `curl -X PUT "${BASE_URL}/me/profile" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"specializations":["backend"],"skills":["go","postgresql"]}'`,
      responseExample: `{ "data": { "specializations": ["backend"], "skills": ["go", "postgresql"], "location_preferences": null } }`,
    },
    {
      method: 'DELETE',
      path: '/me/profile',
      auth: 'cookie',
      summary: 'Clear your profile (idempotent).',
      description: 'Returns `204 No Content`.',
      curl: `curl -X DELETE "${BASE_URL}/me/profile" -b cookies.txt`,
    },
    {
      method: 'GET',
      path: '/me/profile/verdict',
      auth: 'cookie',
      summary: 'Market-coverage verdict for your profile skills.',
      description:
        'How many of your selected role’s open vacancies your skills reach, and ' +
        'which missing skill unlocks the most. The role is the request’s facet ' +
        'params (defaulting to your specializations). No profile is a `404`; `503` ' +
        'when search is unavailable.',
      query: [
        {
          name: '(any search filter)',
          type: 'string',
          description:
            'Any search facet param scopes the role (the `skills` facet is ignored; your profile skills are the measured set).',
          example: 'category=backend',
        },
      ],
      curl: `curl "${BASE_URL}/me/profile/verdict" -b cookies.txt`,
      responseExample: `{
  "data": {
    "total": 1820,
    "covered": 1400,
    "coverage_percent": 77,
    "gaps": [ { "name": "kubernetes", "new_vacancies": 120, "unlock_percent": 7 } ],
    "skills": [ { "...": "..." } ],
    "coherence_percent": 64,
    "bundles": [ { "...": "..." } ]
  }
}`,
    },
    {
      method: 'GET',
      path: '/me/profile/ats-report',
      auth: 'cookie',
      summary: 'CV ATS-readiness report (deterministic + any cached LLM review).',
      description:
        'Scores your stored CV’s structure and its keyword match against the ' +
        'selected role. `has_cv` is false when no CV is stored; no profile is a ' +
        '`404`; `503` when search is unavailable.',
      curl: `curl "${BASE_URL}/me/profile/ats-report" -b cookies.txt`,
      responseExample: `{
  "data": {
    "has_cv": true,
    "report": {
      "overall": 78,
      "potential": 90,
      "categories": [ { "id": "structure", "label": "Structure", "score": 18, "max": 20, "items": [ { "...": "..." } ] } ],
      "strong_keywords": ["go", "postgresql"],
      "recommended_keywords": ["kubernetes"],
      "reviewed": false
    }
  }
}`,
    },
    {
      method: 'POST',
      path: '/me/profile/ats-report',
      auth: 'cookie',
      summary: 'Run the optional LLM qualitative ATS review and cache it.',
      description:
        'Runs the LLM review over your stored CV and folds it into the report ' +
        '(`reviewed: true`). Best-effort: an unconfigured or failing LLM returns the ' +
        'deterministic report (200).',
      curl: `curl -X POST "${BASE_URL}/me/profile/ats-report" -b cookies.txt`,
      responseExample: `{ "data": { "has_cv": true, "report": { "overall": 78, "reviewed": true, "...": "..." } } }`,
    },
    {
      method: 'POST',
      path: '/me/resume/extract',
      auth: 'cookie',
      summary: 'Extract a structured profile from an uploaded résumé (no LLM).',
      description:
        'Accepts a PDF (`multipart/form-data` field `file`) or plain text ' +
        '(`application/json` `{ "text": ... }`). Returns canonical skill slugs, the ' +
        'categories it spans, and the resolved seniority (omitted when unresolved). ' +
        'When storage is configured it also stores the résumé once.',
      body: [
        {
          name: 'text',
          type: 'string',
          description: 'Résumé plain text (JSON path); or send a PDF as multipart field `file`.',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/me/resume/extract" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"text":"Senior Go engineer, 6 years..."}'`,
      responseExample: `{ "data": { "skills": ["go", "postgresql"], "categories": ["backend"], "seniority": "senior" } }`,
    },
    {
      method: 'PUT',
      path: '/me/resume',
      auth: 'cookie',
      summary: 'Store or replace your résumé.',
      description:
        'Accepts a PDF (multipart `file`) or JSON `{ "text": ... }`. Returns the ' +
        'résumé metadata. `501` when object storage is unconfigured.',
      body: [
        {
          name: 'text',
          type: 'string',
          description: 'Résumé plain text (JSON path); or send a PDF as multipart field `file`.',
        },
      ],
      curl: `curl -X PUT "${BASE_URL}/me/resume" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"text":"Senior Go engineer, 6 years..."}'`,
      responseExample: `{ "data": { "enabled": true, "present": true, "uploaded_at": "2026-06-19T10:00:00Z", "structured": null } }`,
    },
    {
      method: 'GET',
      path: '/me/resume',
      auth: 'cookie',
      summary: 'Your résumé status (enabled / present / uploaded_at).',
      description:
        'Always `200`: unconfigured storage or no résumé is a normal state. ' +
        '`structured` carries the read-only structured résumé, or null when none is ' +
        'current.',
      curl: `curl "${BASE_URL}/me/resume" -b cookies.txt`,
      responseExample: `{ "data": { "enabled": true, "present": true, "uploaded_at": "2026-06-19T10:00:00Z", "structured": { "...": "..." } } }`,
    },
    {
      method: 'DELETE',
      path: '/me/resume',
      auth: 'cookie',
      summary: 'Delete your stored résumé.',
      description: 'Returns `204 No Content`. `501` when object storage is unconfigured.',
      curl: `curl -X DELETE "${BASE_URL}/me/resume" -b cookies.txt`,
    },
    {
      method: 'PUT',
      path: '/me/resume/contacts',
      auth: 'cookie',
      summary: 'Override one or more contact-block fields on your profile.',
      description:
        'Each body field is owned per field: a value you set here takes over ' +
        'that field from whatever the structured résumé extract would otherwise ' +
        'contribute. Omit a field to leave it as-is; to clear rather than ' +
        'override, use `POST /me/resume/contacts/replace-from-cv`.',
      body: [
        { name: 'full_name', type: 'string', description: 'Overrides the extracted name.' },
        { name: 'email', type: 'string', description: 'Overrides the extracted email.' },
        { name: 'phone', type: 'string', description: 'Overrides the extracted phone.' },
        { name: 'location', type: 'string', description: 'Overrides the extracted location.' },
        { name: 'links', type: 'string[]', description: 'Overrides the extracted links.' },
        { name: 'headline', type: 'string', description: 'A one-line positioning statement.' },
        { name: 'summary', type: 'string', description: 'A short profile summary.' },
        { name: 'languages', type: 'string[]', description: 'Spoken/written languages.' },
        { name: 'certifications', type: 'string[]', description: 'Certifications to show.' },
        { name: 'education', type: 'object[]', description: 'Education entries to show.' },
      ],
      curl: `curl -X PUT "${BASE_URL}/me/resume/contacts" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"email":"me@work.com","phone":"+1 555 0100"}'`,
      responseExample: `{ "data": { "full_name": "Jane Doe", "email": "me@work.com", "phone": "+1 555 0100", "location": "Berlin, Germany" } }`,
    },
    {
      method: 'POST',
      path: '/me/resume/contacts/replace-from-cv',
      auth: 'cookie',
      summary: 'Reset every contact override from your current structured résumé.',
      description:
        'A full reset, not just identity fields — every owned field is replaced ' +
        'from the current structured extract. `409` when you have no structured ' +
        'résumé to copy from.',
      curl: `curl -X POST "${BASE_URL}/me/resume/contacts/replace-from-cv" -b cookies.txt`,
      responseExample: `{ "data": { "full_name": "Jane Doe", "email": "jane@example.com", "location": "Berlin, Germany" } }`,
    },
  ],
};
