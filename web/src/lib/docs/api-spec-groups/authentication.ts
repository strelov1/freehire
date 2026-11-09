import { BASE_URL, type Group } from '../api-spec-types';

export const authenticationGroup: Group = {
  title: 'Authentication',
  intro:
    'Register/login set the session cookie and return the user. Logout clears ' +
    'it. `me` resolves the caller (cookie or API key). OAuth sign-in is a ' +
    'redirect flow.',
  endpoints: [
    {
      method: 'POST',
      path: '/auth/register',
      auth: 'none',
      summary: 'Create an account and start a session.',
      body: [
        {
          name: 'email',
          type: 'string',
          required: true,
          description: 'Account email (canonical key).',
          example: 'me@example.com',
        },
        { name: 'password', type: 'string', required: true, description: 'Account password.' },
      ],
      curl: `curl -X POST "${BASE_URL}/auth/register" \\
  -H 'Content-Type: application/json' \\
  -c cookies.txt \\
  -d '{"email":"me@example.com","password":"hunter2hunter2"}'`,
      responseExample: `{ "data": { "id": 1, "email": "me@example.com", "role": "user", "created_at": "2026-06-19T10:00:00Z" } }`,
    },
    {
      method: 'POST',
      path: '/auth/login',
      auth: 'none',
      summary: 'Sign in and start a session.',
      body: [
        { name: 'email', type: 'string', required: true, description: 'Account email.', example: 'me@example.com' },
        { name: 'password', type: 'string', required: true, description: 'Account password.' },
      ],
      curl: `curl -X POST "${BASE_URL}/auth/login" \\
  -H 'Content-Type: application/json' \\
  -c cookies.txt \\
  -d '{"email":"me@example.com","password":"hunter2hunter2"}'`,
      responseExample: `{ "data": { "id": 1, "email": "me@example.com", "role": "user" } }`,
    },
    {
      method: 'POST',
      path: '/auth/logout',
      auth: 'none',
      summary: 'Clear the session cookie.',
      curl: `curl -X POST "${BASE_URL}/auth/logout" -b cookies.txt`,
      responseExample: `{ "data": { "ok": true } }`,
    },
    {
      method: 'GET',
      path: '/auth/me',
      auth: 'cookie-or-key',
      summary: 'The current user (cookie or API key).',
      curl: `curl "${BASE_URL}/auth/me" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
      responseExample: `{ "data": { "id": 1, "email": "me@example.com", "role": "user" } }`,
    },
    {
      method: 'GET',
      path: '/auth/oauth/providers',
      auth: 'none',
      summary: 'List the enabled OAuth providers.',
      curl: `curl "${BASE_URL}/auth/oauth/providers"`,
      responseExample: `{ "data": ["google", "github"] }`,
    },
    {
      method: 'GET',
      path: '/auth/oauth/{provider}/start',
      auth: 'none',
      summary: 'Begin the OAuth sign-in redirect.',
      description:
        'Browser-only: redirects to the provider, then back to ' +
        '`/auth/oauth/{provider}/callback`, which sets the session cookie and ' +
        'redirects to the app. Not a JSON endpoint.',
      pathParams: [
        {
          name: 'provider',
          type: 'string',
          required: true,
          description: 'One of the enabled providers.',
          example: 'google',
        },
      ],
      curl: `# open in a browser:
${BASE_URL}/auth/oauth/google/start`,
    },
    {
      method: 'POST',
      path: '/auth/verify/request',
      auth: 'cookie',
      summary: 'Send (or resend) a six-digit email verification code.',
      description:
        'Mails a fresh code to the caller’s own address, never a body field. Returns `202 Accepted`. `409` if the address is already confirmed.',
      curl: `curl -X POST "${BASE_URL}/auth/verify/request" -b cookies.txt`,
    },
    {
      method: 'POST',
      path: '/auth/verify/confirm',
      auth: 'cookie',
      summary: 'Confirm the address with the mailed code.',
      body: [
        {
          name: 'code',
          type: 'string',
          required: true,
          description: 'The six-digit code mailed to your address.',
          example: '123456',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/auth/verify/confirm" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"code":"123456"}'`,
      responseExample: `{ "data": { "id": 1, "email": "me@example.com", "role": "user", "...": "..." } }`,
    },
    {
      method: 'POST',
      path: '/auth/password/forgot',
      auth: 'none',
      summary: 'Request a password-reset code by email.',
      description:
        'Always answers `202`, whether or not the address has an account — this is deliberately not an email-enumeration oracle.',
      body: [
        { name: 'email', type: 'string', required: true, description: 'Account email.', example: 'me@example.com' },
      ],
      curl: `curl -X POST "${BASE_URL}/auth/password/forgot" \\
  -H 'Content-Type: application/json' \\
  -d '{"email":"me@example.com"}'`,
    },
    {
      method: 'POST',
      path: '/auth/password/reset',
      auth: 'none',
      summary: 'Set a new password against a mailed code.',
      description:
        'Public — the code is the credential. Success revokes every existing session, so sign in fresh with the new password afterward.',
      body: [
        { name: 'email', type: 'string', required: true, description: 'Account email.', example: 'me@example.com' },
        { name: 'code', type: 'string', required: true, description: 'The mailed reset code.', example: '123456' },
        { name: 'password', type: 'string', required: true, description: 'New password.' },
      ],
      curl: `curl -X POST "${BASE_URL}/auth/password/reset" \\
  -H 'Content-Type: application/json' \\
  -d '{"email":"me@example.com","code":"123456","password":"hunter2hunter2"}'`,
      responseExample: `{ "data": { "reset": true } }`,
    },
    {
      method: 'POST',
      path: '/auth/logout-all',
      auth: 'cookie',
      summary: 'Sign out every session on the account, including this one.',
      description:
        'Bumps the account’s session generation (stranding every issued token and API-key session state) then clears the caller’s cookie. Cookie-only — an API key must not be able to sign a human out of their browser. Returns `204 No Content`.',
      curl: `curl -X POST "${BASE_URL}/auth/logout-all" -b cookies.txt`,
    },
    {
      method: 'POST',
      path: '/auth/oauth/exchange',
      auth: 'none',
      summary: 'Redeem a mobile OAuth callback code for a session.',
      description:
        'Mobile-only: the app’s custom-scheme OAuth callback carries a ' +
        'single-use code (not the session directly); this exchanges it for ' +
        'the session cookie, landing in the app’s own cookie jar. Public — the ' +
        'code is the credential, and a missing/expired/reused one is a `401`.',
      body: [
        {
          name: 'code',
          type: 'string',
          required: true,
          description: 'The one-time code from the mobile callback redirect.',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/auth/oauth/exchange" \\
  -H 'Content-Type: application/json' -c cookies.txt \\
  -d '{"code":"..."}'`,
      responseExample: `{ "data": { "id": 1, "email": "me@example.com", "role": "user" } }`,
    },
    {
      method: 'GET',
      path: '/auth/extension/connect',
      auth: 'extension',
      summary: 'Consent screen for "Sign in with freehire" from the browser extension.',
      description:
        'Opened by the extension via `chrome.identity.launchWebAuthFlow`, not ' +
        'called directly — renders an HTML consent page, not JSON. ' +
        '`redirect_uri` must be an allowlisted `https://<extension-id>.chromiumapp.org` ' +
        'origin, or this is a `400`. A signed-out visitor is bounced through ' +
        'sign-in first, then back to this same consent step.',
      query: [
        {
          name: 'redirect_uri',
          type: 'string',
          required: true,
          description: 'The allowlisted chromiumapp.org redirect the extension is listening on.',
        },
        { name: 'state', type: 'string', description: 'Opaque value echoed back on completion.' },
      ],
      curl: `# opened by the extension, not called directly:
${BASE_URL}/auth/extension/connect?redirect_uri=https://<extension-id>.chromiumapp.org/&state=...`,
    },
    {
      method: 'POST',
      path: '/auth/extension/connect',
      auth: 'extension',
      summary: 'Submit the consent decision and mint the extension’s session token.',
      description:
        'Submitted by the consent screen’s own form, not called directly. On ' +
        '`decision=allow`, 302-redirects to `redirect_uri` with a session ' +
        'token in the URL fragment (never the query, so it is never logged or ' +
        'sent to a server); on anything else, redirects with `error=access_denied`. ' +
        'Not a JSON endpoint.',
      body: [
        {
          name: 'redirect_uri',
          type: 'string',
          required: true,
          description: 'Must match the allowlisted redirect validated on the GET step.',
        },
        { name: 'state', type: 'string', description: 'Opaque value echoed back.' },
        { name: 'decision', type: 'string', required: true, description: '`allow` or `cancel`.', example: 'allow' },
      ],
      curl: `# submitted by the consent page's own form, not called directly`,
    },
  ],
};
