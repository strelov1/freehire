import "@typespec/http";

using Http;

namespace Freehire;

// The signed-in account's own settings and the auth reads. What sets or clears a session
// cookie (register, login, logout, password change, email change confirm, account delete),
// the OAuth redirects and the MCP OAuth provider (RFC formats) are not here: they stay
// hand-written. Every answer is one person's own; the whole /me and /auth tree is no-store.

/** The account as the SPA and the CLI read it. */
model AuthUser {
  id: int64;
  email: string;
  role: string;
  beta_tester: boolean;

  /** An affordance only: the server never trusts the client's copy of it. */
  email_verified: boolean;

  /** False for an OAuth-only account. */
  has_password: boolean;

  created_at: utcDateTime | null;

  /** The account's IANA name, null until set. */
  timezone: string | null;

  language: string;

  /** Null for an account that never finished (or declined) the onboarding wizard. */
  onboarding_completed_at: utcDateTime | null;

  /** The resolved plan: `free`, `pro` or `ultra`. */
  tier: string;

  /** Whether the account holds a live API key or MCP grant. */
  agent_connected: boolean;
}

model AccountTimezoneUpdate {
  timezone?: string;

  /** Leave a timezone that is already set alone. */
  only_if_unset?: boolean;
}

model AccountLanguageUpdate {
  language?: string;
}

model AccountUsername {
  /** Null until claimed or allocated. */
  username: string | null;

  /** Null for a lazy default that was never explicitly claimed. */
  updated_at: utcDateTime | null;
}

model AccountUsernameClaim {
  username?: string;
}

model AccountUsernameAvailability {
  available: boolean;
}

model AccountTotalMeta {
  total: int64;
}

/** An unpaginated, owner-scoped list. */
model AccountTotalList<T> {
  data: T[];
  meta: AccountTotalMeta;
}

/** The public, secret-free shape of an API key. */
model ApiKey {
  id: int64;
  name: string;

  /** What identifies the key in a list. */
  token_prefix: string;

  /** What the key may reach: `full`, or `cv` for the tailoring agent. */
  scope: string;

  created_at: utcDateTime | null;
  last_used_at: utcDateTime | null;
  expires_at: utcDateTime | null;
}

/** The key plus its plaintext token, revealed exactly once, in the answer that creates it. */
model ApiKeyCreated {
  ...ApiKey;
  token: string;
}

model ApiKeyCreate {
  /** Required: an empty name is refused with a 400. */
  name?: string;

  /** Null or absent: the key never expires. */
  expires_at?: utcDateTime | null;

  /** Absent or empty mints a full-scope key; `embed` is the only other value a client may ask for. */
  scope?: string;
}

model ApiKeyPlan {
  /** The API product tier the account holds; empty for none. */
  tier: "" | "api" | "api_data";

  until?: utcDateTime;
}

/** A connected MCP client. */
model AccountOauthGrant {
  id: int64;
  client_name: string;
  created_at: utcDateTime | null;
  expires_at: utcDateTime | null;
  last_used_at: utcDateTime | null;
}

/** How a just-created account arrived. Every field is optional and untrusted. */
model AccountAttribution {
  utm_source?: string;
  utm_medium?: string;
  utm_campaign?: string;
  utm_content?: string;
  utm_term?: string;
  referrer_host?: string;
  landing_path?: string;
  method?: string;
  ga_client_id?: string;
  ga_session_id?: string;
}

model AccountAttributionResult {
  /** Whether the server sent the GA4 `sign_up`, so the browser stays out of it. */
  ga4_sent: boolean;
}

model PushTokenRegister {
  token?: string;

  /** `ios` or `android`. */
  platform?: string;
}

model PushTokenRemove {
  token?: string;
}

model PushToken {
  token: string;
  platform: string;
  created_at: utcDateTime | null;
  last_seen_at: utcDateTime | null;
}

/** What a self-test send did per registered device. */
model PushTokenTestResult {
  devices: int64;
  sent: int64;
  pruned: int64;
  failed: int64;
}

model Notification {
  id: int64;
  kind: string;
  title: string;
  body: string;
  public_slug: string | null;

  /** The stored snapshot of a multi-job digest; left out for every other kind. */
  jobs?: unknown;

  created_at: utcDateTime | null;
  read_at: utcDateTime | null;
}

model NotificationListMeta {
  total: int64;
  unread_count: int64;
  limit: int64;
  offset: int64;
}

model NotificationPage {
  data: Notification[];
  meta: NotificationListMeta;
}

model NotificationsMarked {
  marked: int64;
}

model AuthCode {
  code?: string;
}

model AuthEmailChange {
  email?: string;
}

model AuthPasswordForgot {
  email?: string;
}

model AuthPasswordReset {
  email?: string;
  code?: string;
  password?: string;
}

model AuthPasswordResetResult {
  reset: boolean;
}

