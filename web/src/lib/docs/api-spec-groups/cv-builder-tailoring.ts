import { BASE_URL, type Group } from '../api-spec-types';

export const cvBuilderTailoringGroup: Group = {
  title: 'CV builder & tailoring',
  intro:
    'Your CVs, and the tailoring flow that reframes one toward a single ' +
    'vacancy. Authoring is browser-owned — create, update and delete are ' +
    'session-only — while reading a CV, patching it, and rendering the PDF also ' +
    'accept an API key, because that is the half a tailoring agent drives. Every ' +
    'CV is owner-scoped: an id you do not own is a 404, not a 403.',
  endpoints: [
    {
      method: 'GET',
      path: '/me/cvs',
      auth: 'cookie',
      summary: 'List your CVs, without their documents.',
      curl: `curl "${BASE_URL}/me/cvs" -b cookies.txt`,
      responseExample: `{ "data": [ { "id": "0f2c…", "title": "Backend engineer", "template_id": "classic", "created_at": "2026-07-01T10:00:00Z", "updated_at": "2026-07-20T18:30:00Z" } ] }`,
    },
    {
      method: 'GET',
      path: '/me/cvs/bases',
      auth: 'cookie',
      summary: 'List your base CVs, without their documents.',
      description:
        'The non-tailored CVs you can edit, duplicate and tailor from, oldest created first. ' +
        '`is_default` marks the one used when a tailor request names no base; `hand_edited` ' +
        'marks a base you changed in the editor, which a new CV upload will not overwrite. ' +
        'GET /me/cvs lists tailored copies and is unchanged.',
      curl: `curl "${BASE_URL}/me/cvs/bases" -b cookies.txt`,
      responseExample: `{ "data": [ { "id": "0f2c…", "title": "Backend engineer", "template_id": "classic-ats", "is_default": true, "hand_edited": false, "created_at": "2026-07-01T10:00:00Z", "updated_at": "2026-07-20T18:30:00Z" } ] }`,
    },
    {
      method: 'POST',
      path: '/me/cvs/{id}/base-session',
      auth: 'cookie',
      summary: 'Open the agent conversation for a base CV.',
      description:
        'Returns the assistant conversation bound to one of your base CVs, creating it on first use. ' +
        "It is the tailoring workspace's agent without a vacancy: it edits this general CV through the " +
        'same evidence-gated tools. Opening it is free and needs no paid plan; each message still ' +
        "passes the assistant's paid gate. 409 for a tailored copy (use the tailor session), 404 when " +
        'the id is not yours.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The base CV id.' }],
      curl: `curl -X POST "${BASE_URL}/me/cvs/0f2c…/base-session" -b cookies.txt`,
      responseExample: `{ "data": { "base_cv_id": "0f2c…", "session_id": "s_9f…" } }`,
    },
    {
      method: 'POST',
      path: '/me/cvs/{id}/duplicate',
      auth: 'cookie',
      summary: 'Copy a base CV.',
      description:
        'Creates a new base CV with the same document, template and appearance, titled ' +
        '"<title> (copy)". Only a base can be duplicated: 409 for a tailored copy, 404 when ' +
        'the id is not yours.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The base CV id.' }],
      curl: `curl -X POST "${BASE_URL}/me/cvs/0f2c…/duplicate" -b cookies.txt`,
      responseExample: `{ "data": { "id": "9a41…", "title": "Backend engineer (copy)", "template_id": "classic-ats", "document": { … } } }`,
    },
    {
      method: 'POST',
      path: '/me/cvs',
      auth: 'cookie',
      summary: 'Create a CV, optionally seeded from your stored history.',
      body: [
        { name: 'title', type: 'string', description: 'Label; up to 200 characters.' },
        { name: 'template_id', type: 'string', description: 'Which template to render with.', example: 'classic' },
        {
          name: 'seed',
          type: 'boolean',
          description: 'Pre-fill from your parsed CV / experience bank instead of starting blank.',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/me/cvs" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"title":"Backend engineer","seed":true}'`,
      responseExample: `{ "data": { "id": "0f2c…", "title": "Backend engineer", "template_id": "classic", "document": { "header": { … }, "experience": [ … ] } } }`,
    },
    {
      method: 'GET',
      path: '/me/cvs/{id}',
      auth: 'cookie-or-key',
      summary: 'One CV with its full document.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' }],
      curl: `curl "${BASE_URL}/me/cvs/0f2c…" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": { "id": "0f2c…", "title": "Backend engineer", "template_id": "classic", "agent_session_id": "s_9f…", "document": { … } } }`,
    },
    {
      method: 'PUT',
      path: '/me/cvs/{id}',
      auth: 'cookie',
      summary: 'Replace a CV — title, template and document.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' }],
      body: [
        { name: 'title', type: 'string', description: 'New label.' },
        { name: 'template_id', type: 'string', description: 'New template.' },
        { name: 'document', type: 'object', required: true, description: 'The whole CV document.' },
      ],
      curl: `curl -X PUT "${BASE_URL}/me/cvs/0f2c…" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"title":"Backend engineer","document":{…}}'`,
      responseExample: `{ "data": { "id": "0f2c…", "updated_at": "2026-07-28T20:10:00Z" } }`,
    },
    {
      method: 'PATCH',
      path: '/me/cvs/{id}',
      auth: 'cookie-or-key',
      summary: 'Apply a batch of edits, addressed by path.',
      description:
        'An edit is a `kind` (`set`, `insert`, `remove`, `move`) and a `path` into the CV — ' +
        '`summary`, `experience[2].bullets[1]`, `skills[0].items[3]`, `style.font_size`. ' +
        'Indices are 0-based. The whole batch applies or none of it does: an unknown path or ' +
        'an index past the end refuses the request without touching the CV. An API key edits ' +
        "as the tailoring agent, so the candidate's own fields are refused and a claim about " +
        "them needs `evidence_id` from the experience bank. Every request lands in the CV's " +
        'history and can be undone on its own.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' }],
      body: [
        { name: 'ops', type: 'object[]', required: true, description: 'The edits, applied in order.' },
        {
          name: 'ops[].kind',
          type: 'string',
          required: true,
          description: '`set`, `insert`, `remove` or `move`.',
          example: 'set',
        },
        {
          name: 'ops[].path',
          type: 'string',
          required: true,
          description: 'Where to edit.',
          example: 'experience[0].bullets[1]',
        },
        { name: 'ops[].value', type: 'any', description: 'The new content, for `set` and `insert`.' },
        { name: 'ops[].to', type: 'integer', description: "The element's new position, for `move`." },
        { name: 'ops[].evidence_id', type: 'string', description: 'The banked achievement this rests on.' },
        { name: 'note', type: 'string', description: 'One line on why, shown beside the entry in the history.' },
      ],
      curl: `curl -X PATCH "${BASE_URL}/me/cvs/0f2c…" \\
  -H "Authorization: Bearer fhk_…" \\
  -H 'Content-Type: application/json' \\
  -d '{"ops":[{"kind":"set","path":"experience[0].bullets[1]","value":"Cut p99 checkout latency 40% by …","evidence_id":"a71f…"}]}'`,
      responseExample: `{ "data": { "id": "0f2c…", "document": { … } } }`,
    },
    {
      method: 'PUT',
      path: '/me/cvs/{id}/template',
      auth: 'cookie',
      summary: 'Switch the template only.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' }],
      body: [
        { name: 'template_id', type: 'string', required: true, description: 'A template id from `/cv-templates`.' },
      ],
      curl: `curl -X PUT "${BASE_URL}/me/cvs/0f2c…/template" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"template_id":"compact"}'`,
      responseExample: `{ "data": { "id": "0f2c…", "template_id": "compact" } }`,
    },
    {
      method: 'DELETE',
      path: '/me/cvs/{id}',
      auth: 'cookie',
      summary: 'Delete a CV.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' }],
      curl: `curl -X DELETE "${BASE_URL}/me/cvs/0f2c…" -b cookies.txt`,
    },
    {
      method: 'GET',
      path: '/me/cvs/{id}/pdf',
      auth: 'cookie-or-key',
      summary: 'Render the CV to PDF.',
      description: 'Answers 501 on a deployment with no typst binary configured; everything else still works.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' }],
      curl: `curl "${BASE_URL}/me/cvs/0f2c…/pdf" -H "Authorization: Bearer fhk_…" -o cv.pdf`,
    },
    {
      method: 'GET',
      path: '/cv-templates',
      auth: 'cookie',
      summary: 'The template gallery.',
      curl: `curl "${BASE_URL}/cv-templates" -b cookies.txt`,
      responseExample: `{ "data": [ { "id": "classic", "name": "Classic" }, { "id": "compact", "name": "Compact" } ] }`,
    },
    {
      method: 'GET',
      path: '/cv-fonts',
      auth: 'cookie',
      summary: 'The typefaces a CV may use.',
      description:
        "Read this rather than hard-coding a list: an unregistered `document.style.font_family` is dropped on save and the CV renders in the template's own face. `note` names the familiar face the entry matches; `css` is a font stack for rendering a preview in a browser.",
      curl: `curl "${BASE_URL}/cv-fonts" -b cookies.txt`,
      responseExample: `{ "data": [ { "id": "tinos", "label": "Tinos", "note": "Times New Roman metrics", "css": "Tinos, \\"Times New Roman\\", Times, serif" } ] }`,
    },
    {
      method: 'POST',
      path: '/me/cvs/tailor',
      auth: 'cookie',
      summary: 'Start tailoring a CV toward one vacancy.',
      description:
        'Copies your base CV into a vacancy-bound tailored one and mints the ' +
        'short-lived credential its agent session runs on. Requires a cached match ' +
        'analysis for the job (409 otherwise) and a base CV — one is seeded from ' +
        'your stored history when you have none, and it is a 409 when there is ' +
        'nothing to seed from. Calls no LLM itself.',
      body: [
        { name: 'job_slug', type: 'string', required: true, description: 'The vacancy to tailor toward.' },
        {
          name: 'base_cv_id',
          type: 'string (uuid)',
          description:
            'Which of your base CVs to copy. Optional: omitted means your default base (the one ' +
            'your most recent tailored CV was made from, else your oldest). 404 when it is not ' +
            'yours, 409 when it is a tailored copy. A vacancy you already tailored reopens its ' +
            'existing copy whatever base is named.',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/me/cvs/tailor" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"job_slug":"senior-backend-engineer-acme-1a2b"}'`,
      responseExample: `{ "data": { "tailor_cv_id": "7d1a…", "base_cv_id": "0f2c…", "session_id": "s_9f…", "analysis": { "overall_score": 72, "verdict": "worth_applying" } } }`,
    },
    {
      method: 'POST',
      path: '/me/cvs/{id}/tailor-session',
      auth: 'cookie',
      summary: 'Mint a fresh agent session for an existing tailored CV.',
      description: 'For a tailored CV whose session was lost or never bound. 409 when the CV is not a tailored copy.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The tailored CV id.' }],
      curl: `curl -X POST "${BASE_URL}/me/cvs/7d1a…/tailor-session" -b cookies.txt`,
      responseExample: `{ "data": { "tailor_cv_id": "7d1a…", "base_cv_id": "0f2c…", "session_id": "s_9f…" } }`,
    },
    {
      method: 'POST',
      path: '/me/cvs/base/reseed',
      auth: 'cookie',
      summary: 'Rebuild your base CV from your current seed.',
      description:
        'Replaces the base (non-tailored) document from the current seed ' +
        "(experience bank first, your résumé's own extract for what the bank doesn't " +
        'track). Preserves template and typography. Does not rewrite tailored copies ' +
        'for specific jobs. 409 when there is no usable seed.',
      curl: `curl -X POST "${BASE_URL}/me/cvs/base/reseed" -b cookies.txt`,
      responseExample: `{ "data": { "id": "0f2c…", "title": "My CV", "template_id": "classic-ats", "document": { … } } }`,
    },
    {
      method: 'POST',
      path: '/me/cvs/{id}/reseed',
      auth: 'cookie',
      summary: 'Rebuild one CV from your current seed.',
      description:
        "Replaces this CV's content from the current seed " +
        "(experience bank first, your résumé's own extract for what the bank doesn't " +
        'track). Works on a tailored copy or on a base, and touches only that CV: refreshing a ' +
        "tailored copy no longer rewrites any base, and refreshing a base — the editor's " +
        '"Update from my CV", which replaces hand edits — leaves its copies alone. Keeps the same ' +
        'id and agent session; preserves template and typography. 409 when ' +
        'there is no usable seed. Upload alone does not do ' +
        'this — this is the explicit apply.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The CV id (tailored or base).' }],
      curl: `curl -X POST "${BASE_URL}/me/cvs/7d1a…/reseed" -b cookies.txt`,
      responseExample: `{ "data": { "id": "7d1a…", "title": "Tailored for …", "template_id": "classic-ats", "document": { … } } }`,
    },
    {
      method: 'GET',
      path: '/me/cvs/{id}/tailor-context',
      auth: 'cookie-or-key',
      summary: 'The match analysis a tailored CV should reframe toward.',
      description:
        'The split that keeps tailoring honest: `missing_have` are requirements ' +
        'your history already covers but the CV buries — reframe those — and ' +
        '`missing_gap` are the ones it does not, which an agent must ask about ' +
        'rather than invent. Served from cache; calls no LLM. 409 when the CV is ' +
        'not a tailored copy or has no analysis. `job.requirements` is what the posting ' +
        'itself asks for, in its own words — a fuller list than the missing_* split, ' +
        'which is only what the analysis found the CV lacks. Absent when the posting ' +
        'states none.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The tailored CV id.' }],
      curl: `curl "${BASE_URL}/me/cvs/7d1a…/tailor-context" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{
  "data": {
    "job": {
      "slug": "senior-backend-engineer-acme-1a2b", "title": "Senior Backend Engineer", "company": "Acme",
      "requirements": [ { "text": "5+ years with Go", "priority": "required" } ]
    },
    "verdict": "worth_applying",
    "overall_score": 72,
    "recommendation": "Lead with the payments migration …",
    "missing_have": [ { "requirement": "Kafka at scale", "evidence": "Ran the event bus at …" } ],
    "missing_gap": [ { "requirement": "Kubernetes operators" } ],
    "strengths": [ "Go", "payments" ],
    "gaps": [ "k8s operators" ]
  }
}`,
    },
    {
      method: 'PUT',
      path: '/me/cvs/{id}/session',
      auth: 'cookie-or-key',
      summary: 'Bind an agent session to a CV.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' }],
      body: [{ name: 'session_id', type: 'string', required: true, description: 'The agent session to bind.' }],
      curl: `curl -X PUT "${BASE_URL}/me/cvs/7d1a…/session" -H "Authorization: Bearer fhk_…" \\
  -H 'Content-Type: application/json' -d '{"session_id":"s_9f…"}'`,
      responseExample: `{ "data": { "id": "7d1a…", "agent_session_id": "s_9f…" } }`,
    },
    {
      method: 'PUT',
      path: '/me/cvs/{id}/tracer-links',
      auth: 'cookie',
      summary: "Turn link tracing on or off for this CV's PDF.",
      description:
        'Off is the default for every CV. Turning it on is refused with a `409` ' +
        'on a deployment with no visitor salt configured — there would be no ' +
        'honest way to count distinct visitors. Turning it off is never refused.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' }],
      body: [{ name: 'enabled', type: 'boolean', required: true, description: 'Turn tracing on or off.' }],
      curl: `curl -X PUT "${BASE_URL}/me/cvs/0f2c…/tracer-links" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"enabled":true}'`,
      responseExample: `{ "data": { "tracer_links_enabled": true } }`,
    },
    {
      method: 'GET',
      path: '/me/cvs/{id}/tracer-links',
      auth: 'cookie',
      summary: "What is known about this CV's traced links.",
      description:
        'One entry per link the PDF carries (header links, project links). ' +
        'Clicks you made on your own CV are excluded from the counts; ' +
        '`bot_clicks` is tallied separately rather than folded in. ' +
        '`distinct_visitors` is omitted — not zero — on a deployment with no ' +
        'visitor salt, since there is no honest count to report.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' }],
      curl: `curl "${BASE_URL}/me/cvs/0f2c…/tracer-links" -b cookies.txt`,
      responseExample: `{
  "data": [
    {
      "source_path": "header.links[0]",
      "destination_url": "https://github.com/you",
      "traced_url": "https://freehire.me/cv/acme-x7abc",
      "clicks": 12,
      "bot_clicks": 3,
      "distinct_visitors": 9,
      "last_click_at": "2026-07-20T10:00:00Z"
    }
  ]
}`,
    },
    {
      method: 'GET',
      path: '/me/cvs/{id}/revisions',
      auth: 'cookie',
      summary: 'The edit history of this CV, newest first.',
      description:
        'Each entry names what changed and where (`paths`), not the edit ' +
        'itself — the operations carry your own prior text and stay ' +
        'server-side. `undoable` is false for an entry with nothing to reverse ' +
        '(e.g. the CV’s own creation); `reverted` marks an entry already undone ' +
        'rather than removing it from the feed.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' }],
      curl: `curl "${BASE_URL}/me/cvs/0f2c…/revisions" -b cookies.txt`,
      responseExample: `{
  "data": [
    {
      "id": "b1a2…",
      "actor": "agent",
      "origin": "tailoring",
      "batch_id": "f3c1…",
      "title": "Reframed 2 bullets toward Kafka",
      "note": "Emphasized event-bus scale to match the requirement",
      "paths": ["experience[0].bullets[1]"],
      "reverted": false,
      "undoable": true,
      "created_at": "2026-07-28T20:10:00Z"
    }
  ]
}`,
    },
    {
      method: 'POST',
      path: '/me/cvs/{id}/revisions/{rid}/undo',
      auth: 'cookie',
      summary: 'Reverse one revision, leaving later edits in place.',
      description:
        'The undo is itself recorded as a new revision. A `409` when the entry ' +
        'has nothing to reverse or the part of the document it changed is ' +
        'already gone.',
      pathParams: [
        { name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' },
        {
          name: 'rid',
          type: 'string (uuid)',
          required: true,
          description: 'The revision id, from the history feed.',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/me/cvs/0f2c…/revisions/b1a2…/undo" -b cookies.txt`,
      responseExample: `{
  "data": {
    "cv": { "id": "0f2c…", "title": "Backend engineer", "template_id": "classic", "created_at": "2026-07-01T10:00:00Z", "updated_at": "2026-07-28T20:15:00Z" },
    "revision": { "id": "c4d5…", "actor": "user", "origin": "editor", "title": "Undid \\"Reframed 2 bullets toward Kafka\\"", "reverts_id": "b1a2…", "reverted": false, "undoable": false, "paths": ["experience[0].bullets[1]"], "created_at": "2026-07-28T20:15:00Z" }
  }
}`,
    },
    {
      method: 'POST',
      path: '/me/cvs/{id}/revisions/batch/{bid}/undo',
      auth: 'cookie',
      summary: 'Reverse every standing edit of one agent turn, newest first.',
      description:
        '`bid` is a revision’s `batch_id` from the history feed, not a separate ' +
        'id — undoing one entry of a batch and then the batch itself is ' +
        'well-defined, since an already-reverted entry is simply skipped.',
      pathParams: [
        { name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' },
        {
          name: 'bid',
          type: 'string (uuid)',
          required: true,
          description: 'The batch id shared by the revisions to undo.',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/me/cvs/0f2c…/revisions/batch/f3c1…/undo" -b cookies.txt`,
      responseExample: `{ "data": { "id": "0f2c…", "title": "Backend engineer", "template_id": "classic", "created_at": "2026-07-01T10:00:00Z", "updated_at": "2026-07-28T20:16:00Z" } }`,
    },
    {
      method: 'GET',
      path: '/me/cvs/{id}/ats-delta',
      auth: 'cookie',
      summary: "How tailoring changed this CV's ATS-readiness score.",
      description:
        'The tailored copy scored against the base CV it came from, with ' +
        'template, page margins and typography held identical so the ' +
        'difference reflects content alone — the base is a copy rendered with ' +
        'the tailored copy’s formatting, never the stored base itself. ' +
        'Recomputed per request, never stored. `available: false` (with ' +
        '`reason`, no `delta`) when rendering is unavailable rather than an ' +
        'error. `409` when the CV is not a tailored copy, has no base CV to ' +
        'compare against, or its vacancy no longer exists.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The tailored CV id.' }],
      curl: `curl "${BASE_URL}/me/cvs/7d1a…/ats-delta" -b cookies.txt`,
      responseExample: `{
  "data": {
    "available": true,
    "base_cv_id": "0f2c…",
    "delta": {
      "base": 62,
      "tailored": 78,
      "change": 16,
      "categories": [ { "id": "keyword_coverage", "label": "Keyword coverage", "...": "..." } ],
      "regressed": false
    }
  }
}`,
    },
    {
      method: 'GET',
      path: '/me/cvs/{id}/job-match',
      auth: 'cookie',
      summary: 'How well a tailored CV matches the vacancy it was written for.',
      description:
        'Deterministic and free — no model call, unlike the AI match analysis. ' +
        'Scored off the CV’s rendered PDF text layer against the vacancy alone ' +
        '(no base-CV comparison, unlike the ATS delta), so it is cheap enough ' +
        'to refresh after every saved edit. `available: false` when nothing ' +
        'about the vacancy could be matched automatically, or rendering ' +
        'failed. `409` when the CV is not a tailored copy or its vacancy no ' +
        'longer exists.',
      pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The tailored CV id.' }],
      curl: `curl "${BASE_URL}/me/cvs/7d1a…/job-match" -b cookies.txt`,
      responseExample: `{
  "data": {
    "available": true,
    "score": {
      "overall": 74,
      "categories": [ { "...": "..." } ],
      "contributing": ["skills", "requirements"],
      "matched_skills": ["go", "postgresql"],
      "missing_skills": ["kubernetes"],
      "requirements": [ { "text": "5+ years Go", "priority": "required", "cached_status": "covered" } ]
    }
  }
}`,
    },
  ],
};
