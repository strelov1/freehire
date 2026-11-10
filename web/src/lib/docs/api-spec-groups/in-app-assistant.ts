import { BASE_URL, type Group } from '../api-spec-types';

export const inAppAssistantGroup: Group = {
  title: 'In-app assistant',
  intro:
    'The same agent the web app and browser extension chat with, over HTTP: create a ' +
    'conversation, then post messages to it. All routes accept the session cookie or an ' +
    'API key (`autopilot` is cookie-only — see below) and act on a session you own; ' +
    'someone else’s session id answers `404`, same as one that never existed. Turns run ' +
    'one at a time per session — a message sent while one is already running queues ' +
    'rather than running alongside it. `preset` selects which conversation this is: ' +
    '`chat` (default), `profile` (the experience interviewer), `browse` (a browsing ' +
    'session held from the extension’s side panel), `interview` (a mock-interview ' +
    'rehearsal), `debrief` (after a real one) or `apply` (the conversation about one ' +
    'automatic application) — the last three must name the vacancy via ' +
    '`?job=<slug>`, an application you already hold. An application holds one `apply` ' +
    'conversation: asking again returns it with `200` instead of creating a second. A `tailor` session cannot be created ' +
    'here; it is minted by the CV tailoring bootstrap, which knows the CV and vacancy to ' +
    'bind it to.',
  endpoints: [
    {
      method: 'POST',
      path: '/assistant/sessions',
      auth: 'cookie-or-key',
      summary: 'Start a new conversation.',
      description:
        'For shortlisting the current jobs list, pass search_query (the complete serialized query; an empty string is valid) and a client-generated request_id UUID. Only chat sessions accept this binding. Reusing the UUID with the same owner and canonical query returns the same session; changing its query returns 409. Unknown search parameters return 400 at creation. A condition that cannot be enforced when a search runs (for example hide_seen without a readable history) fails that search instead, and the assistant reports it rather than widening the search. Search-bound responses include search_context: { query, brief }; the original query is immutable and the selection brief is local to the conversation.',
      body: [
        {
          name: 'search_query',
          type: 'string',
          description:
            'Optional complete job-search query, at most 8000 bytes. Pagination and tracking parameters are removed.',
          example: 'skills=go&salary_min=80000&work_mode=remote',
        },
        {
          name: 'request_id',
          type: 'string (UUID)',
          description: 'Required with search_query. Reuse the same UUID when retrying creation.',
          example: 'b2f1c2b0-6e2a-4c9e-9c2e-0a1b2c3d4e5f',
        },
      ],
      query: [
        {
          name: 'preset',
          type: 'string',
          description: 'One of `chat` (default), `profile`, `browse`, `interview`, `debrief`, `apply`.',
          example: 'chat',
        },
        {
          name: 'job',
          type: 'string',
          description:
            'Public slug of an application you hold. Required for `interview`/`debrief`/`apply`, ignored otherwise.',
          example: 'senior-go-engineer-acme-1a2b',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/assistant/sessions?preset=chat" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": { "id": "b2f1c2b0-6e2a-4c9e-9c2e-0a1b2c3d4e5f", "preset": "chat", "label": "" } }`,
    },
    {
      method: 'GET',
      path: '/assistant/sessions',
      auth: 'cookie-or-key',
      summary: 'List your chat conversations, newest activity first.',
      description: 'Tailoring conversations are not chats — each belongs to a CV — so they never appear here.',
      curl: `curl "${BASE_URL}/assistant/sessions" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{
  "data": [
    { "id": "b2f1c2b0-6e2a-4c9e-9c2e-0a1b2c3d4e5f", "preset": "chat", "label": "Relocating to Berlin?" }
  ],
  "meta": { "total": 1 }
}`,
    },
    {
      method: 'GET',
      path: '/assistant/sessions/{id}',
      auth: 'cookie-or-key',
      summary: 'One owned conversation with its full transcript.',
      pathParams: [
        {
          name: 'id',
          type: 'string (UUID)',
          required: true,
          description: 'The session id.',
          example: 'b2f1c2b0-6e2a-4c9e-9c2e-0a1b2c3d4e5f',
        },
      ],
      curl: `curl "${BASE_URL}/assistant/sessions/<id>" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{
  "data": {
    "session": { "id": "b2f1c2b0-6e2a-4c9e-9c2e-0a1b2c3d4e5f", "preset": "chat", "label": "Relocating to Berlin?" },
    "messages": [
      { "seq": 1, "role": "user", "content": { "text": "Should I relocate for this role?" } },
      { "seq": 2, "role": "assistant", "content": { "text": "...", "...": "..." } }
    ]
  }
}`,
    },
    {
      method: 'DELETE',
      path: '/assistant/sessions/{id}',
      auth: 'cookie-or-key',
      summary: 'Delete an owned conversation and its transcript.',
      description: 'Returns `204 No Content`.',
      pathParams: [{ name: 'id', type: 'string (UUID)', required: true, description: 'The session id.' }],
      curl: `curl -X DELETE "${BASE_URL}/assistant/sessions/<id>" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
    },
    {
      method: 'POST',
      path: '/assistant/sessions/{id}/messages',
      auth: 'cookie-or-key',
      summary: 'Send a message and stream the turn as Server-Sent Events.',
      description:
        'Body is `{"text": "..."}` (max ~8000 characters, deployment-configured). The ' +
        'response is `text/event-stream`, not JSON: each frame is `event: <kind>` plus a ' +
        '`data:` line carrying the JSON event (whose own `type` field repeats the kind). ' +
        'Kinds: `queued` (this turn waited for one already running), `user_prompt`, ' +
        '`assistant_text` / `assistant_thought` (streamed deltas), `tool_use` / ' +
        '`tool_result`, `usage`, and exactly one terminal `result` carrying `stop_reason` ' +
        '(`completed`, `cancelled`, or `error`). The turn keeps running even if you stop ' +
        'reading — stopping it is a separate call to `.../cancel`.',
      pathParams: [{ name: 'id', type: 'string (UUID)', required: true, description: 'The session id.' }],
      body: [
        {
          name: 'text',
          type: 'string',
          required: true,
          description: 'Your message.',
          example: 'Should I relocate for this role?',
        },
      ],
      curl: `curl -N -X POST "${BASE_URL}/assistant/sessions/<id>/messages" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" -H 'Content-Type: application/json' \\
  -d '{"text":"Should I relocate for this role?"}'`,
      responseExample: `event: user_prompt
data: {"type":"user_prompt","text":"Should I relocate for this role?"}

event: assistant_text
data: {"type":"assistant_text","text":"Based on the salary range..."}

event: result
data: {"type":"result","stop_reason":"completed"}

`,
    },
    {
      method: 'POST',
      path: '/assistant/sessions/{id}/cancel',
      auth: 'cookie-or-key',
      summary: 'Stop the session’s running turn.',
      description:
        'A no-op (still `204`) when nothing is running — you cannot see whether a turn is live before asking.',
      pathParams: [{ name: 'id', type: 'string (UUID)', required: true, description: 'The session id.' }],
      curl: `curl -X POST "${BASE_URL}/assistant/sessions/<id>/cancel" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
    },
    {
      method: 'POST',
      path: '/assistant/sessions/{id}/opening',
      auth: 'cookie-or-key',
      summary: 'Have the assistant speak first, on an `interview`/`debrief` session.',
      description:
        'Streams one turn under a server-side brief (no body), the same SSE shape as ' +
        '`.../messages`. `409` if the session’s preset does not open by itself, or if it ' +
        'already has an assistant message (an opening cannot be re-run).',
      pathParams: [{ name: 'id', type: 'string (UUID)', required: true, description: 'The session id.' }],
      curl: `curl -N -X POST "${BASE_URL}/assistant/sessions/<id>/opening" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
    },
    {
      method: 'POST',
      path: '/assistant/sessions/{id}/sync',
      auth: 'cookie-or-key',
      summary: 'Catch an `apply` session up on its automatic application.',
      description:
        'When the application has events the conversation has not narrated, streams one turn ' +
        'telling the candidate what changed (no body), the same SSE shape as `.../messages`; ' +
        'otherwise `204` and nothing runs. Not charged to the assistant-message allowance. ' +
        'A turn that fails is narrated again on the next call. `409` for a session that is ' +
        'not an `apply` conversation.',
      pathParams: [{ name: 'id', type: 'string (UUID)', required: true, description: 'The session id.' }],
      curl: `curl -N -X POST "${BASE_URL}/assistant/sessions/<id>/sync" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
    },
    {
      method: 'POST',
      path: '/assistant/sessions/{id}/retry',
      auth: 'cookie-or-key',
      summary: 'Resume after a failed turn, without adding another user message.',
      description:
        'Re-runs the loop over the existing history (no body) — the same SSE shape as ' +
        '`.../messages`. `409 "nothing to retry"` when the session has no prior user message.',
      pathParams: [{ name: 'id', type: 'string (UUID)', required: true, description: 'The session id.' }],
      curl: `curl -N -X POST "${BASE_URL}/assistant/sessions/<id>/retry" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
    },
    {
      method: 'POST',
      path: '/assistant/sessions/{id}/extend',
      auth: 'cookie-or-key',
      summary: 'Buy a CV editing session another ceiling’s worth of turns.',
      description:
        'A CV editing session is bounded by a turn ceiling as well as by the daily session ' +
        'allowance, and a turn past the ceiling is a `402` naming the session. This spends ' +
        'another of the day’s CV editing sessions to raise it (no body), and is idempotent ' +
        'under a double click — two calls in flight buy one ceiling, not two. `409` on any ' +
        'preset other than a CV editing session: a chat is bounded by the daily assistant ' +
        'allowance, and a day cannot be topped up. `402` when there is no session left to ' +
        'spend, with the same body every refusal carries.',
      pathParams: [{ name: 'id', type: 'string (UUID)', required: true, description: 'The session id.' }],
      curl: `curl -X POST "${BASE_URL}/assistant/sessions/<id>/extend" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{
  "data": {
    "turns": 15,
    "ceiling": 30,
    "unlimited": false,
    "allowance": { "feature": "tailor", "used": 2, "limit": 2, "unlimited": false, "enforced": false, "resets_at": "2026-09-01T00:00:00Z" }
  }
}`,
    },
    {
      method: 'POST',
      path: '/assistant/sessions/{id}/voice-token',
      auth: 'cookie-or-key',
      summary: 'Mint a short-lived credential for a hands-free voice call.',
      description:
        'Only on an `interview` session; `409` otherwise. The audio itself never reaches ' +
        'this server: take the returned ' +
        '`value` to `calls_url` (this deployment’s own realtime gateway, not api.openai.com ' +
        'directly) to negotiate the WebRTC call. `501` when voice mode is not configured.',
      pathParams: [{ name: 'id', type: 'string (UUID)', required: true, description: 'The session id.' }],
      curl: `curl -X POST "${BASE_URL}/assistant/sessions/<id>/voice-token" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": { "value": "ek_...", "model": "gpt-realtime", "calls_url": "https://freehire.me/api/v1/realtime/calls" } }`,
    },
    {
      method: 'POST',
      path: '/assistant/sessions/{id}/voice-turns',
      auth: 'cookie-or-key',
      summary: 'Append one completed spoken exchange to the transcript.',
      description:
        'Only on an `interview` session. Lets a call continued by typing carry the spoken ' +
        'turns forward in context. Returns `204 No Content`.',
      pathParams: [{ name: 'id', type: 'string (UUID)', required: true, description: 'The session id.' }],
      body: [
        { name: 'role', type: 'string', required: true, description: '`user` or `assistant`.', example: 'user' },
        { name: 'content', type: 'string', required: true, description: 'The transcribed/spoken text.' },
      ],
      curl: `curl -X POST "${BASE_URL}/assistant/sessions/<id>/voice-turns" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" -H 'Content-Type: application/json' \\
  -d '{"role":"user","content":"I led the migration to Kubernetes."}'`,
    },
    {
      method: 'POST',
      path: '/assistant/sessions/{id}/autopilot',
      auth: 'cookie',
      summary: 'Run an unattended CV-tailoring pass as one long streamed turn.',
      description:
        'Session-only, not API-key — an unattended run edits a CV, and the browser is the ' +
        'only place you can watch it happen and undo it. Requires a `tailor` session bound ' +
        'to both a CV and a vacancy (`409` otherwise). Same SSE shape as `.../messages`, no ' +
        'body; every edit lands in one revision batch, so undoing the run is reverting that ' +
        'batch.',
      pathParams: [{ name: 'id', type: 'string (UUID)', required: true, description: 'The session id.' }],
      curl: `curl -N -X POST "${BASE_URL}/assistant/sessions/<id>/autopilot" -b cookies.txt`,
    },
  ],
};
