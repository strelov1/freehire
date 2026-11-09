import { BASE_URL, type Group } from '../api-spec-types';

export const applicationMailGroup: Group = {
  title: 'Application mail',
  intro:
    'Your job mail, and the link between a message and the application it ' +
    'belongs to. Mail arrives three ways: a connected Gmail account, the hosted ' +
    'freehire address, or a batch your own client pushed. Only the first two are ' +
    'classified by freehire — pushed mail is yours to triage, which is what makes ' +
    'that tier free. Everything here takes a full-scope API key, so a harness can ' +
    'drive the whole surface; the Gmail consent redirect is the one exception and ' +
    'stays browser-only.',
  endpoints: [
    {
      method: 'GET',
      path: '/me/tracking/{slug}/followup',
      auth: 'cookie-or-key',
      summary: 'Draft a chase for a silent application.',
      description:
        'Offered only when the application is in the silent state the tracking ' +
        'board’s own badge uses — a `409` otherwise. `recipient`/`recipient_name` ' +
        'are omitted when nobody at the company ever replied, which is the common ' +
        'case: the draft is handed to you, nothing is sent.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      curl: `curl "${BASE_URL}/me/tracking/<slug>/followup" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": { "subject": "Following up: Senior Go Engineer application", "body": "Hi …", "recipient": "recruiting@acme.com", "recipient_name": "Acme Recruiting", "days_silent": 12 } }`,
    },
    {
      method: 'POST',
      path: '/me/tracking/{slug}/followup',
      auth: 'cookie-or-key',
      summary: 'Record that you sent a chase.',
      description:
        'Does not itself affect the silence calculation — that reads when the ' +
        'other side last moved, and a chase is not a reply. Idempotent within the ' +
        'hour: a double click overwrites the timestamp rather than logging a ' +
        'second event. Returns `204 No Content`.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      curl: `curl -X POST "${BASE_URL}/me/tracking/<slug>/followup" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
    },
    {
      method: 'POST',
      path: '/me/tracking/{slug}/mail-recall',
      auth: 'cookie-or-key',
      summary: 'Sweep your connected mailbox for mail belonging to this application.',
      description:
        'For an application whose mail arrived before you connected an inbox, or ' +
        'that a sync missed. Runs an LLM pass over your mailbox and returns ' +
        'matches as suggestions — it never links or writes the ledger itself; ' +
        'confirm a pick with `POST /me/tracking/{slug}/mail-recall/link`. ' +
        'A `503` when no mail gateway is configured, a `502` when ' +
        'the model call fails.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      curl: `curl -X POST "${BASE_URL}/me/tracking/<slug>/mail-recall" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{
  "data": {
    "scanned": 340,
    "suggested": [
      { "id": 0, "provider_id": "18f2a…", "from_addr": "recruiting@acme.com", "from_name": "Acme Recruiting", "subject": "Interview invitation", "received_at": "2026-07-15T09:00:00Z", "invitation": true }
    ],
    "invitations": 1
  }
}`,
    },
    {
      method: 'POST',
      path: '/me/tracking/{slug}/mail-recall/link',
      auth: 'cookie-or-key',
      summary: 'Import and link one message a mail-recall sweep proposed.',
      description:
        'Imports the message from your mailbox by `provider_id` (from a ' +
        'mail-recall response) and links it to the application in one step — the ' +
        'same import-then-link path a Gmail sync uses, so the response is the ' +
        'full message body, the same shape `POST /me/emails/{id}/link` returns.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
      body: [
        {
          name: 'provider_id',
          type: 'string',
          required: true,
          description: 'The message’s `provider_id`, from a mail-recall response.',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/me/tracking/<slug>/mail-recall/link" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" -H 'Content-Type: application/json' \\
  -d '{"provider_id":"18f2a…"}'`,
      responseExample: `{ "data": { "id": 4822, "subject": "Interview invitation", "linked_slug": "senior-go-engineer-acme-1a2b", "link_source": "manual", "...": "..." } }`,
    },
    {
      method: 'GET',
      path: '/me/inbox',
      auth: 'cookie-or-key',
      summary: 'List your mail, newest first, excluding deleted.',
      description:
        "`?body=1` returns each message's readable text inline and marks nothing " +
        'read — that is the agent read path, so sweeping a backlog never zeroes ' +
        "the owner's unread count (`GET /me/emails/{id}` does mark read). Pages " +
        'carrying bodies are capped at 50. An unknown `source`, `status` or `link` ' +
        'value is a 400, never a silently empty page.',
      query: [
        { name: 'source', type: 'string', description: 'One account: `gmail`, `hosted`, or `external`.' },
        {
          name: 'status',
          type: 'string',
          description: 'One classified label (see the triage vocabulary below).',
          example: 'rejection',
        },
        {
          name: 'unclassified',
          type: 'boolean',
          description: "Only mail nothing has judged yet — the agent's work queue.",
        },
        { name: 'unread', type: 'boolean', description: 'Only messages you have not opened.' },
        {
          name: 'link',
          type: 'string',
          description: 'One link state: `linked`, `suggested` (awaiting your word), or `unlinked`.',
        },
        { name: 'q', type: 'string', description: 'Match subject, sender, or body.' },
        {
          name: 'include_other',
          type: 'boolean',
          description: 'Also list the `other` label the default hides. Asking for it by `status=other` implies this.',
        },
        { name: 'body', type: 'boolean', description: 'Include each message body; caps the page at 50.' },
        { name: 'limit', type: 'integer', description: 'Page size.' },
        { name: 'offset', type: 'integer', description: 'Page offset.' },
      ],
      curl: `curl "${BASE_URL}/me/inbox?unclassified=1&body=1&limit=50" \\
  -H "Authorization: Bearer fhk_…"`,
      responseExample: `{
  "data": [
    {
      "id": 4821,
      "source": "gmail",
      "external_id": "<CAF...@mail.gmail.com>",
      "from_addr": "no-reply@us.greenhouse-mail.io",
      "from_name": "Speechify Recruiting",
      "subject": "Thank you for applying to Speechify",
      "snippet": "We have received your application…",
      "received_at": "2026-07-24T09:12:00Z",
      "read": false,
      "status_signal": "acknowledgement",
      "link_source": "thread",
      "linked_slug": "tech-lead-web-core-speechify-a1b2"
    }
  ],
  "meta": { "total": 137, "limit": 50, "offset": 0 }
}`,
    },
    {
      method: 'GET',
      path: '/me/emails/{id}',
      auth: 'cookie-or-key',
      summary: 'One message in full. Marks it read.',
      description:
        'Use `GET /me/inbox?body=1` instead when sweeping a backlog — `read_at` ' +
        'means "a human saw this", and this endpoint sets it.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The message id.', example: '4821' }],
      curl: `curl "${BASE_URL}/me/emails/4821" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": { "id": 4821, "subject": "Thank you for applying to Speechify", "body_text": "Dear …", "status_signal": "acknowledgement", "linked_slug": "tech-lead-web-core-speechify-a1b2" } }`,
    },
    {
      method: 'POST',
      path: '/me/emails',
      auth: 'cookie-or-key',
      summary: 'Push a batch your own mail client fetched.',
      description:
        'Stored under source `external`. freehire provides no transport here and ' +
        'never classifies this mail — that is the point of the tier. `external_id` ' +
        '(the Message-ID in practice) is the dedup key: re-pushing the same id ' +
        'updates that message instead of storing a copy, so a nightly re-sync is ' +
        'safe. Content columns are refreshed; your read, deleted and triage state ' +
        'is not, so a re-sync cannot resurrect deleted mail or wipe a verdict. At ' +
        'most 100 messages per call, applied as one transaction — an oversized ' +
        'batch is refused, never truncated.',
      body: [
        { name: 'messages', type: 'array', required: true, description: 'Up to 100 messages.' },
        {
          name: 'messages[].external_id',
          type: 'string',
          required: true,
          description: 'Dedup key; the Message-ID header.',
          example: '<CAF...@mail.gmail.com>',
        },
        { name: 'messages[].thread_id', type: 'string', description: 'Provider thread id, if you have one.' },
        { name: 'messages[].from_addr', type: 'string', description: 'Sender address.' },
        {
          name: 'messages[].from_name',
          type: 'string',
          description: 'Sender display name — the matcher reads the company name from here.',
        },
        { name: 'messages[].subject', type: 'string', description: 'Subject line.' },
        { name: 'messages[].body_text', type: 'string', description: 'Plain-text body.' },
        { name: 'messages[].body_html', type: 'string', description: 'HTML body; used when there is no text part.' },
        { name: 'messages[].received_at', type: 'string (RFC3339)', description: 'When the message arrived.' },
      ],
      curl: `curl -X POST "${BASE_URL}/me/emails" \\
  -H "Authorization: Bearer fhk_…" \\
  -H 'Content-Type: application/json' \\
  -d '{"messages":[{"external_id":"<CAF...@mail.gmail.com>","from_addr":"no-reply@us.greenhouse-mail.io","subject":"Thank you for applying","received_at":"2026-07-24T09:12:00Z"}]}'`,
      responseExample: `{ "data": { "inserted": 1, "updated": 0 } }`,
    },
    {
      method: 'POST',
      path: '/me/emails/{id}/triage',
      auth: 'cookie-or-key',
      summary: 'Record what a message is, and optionally which application it belongs to.',
      description:
        'The vocabulary is closed: `acknowledgement`, `screening`, ' +
        '`interview_invitation`, `assessment`, `offer`, `rejection`, ' +
        '`info_request`, `incomplete_application`, `other`. Anything else is ' +
        'refused. A confident verdict on a linked message advances the ' +
        "application's stage — strictly forward, never out of a settled stage, and " +
        'a rejection never advances anything by itself.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The message id.' }],
      body: [
        {
          name: 'signal',
          type: 'string',
          required: true,
          description: 'One value from the vocabulary above.',
          example: 'interview_invitation',
        },
        { name: 'slug', type: 'string', description: 'Link the message to this application at the same time.' },
        {
          name: 'confidence',
          type: 'number',
          description:
            'Your confidence in the verdict, 0–1. Below the stage threshold the message is classified but the card does not move.',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/me/emails/4821/triage" \\
  -H "Authorization: Bearer fhk_…" \\
  -H 'Content-Type: application/json' \\
  -d '{"signal":"interview_invitation","slug":"tech-lead-web-core-speechify-a1b2","confidence":0.9}'`,
      responseExample: `{ "data": { "id": 4821, "subject": "Interview for …", "status_signal": "interview_invitation", "linked_slug": "tech-lead-web-core-speechify-a1b2" } }`,
    },
    {
      method: 'POST',
      path: '/me/emails/{id}/link',
      auth: 'cookie-or-key',
      summary: 'Attach a message to one of your applications.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The message id.' }],
      body: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug` to attach to.' }],
      curl: `curl -X POST "${BASE_URL}/me/emails/4821/link" \\
  -H "Authorization: Bearer fhk_…" \\
  -H 'Content-Type: application/json' -d '{"slug":"tech-lead-web-core-speechify-a1b2"}'`,
      responseExample: `{ "data": { "id": 4821, "linked_slug": "tech-lead-web-core-speechify-a1b2", "link_source": "manual" } }`,
    },
    {
      method: 'POST',
      path: '/me/emails/{id}/unlink',
      auth: 'cookie-or-key',
      summary: "Clear a message's application link; its classification stays.",
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The message id.' }],
      curl: `curl -X POST "${BASE_URL}/me/emails/4821/unlink" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": { "id": 4821, "linked_slug": "", "status_signal": "acknowledgement" } }`,
    },
    {
      method: 'POST',
      path: '/me/emails/{id}/confirm',
      auth: 'cookie-or-key',
      summary: 'Accept a suggested link.',
      description:
        'Only a deterministic match — the mail thread, or the company name in the ' +
        "sender or subject — links itself. A model's pick lands here as a " +
        'suggestion instead, and `?link=suggested` is the queue of them.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The message id.' }],
      curl: `curl -X POST "${BASE_URL}/me/emails/4821/confirm" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": { "id": 4821, "linked_slug": "tech-lead-web-core-speechify-a1b2", "link_source": "confirmed" } }`,
    },
    {
      method: 'POST',
      path: '/me/emails/{id}/reject',
      auth: 'cookie-or-key',
      summary: 'Dismiss a suggested link without attaching it.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The message id.' }],
      curl: `curl -X POST "${BASE_URL}/me/emails/4821/reject" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": { "id": 4821, "suggested_slug": "" } }`,
    },
    {
      method: 'POST',
      path: '/me/emails/{id}/application',
      auth: 'cookie-or-key',
      summary: 'Record an application from a message, and link the message to it.',
      description:
        'The way out of `?link=unlinked`: mail about a job you never tracked. The ' +
        "application is dated by the mail's `received_at`, not by now — the " +
        'application demonstrably existed by the time the employer wrote.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The message id.' }],
      body: [
        {
          name: 'slug',
          type: 'string',
          required: true,
          description: 'The job `public_slug` to record an application for.',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/me/emails/4821/application" \\
  -H "Authorization: Bearer fhk_…" \\
  -H 'Content-Type: application/json' -d '{"slug":"tech-lead-web-core-speechify-a1b2"}'`,
      responseExample: `{ "data": { "id": 4821, "linked_slug": "tech-lead-web-core-speechify-a1b2", "applied_at": "2026-07-24T09:12:00Z" } }`,
    },
    {
      method: 'POST',
      path: '/me/inbox/read-all',
      auth: 'cookie-or-key',
      summary: 'Mark every unread message matching the filters as read.',
      description:
        'It takes the same filters as `GET /me/inbox` and marks exactly what that ' +
        'listing shows — including the default that hides the `other` label, so an ' +
        'unfiltered call leaves that mail unread. There is no undo.',
      query: [
        { name: 'source', type: 'string', description: 'Narrow to one account.' },
        { name: 'status', type: 'string', description: 'Narrow to one label.' },
        { name: 'unclassified', type: 'boolean', description: 'Narrow to mail nothing has judged yet.' },
        { name: 'include_other', type: 'boolean', description: 'Also mark the `other` label the default hides.' },
        { name: 'link', type: 'string', description: 'Narrow to one link state.' },
        { name: 'q', type: 'string', description: 'Narrow by search.' },
      ],
      curl: `curl -X POST "${BASE_URL}/me/inbox/read-all" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": { "marked": 12 } }`,
    },
    {
      method: 'POST',
      path: '/me/emails/{id}/delete',
      auth: 'cookie-or-key',
      summary: 'Soft-delete a message. Answers 204.',
      description: 'A later re-sync will not resurrect it — deletion is reader state, not mail-server state.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The message id.' }],
      curl: `curl -X POST "${BASE_URL}/me/emails/4821/delete" -H "Authorization: Bearer fhk_…"`,
    },
    {
      method: 'POST',
      path: '/me/emails/{id}/restore',
      auth: 'cookie-or-key',
      summary: 'Undo a soft-delete. Answers 204.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The message id.' }],
      curl: `curl -X POST "${BASE_URL}/me/emails/4821/restore" -H "Authorization: Bearer fhk_…"`,
    },
    {
      method: 'GET',
      path: '/me/mailbox',
      auth: 'cookie-or-key',
      summary: 'Your hosted freehire address, and whether the feature is configured.',
      description:
        '`address` is null until you claim one; `available` is false when the instance runs without a mailbox domain.',
      curl: `curl "${BASE_URL}/me/mailbox" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": { "available": true, "address": "you@mail.freehire.me" } }`,
    },
    {
      method: 'POST',
      path: '/me/mailbox',
      auth: 'cookie-or-key',
      summary: 'Claim your hosted address.',
      curl: `curl -X POST "${BASE_URL}/me/mailbox" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": { "available": true, "address": "you@mail.freehire.me" } }`,
    },
    {
      method: 'DELETE',
      path: '/me/mailbox',
      auth: 'cookie-or-key',
      summary: 'Release the address; it stops receiving.',
      curl: `curl -X DELETE "${BASE_URL}/me/mailbox" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": { "available": true, "address": null } }`,
    },
    {
      method: 'GET',
      path: '/me/gmail',
      auth: 'cookie-or-key',
      summary: 'Gmail connection status.',
      description:
        '`available` reflects whether the instance has Gmail OAuth configured. ' +
        '`status` is `needs_reconsent` when Google revoked the grant. Connecting ' +
        'itself is a browser redirect (`/me/gmail/connect`), so it is session-only ' +
        'and not part of the key-driven surface.',
      curl: `curl "${BASE_URL}/me/gmail" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": { "available": true, "connected": true, "email": "you@gmail.com", "status": "ok" } }`,
    },
    {
      method: 'POST',
      path: '/me/gmail/sync',
      auth: 'cookie-or-key',
      summary: 'Start an incremental sync.',
      description:
        'Returns as soon as the sync is queued — a full backfill outlives the ' +
        'request — so poll the inbox for results rather than reading counts here.',
      curl: `curl -X POST "${BASE_URL}/me/gmail/sync" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": { "started": true } }`,
    },
    {
      method: 'DELETE',
      path: '/me/gmail',
      auth: 'cookie-or-key',
      summary: 'Disconnect Gmail and purge the mail it synced.',
      description:
        'Best-effort revokes the grant at Google, deletes the stored token, and ' +
        'removes the Gmail-sourced mail. Mail from the hosted address stays.',
      curl: `curl -X DELETE "${BASE_URL}/me/gmail" -H "Authorization: Bearer fhk_…"`,
      responseExample: `{ "data": { "connected": false } }`,
    },
  ],
};