@tag("Account")
@route("/me/timezone")
@patch
op updateTimezone(@body body: AccountTimezoneUpdate): Data<AuthUser> | Problem;

@tag("Account")
@route("/me/language")
@patch
op updateLanguage(@body body: AccountLanguageUpdate): Data<AuthUser> | Problem;

@tag("Account")
@route("/me/username")
@get
op getUsername(): Data<AccountUsername> | Problem;

/** Claims or changes the username. */
@tag("Account")
@route("/me/username")
@put
op claimUsername(@body body: AccountUsernameClaim): Data<AccountUsername> | Problem;

/** Public: a name picker probes before the caller is signed in. */
@tag("Account")
@route("/username/check")
@get
op checkUsername(@query value?: string): Data<AccountUsernameAvailability> | Problem;

/** Mails a confirmation code to the new address. Behind the recent-auth gate. */
@tag("Account")
@route("/me/email")
@post
op requestEmailChange(@body body: AuthEmailChange): {
  @statusCode _: 202;
} | Problem;

/** Creates a key and returns its plaintext token, this once. Behind the recent-auth gate. */
@tag("Account")
@route("/me/api-keys")
@post
op createApiKey(@body body: ApiKeyCreate): {
  @statusCode _: 201;
  @body body: Data<ApiKeyCreated>;
} | Problem;

@tag("Account")
@route("/me/api-keys")
@get
op listApiKeys(): AccountTotalList<ApiKey> | Problem;

/** An id that is not the caller's is a 404. Behind the recent-auth gate. */
@tag("Account")
@route("/me/api-keys/{id}")
@delete
op revokeApiKey(@path id: string): NoContentResponse | Problem;

@tag("Account")
@route("/me/api-plan")
@get
op getApiPlan(): Data<ApiKeyPlan> | Problem;

@tag("Account")
@route("/me/oauth-grants")
@get
op listOauthGrants(): AccountTotalList<AccountOauthGrant> | Problem;

/** An id that is not the caller's is a 404. Behind the recent-auth gate. */
@tag("Account")
@route("/me/oauth-grants/{id}")
@delete
op revokeOauthGrant(@path id: string): NoContentResponse | Problem;

/** A failure to store or send never surfaces as an error. */
@tag("Account")
@route("/me/attribution")
@post
op reportSignupAttribution(@body body: AccountAttribution): Data<AccountAttributionResult> | Problem;

/** Registers or refreshes the device's push token; a token is reassigned to the caller. */
@tag("Account")
@route("/me/push-tokens")
@post
op registerPushToken(@body body: PushTokenRegister): NoContentResponse | Problem;

@tag("Account")
@route("/me/push-tokens")
@get
op listPushTokens(): AccountTotalList<PushToken> | Problem;

/** A token that is not the caller's is a 404. */
@tag("Account")
@route("/me/push-tokens")
@delete
op unregisterPushToken(@body body: PushTokenRemove): NoContentResponse | Problem;

/** Sends a test push to the caller's own registered tokens, never anyone else's. */
@tag("Account")
@route("/me/push-tokens/test")
@post
op testPushToken(): Data<PushTokenTestResult> | Problem;

@tag("Account")
@route("/me/notifications")
@get
op listNotifications(@query limit?: string, @query offset?: string): NotificationPage | Problem;

/** Another user's id, or one that does not exist, is a 404. */
@tag("Account")
@route("/me/notifications/{id}")
@get
op getNotification(@path id: string): Data<Notification> | Problem;

/** Idempotent; another user's id, or one that does not exist, is a 404. */
@tag("Account")
@route("/me/notifications/{id}/read")
@post
op markNotificationRead(@path id: string): NoContentResponse | Problem;

@tag("Account")
@route("/me/notifications/read-all")
@post
op markAllNotificationsRead(): Data<NotificationsMarked> | Problem;

/** Readable with a session or an API key, narrow scope included. */
@tag("Auth")
@route("/auth/me")
@get
op getAuthMe(): Data<AuthUser> | Problem;

/** Mails the caller a fresh verification code. */
@tag("Auth")
@route("/auth/verify/request")
@post
op requestEmailVerification(): {
  @statusCode _: 202;
} | Problem;

@tag("Auth")
@route("/auth/verify/confirm")
@post
op confirmEmailVerification(@body body: AuthCode): Data<AuthUser> | Problem;

/** Always answers 202, whether or not the address has an account. */
@tag("Auth")
@route("/auth/password/forgot")
@post
op forgotPassword(@body body: AuthPasswordForgot): {
  @statusCode _: 202;
} | Problem;

/** Public: the mailed code is the credential. Revokes every session. */
@tag("Auth")
@route("/auth/password/reset")
@post
op resetPassword(@body body: AuthPasswordReset): Data<AuthPasswordResetResult> | Problem;

/** The names of the enabled sign-in providers. */
@tag("Auth")
@route("/auth/oauth/providers")
@get
op listOauthProviders(): Data<string[]> | Problem;
