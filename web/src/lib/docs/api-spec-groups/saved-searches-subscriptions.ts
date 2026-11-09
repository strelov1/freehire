import { BASE_URL, type Group } from '../api-spec-types';

export const savedSearchesSubscriptionsGroup: Group = {
  title: 'Saved searches & subscriptions',
  intro:
    'Browser conveniences, session-only. A saved search stores a canonical ' +
    'filter query string; a subscription turns one into a recurring digest ' +
    '(Telegram, email, or your own webhook). Each operation is owner-scoped — ' +
    'a non-owned id is a 404.',
  endpoints: [
    {
      method: 'GET',
      path: '/me/searches',
      auth: 'cookie',
      summary: 'List your saved searches.',
      curl: `curl "${BASE_URL}/me/searches" -b cookies.txt`,
      responseExample: `{ "data": [ { "id": 2, "name": "Senior Go remote", "query": "q=go&seniority=senior&work_mode=remote" } ] }`,
    },
    {
      method: 'POST',
      path: '/me/searches',
      auth: 'cookie',
      summary: 'Save a search.',
      body: [
        { name: 'name', type: 'string', required: true, description: 'Display name.', example: 'Senior Go remote' },
        {
          name: 'query',
          type: 'string',
          required: true,
          description: 'Canonical filter query string.',
          example: 'q=go&seniority=senior&work_mode=remote',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/me/searches" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"name":"Senior Go remote","query":"q=go&seniority=senior&work_mode=remote"}'`,
      responseExample: `{ "data": { "id": 2, "name": "Senior Go remote", "query": "q=go&seniority=senior&work_mode=remote" } }`,
    },
    {
      method: 'PATCH',
      path: '/me/searches/{id}',
      auth: 'cookie',
      summary: 'Rename or re-query a saved search.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The saved-search id.', example: '2' }],
      body: [
        { name: 'name', type: 'string', description: 'New name (optional).' },
        { name: 'query', type: 'string', description: 'New query (optional).' },
      ],
      curl: `curl -X PATCH "${BASE_URL}/me/searches/2" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"name":"Senior Go — EU remote"}'`,
      responseExample: `{ "data": { "id": 2, "name": "Senior Go — EU remote", "query": "..." } }`,
    },
    {
      method: 'DELETE',
      path: '/me/searches/{id}',
      auth: 'cookie',
      summary: 'Delete a saved search.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The saved-search id.', example: '2' }],
      curl: `curl -X DELETE "${BASE_URL}/me/searches/2" -b cookies.txt`,
      responseExample: `{ "data": { "ok": true } }`,
    },
    {
      method: 'GET',
      path: '/me/subscriptions',
      auth: 'cookie',
      summary: 'List your subscriptions.',
      curl: `curl "${BASE_URL}/me/subscriptions" -b cookies.txt`,
      responseExample: `{ "data": [ { "id": 1, "saved_search_id": 2, "channel": "telegram", "active": true } ] }`,
    },
    {
      method: 'POST',
      path: '/me/subscriptions',
      auth: 'cookie',
      summary: 'Subscribe a saved search to a digest channel.',
      description:
        'Channels: `telegram`, `email`, `push`, `webhook`. `push` needs a device the freehire mobile app has ' +
        'registered (`GET /me/push-tokens`) — without one the request is refused with 409.',
      body: [
        {
          name: 'saved_search_id',
          type: 'integer',
          required: true,
          description: 'The saved search to subscribe.',
          example: '2',
        },
        { name: 'channel', type: 'string', required: true, description: 'Delivery channel.', example: 'telegram' },
      ],
      curl: `curl -X POST "${BASE_URL}/me/subscriptions" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"saved_search_id":2,"channel":"telegram"}'`,
      responseExample: `{ "data": { "id": 1, "saved_search_id": 2, "channel": "telegram", "active": true } }`,
    },
    {
      method: 'PATCH',
      path: '/me/subscriptions/{id}',
      auth: 'cookie',
      summary: 'Pause or resume a subscription.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The subscription id.', example: '1' }],
      body: [
        {
          name: 'active',
          type: 'boolean',
          required: true,
          description: 'Whether the subscription is active.',
          example: 'false',
        },
      ],
      curl: `curl -X PATCH "${BASE_URL}/me/subscriptions/1" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"active":false}'`,
      responseExample: `{ "data": { "id": 1, "active": false } }`,
    },
    {
      method: 'DELETE',
      path: '/me/subscriptions/{id}',
      auth: 'cookie',
      summary: 'Delete a subscription.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The subscription id.', example: '1' }],
      curl: `curl -X DELETE "${BASE_URL}/me/subscriptions/1" -b cookies.txt`,
      responseExample: `{ "data": { "ok": true } }`,
    },
    {
      method: 'GET',
      path: '/me/telegram',
      auth: 'cookie',
      summary: 'Your Telegram link status (for digests).',
      curl: `curl "${BASE_URL}/me/telegram" -b cookies.txt`,
      responseExample: `{ "data": { "enabled": true, "linked": true, "chat_id": 123456789 } }`,
    },
    {
      method: 'POST',
      path: '/me/telegram/link',
      auth: 'cookie',
      summary: 'Start linking your Telegram account.',
      curl: `curl -X POST "${BASE_URL}/me/telegram/link" -b cookies.txt`,
      responseExample: `{ "data": { "url": "https://t.me/free_hire_bot?start=..." } }`,
    },
    {
      method: 'DELETE',
      path: '/me/telegram',
      auth: 'cookie',
      summary: 'Unlink your Telegram account.',
      curl: `curl -X DELETE "${BASE_URL}/me/telegram" -b cookies.txt`,
      responseExample: `(204 No Content)`,
    },
    // The Discord connect and callback routes are deliberately absent: both are browser
    // redirects that end on /my/integrations, not JSON a client can call. These two are
    // the JSON surface. Both 404 on a deployment with no Discord application configured.
    {
      method: 'GET',
      path: '/me/discord',
      auth: 'cookie',
      summary: 'Your Discord link status, and whether the paid role is on it.',
      curl: `curl "${BASE_URL}/me/discord" -b cookies.txt`,
      responseExample: `{ "data": { "enabled": true, "linked": true, "role_granted": true } }`,
    },
    {
      method: 'DELETE',
      path: '/me/discord',
      auth: 'cookie',
      summary: 'Unlink your Discord account, giving up the paid role with it.',
      curl: `curl -X DELETE "${BASE_URL}/me/discord" -b cookies.txt`,
      responseExample: `{ "data": { "linked": false } }`,
    },
    {
      method: 'GET',
      path: '/me/webhook',
      auth: 'cookie',
      summary: 'Your webhook destination for saved-search matches, or null if none is configured.',
      curl: `curl "${BASE_URL}/me/webhook" -b cookies.txt`,
      responseExample: `{ "data": { "url": "https://example.com/hook", "enabled": true, "created_at": "2026-09-04T12:00:00Z", "last_success_at": null, "disabled_at": null } }`,
    },
    {
      method: 'POST',
      path: '/me/webhook',
      auth: 'cookie',
      summary: 'Create your webhook destination, or update its URL if one already exists.',
      description:
        'There is exactly one destination per account. Deliveries are plain, unsigned HTTP POSTs — subscribe a ' +
        'saved search to the `webhook` channel (see `POST /me/subscriptions`) to receive its matches here. ' +
        'Before the URL is stored it receives a test delivery (`saved_search_name` "freehire webhook test", an ' +
        'empty `jobs` list); a URL that does not answer it with a 2xx is refused with 422, naming what it answered.',
      body: [
        {
          name: 'url',
          type: 'string',
          required: true,
          description: 'Destination URL — must be http or https.',
          example: 'https://example.com/hook',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/me/webhook" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"url":"https://example.com/hook"}'`,
      responseExample: `{ "data": { "url": "https://example.com/hook", "enabled": true, "created_at": "2026-09-04T12:00:00Z", "last_success_at": null, "disabled_at": null } }`,
    },
    {
      method: 'PATCH',
      path: '/me/webhook',
      auth: 'cookie',
      summary: 'Enable or disable your webhook destination without changing its URL.',
      body: [
        {
          name: 'enabled',
          type: 'boolean',
          required: true,
          description: 'Whether the destination is enabled.',
          example: 'false',
        },
      ],
      curl: `curl -X PATCH "${BASE_URL}/me/webhook" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"enabled":false}'`,
      responseExample: `{ "data": { "url": "https://example.com/hook", "enabled": false, "created_at": "2026-09-04T12:00:00Z", "last_success_at": null, "disabled_at": "2026-09-04T13:00:00Z" } }`,
    },
    {
      method: 'POST',
      path: '/me/webhook/test',
      auth: 'cookie',
      summary: 'Send a test delivery to your stored webhook destination.',
      description:
        'The same sample delivery saving a URL sends. A 2xx answer stamps `last_success_at` and returns the ' +
        'destination; anything else is a 422 naming what the destination answered, and changes nothing. ' +
        '404 when no destination is configured.',
      curl: `curl -X POST "${BASE_URL}/me/webhook/test" -b cookies.txt`,
      responseExample: `{ "data": { "url": "https://example.com/hook", "enabled": true, "created_at": "2026-09-04T12:00:00Z", "last_success_at": "2026-09-04T13:00:00Z", "disabled_at": null } }`,
    },
    {
      method: 'DELETE',
      path: '/me/webhook',
      auth: 'cookie',
      summary: 'Delete your webhook destination.',
      curl: `curl -X DELETE "${BASE_URL}/me/webhook" -b cookies.txt`,
      responseExample: `(204 No Content)`,
    },
  ],
};
