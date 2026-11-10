import { BASE_URL, type Group } from '../api-spec-types';

export const pushNotificationsAlertsGroup: Group = {
  title: 'Push notifications & alerts',
  intro:
    'The mobile app’s device push tokens, and the in-app notification center ' +
    '— a durable, channel-independent record of every delivered digest, ' +
    'reminder and nudge. All cookie-only.',
  endpoints: [
    {
      method: 'POST',
      path: '/me/push-tokens',
      auth: 'cookie',
      summary: 'Register (or reassign) a mobile device’s Expo push token.',
      description:
        'Upserted by token value, not by user: if a different account signs ' +
        'in on the same device, this reassigns the token to the new caller. ' +
        'Returns `204 No Content`.',
      body: [
        {
          name: 'token',
          type: 'string',
          required: true,
          description: 'The Expo push token minted by the device.',
          example: 'ExponentPushToken[xxxxxxxxxxxxxxxxxxxxxx]',
        },
        { name: 'platform', type: 'string', required: true, description: '`ios` or `android`.', example: 'ios' },
      ],
      curl: `curl -X POST "${BASE_URL}/me/push-tokens" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"token":"ExponentPushToken[xxxxxxxxxxxxxxxxxxxxxx]","platform":"ios"}'`,
    },
    {
      method: 'GET',
      path: '/me/push-tokens',
      auth: 'cookie',
      summary: 'List your own registered devices.',
      curl: `curl "${BASE_URL}/me/push-tokens" -b cookies.txt`,
      responseExample: `{
  "data": [ { "token": "ExponentPushToken[...]", "platform": "ios", "created_at": "2026-06-19T10:00:00Z", "last_seen_at": "2026-06-19T10:00:00Z" } ],
  "meta": { "total": 1 }
}`,
    },
    {
      method: 'DELETE',
      path: '/me/push-tokens',
      auth: 'cookie',
      summary: 'Unregister one of your own device tokens.',
      description: 'Returns `204 No Content`. `404` if the token is not yours (or unknown).',
      body: [{ name: 'token', type: 'string', required: true, description: 'The token to remove.' }],
      curl: `curl -X DELETE "${BASE_URL}/me/push-tokens" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"token":"ExponentPushToken[xxxxxxxxxxxxxxxxxxxxxx]"}'`,
    },
    {
      method: 'POST',
      path: '/me/push-tokens/test',
      auth: 'cookie',
      summary: 'Send a test push to all of your own registered devices.',
      description:
        'Never targets another user or a caller-supplied destination — only your own registered tokens. Each call fans out one outbound send per device.',
      curl: `curl -X POST "${BASE_URL}/me/push-tokens/test" -b cookies.txt`,
      responseExample: `{ "data": { "devices": 2, "sent": 1, "pruned": 1, "failed": 0 } }`,
    },
    {
      method: 'GET',
      path: '/me/notifications',
      auth: 'cookie',
      summary: 'List your notification-center entries, newest first.',
      query: [
        { name: 'limit', type: 'integer', description: 'Page size, 1–100.', example: '20' },
        { name: 'offset', type: 'integer', description: 'Rows to skip.', example: '0' },
      ],
      curl: `curl "${BASE_URL}/me/notifications?limit=20" -b cookies.txt`,
      responseExample: `{
  "data": [ { "id": 5, "kind": "subscription_digest", "title": "Senior Go remote", "body": "3 new jobs", "public_slug": null, "jobs": [ { "...": "..." } ], "created_at": "2026-06-19T10:00:00Z", "read_at": null } ],
  "meta": { "total": 12, "unread_count": 3, "limit": 20, "offset": 0 }
}`,
    },
    {
      method: 'GET',
      path: '/me/notifications/{id}',
      auth: 'cookie',
      summary: 'One of your notifications, including its jobs snapshot.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The notification id.', example: '5' }],
      curl: `curl "${BASE_URL}/me/notifications/5" -b cookies.txt`,
      responseExample: `{ "data": { "id": 5, "kind": "reminder", "title": "...", "body": "...", "public_slug": "senior-go-engineer-acme-1a2b", "created_at": "2026-06-19T10:00:00Z", "read_at": null } }`,
    },
    {
      method: 'POST',
      path: '/me/notifications/{id}/read',
      auth: 'cookie',
      summary: 'Mark one notification read (idempotent).',
      description: 'Returns `204 No Content`.',
      pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The notification id.', example: '5' }],
      curl: `curl -X POST "${BASE_URL}/me/notifications/5/read" -b cookies.txt`,
    },
    {
      method: 'POST',
      path: '/me/notifications/read-all',
      auth: 'cookie',
      summary: 'Mark every unread notification read.',
      curl: `curl -X POST "${BASE_URL}/me/notifications/read-all" -b cookies.txt`,
      responseExample: `{ "data": { "marked": 3 } }`,
    },
  ],
};
