import "@typespec/http";
import "@typespec/openapi";

using Http;
using TypeSpec.OpenAPI;

namespace Freehire;

// The signed-in account's own settings and the auth reads. What sets or clears a session
// cookie (register, login, logout, password change, email change confirm, account delete),
// the OAuth redirects and the MCP OAuth provider (RFC formats) are not here: they stay
// hand-written. Every answer is one person's own; the whole /me and /auth tree is no-store.

enum AuthUserTier {
  free,
  pro,
  ultra,
}

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

  /** The resolved plan. */
  tier: AuthUserTier;

  /** Whether the account holds a live API key or MCP grant. */
  agent_connected: boolean;
}

model AccountTimezoneUpdate {
  /** IANA timezone name. (e.g. `Europe/Berlin`) */
  timezone?: string;

  /** Leave a timezone that is already set alone. */
  only_if_unset?: boolean;
}

model AccountLanguageUpdate {
  /** Supported language code. (e.g. `en`) */
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
  /**
   * The Expo push token minted by the device. (e.g.
   * `ExponentPushToken[xxxxxxxxxxxxxxxxxxxxxx]`)
   */
  token?: string;

  /** `ios` or `android`. */
  platform?: string;
}

model PushTokenRemove {
  /** The token to remove. */
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

/**
 * What a `user_notifications` row can be about. The column is free text; these are the kinds
 * the engines write (notify, reminder, nudge, buddy, messaging, auto-apply), plus
 * `auto_apply_tailor_ready`, which only historical rows carry.
 */
enum NotificationKind {
  subscription_digest,
  reminder,
  nudge_follow_up,
  nudge_interview_prep,
  nudge_job_closed,
  nudge_auto_apply_submitted,
  nudge_auto_apply_blocked,
  nudge_auto_apply_failed,
  auto_apply_tailor_ready,
  auto_apply_ready_for_review,
  auto_apply_tailoring_failed,
  buddy_mock_booked,
  buddy_mock_cancelled,
  buddy_mock_reminder,
  buddy_reaction,
  buddy_message,
  buddy_invitation,
}

/** One job of a multi-job digest, as recorded at delivery (notify.SnapshotJob). */
model NotificationDigestJob {
  title: string;
  company: string;
  slug: string;
}

model Notification {
  id: int64;
  kind: NotificationKind;
  title: string;
  body: string;
  public_slug: string | null;

  /** The stored snapshot of a multi-job digest; left out for every other kind. */
  jobs?: NotificationDigestJob[];

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
  /** The six-digit code mailed to your address. (e.g. `123456`) */
  code?: string;
}

model AuthEmailChange {
  email?: string;
}

model AuthPasswordForgot {
  /** Account email. (e.g. `me@example.com`) */
  email?: string;
}

model AuthPasswordReset {
  /** Account email. (e.g. `me@example.com`) */
  email?: string;
  /** The mailed reset code. (e.g. `123456`) */
  code?: string;
  /** New password. */
  password?: string;
}

model AuthPasswordResetResult {
  reset: boolean;
}

@summary("Set your account’s IANA timezone.")
@tag("Account")
@route("/me/timezone")
@patch
op updateTimezone(@body body: AccountTimezoneUpdate): Data<AuthUser> | Problem;

@summary("Set your preferred interface language.")
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
@useAuth(NoAuth)
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
@extension("x-audience", "hidden")
@tag("Account")
@route("/me/api-keys")
@post
op createApiKey(@body body: ApiKeyCreate): {
  @statusCode _: 201;
  @body body: Data<ApiKeyCreated>;
} | Problem;

@extension("x-audience", "hidden")
@tag("Account")
@route("/me/api-keys")
@get
op listApiKeys(): AccountTotalList<ApiKey> | Problem;

/** An id that is not the caller's is a 404. Behind the recent-auth gate. */
@extension("x-audience", "hidden")
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

// Registers or refreshes the device's push token; a token is reassigned to the caller.
/**
 * Upserted by token value, not by user: if a different account signs in on the same
 * device, this reassigns the token to the new caller. Returns `204 No Content`.
 */
@summary("Register (or reassign) a mobile device’s Expo push token.")
@opExample(#{
  parameters: #{
    body: #{
      token: "ExponentPushToken[xxxxxxxxxxxxxxxxxxxxxx]",
      platform: "ios",
    },
  },
})
@tag("Account")
@route("/me/push-tokens")
@post
op registerPushToken(@body body: PushTokenRegister): NoContentResponse | Problem;

@summary("List your own registered devices.")
@opExample(#{
  returnType: #{
    data: #[
      #{
        token: "ExponentPushToken[...]",
        platform: "ios",
        created_at: utcDateTime.fromISO("2026-06-19T10:00:00Z"),
        last_seen_at: utcDateTime.fromISO("2026-06-19T10:00:00Z"),
      },
    ],
    meta: #{
      total: 1,
    },
  },
})
@tag("Account")
@route("/me/push-tokens")
@get
op listPushTokens(): AccountTotalList<PushToken> | Problem;

// A token that is not the caller's is a 404.
/** Returns `204 No Content`. `404` if the token is not yours (or unknown). */
@summary("Unregister one of your own device tokens.")
@opExample(#{
  parameters: #{
    body: #{
      token: "ExponentPushToken[xxxxxxxxxxxxxxxxxxxxxx]",
    },
  },
})
@tag("Account")
@route("/me/push-tokens")
@delete
op unregisterPushToken(@body body: PushTokenRemove): NoContentResponse | Problem;

// Sends a test push to the caller's own registered tokens, never anyone else's.
/**
 * Never targets another user or a caller-supplied destination — only your own registered
 * tokens. Each call fans out one outbound send per device.
 */
@summary("Send a test push to all of your own registered devices.")
@opExample(#{
  returnType: #{
    data: #{
      devices: 2,
      sent: 1,
      pruned: 1,
      failed: 0,
    },
  },
})
@tag("Account")
@route("/me/push-tokens/test")
@post
op testPushToken(): Data<PushTokenTestResult> | Problem;

@summary("List your notification-center entries, newest first.")
@tag("Account")
@route("/me/notifications")
@get
op listNotifications(/** Page size, 1–100. (e.g. `20`) */ @query limit?: string, /** Rows to skip. (e.g. `0`) */ @query offset?: string): NotificationPage | Problem;

/** Another user's id, or one that does not exist, is a 404. */
@summary("One of your notifications, including its jobs snapshot.")
@tag("Account")
@route("/me/notifications/{id}")
@get
op getNotification(/** The notification id. (e.g. `5`) */ @path id: string): Data<Notification> | Problem;

// Idempotent; another user's id, or one that does not exist, is a 404.
/** Returns `204 No Content`. */
@summary("Mark one notification read (idempotent).")
@tag("Account")
@route("/me/notifications/{id}/read")
@post
op markNotificationRead(/** The notification id. (e.g. `5`) */ @path id: string): NoContentResponse | Problem;

@summary("Mark every unread notification read.")
@opExample(#{
  returnType: #{
    data: #{
      marked: 3,
    },
  },
})
@tag("Account")
@route("/me/notifications/read-all")
@post
op markAllNotificationsRead(): Data<NotificationsMarked> | Problem;

/** Readable with a session or an API key, narrow scope included. */
@summary("The current user (cookie or API key).")
@useAuth(SessionOrKey)
@tag("Auth")
@route("/auth/me")
@get
op getAuthMe(): Data<AuthUser> | Problem;

// Mails the caller a fresh verification code.
/**
 * Mails a fresh code to the caller’s own address, never a body field. Returns
 * `202 Accepted`. `409` if the address is already confirmed.
 */
@summary("Send (or resend) a six-digit email verification code.")
@tag("Auth")
@route("/auth/verify/request")
@post
op requestEmailVerification(): {
  @statusCode _: 202;
} | Problem;

@summary("Confirm the address with the mailed code.")
@tag("Auth")
@route("/auth/verify/confirm")
@post
op confirmEmailVerification(@body body: AuthCode): Data<AuthUser> | Problem;

// Always answers 202, whether or not the address has an account.
/**
 * Always answers `202`, whether or not the address has an account — this is deliberately
 * not an email-enumeration oracle.
 */
@summary("Request a password-reset code by email.")
@useAuth(NoAuth)
@opExample(#{
  parameters: #{
    body: #{
      email: "me@example.com",
    },
  },
})
@tag("Auth")
@route("/auth/password/forgot")
@post
op forgotPassword(@body body: AuthPasswordForgot): {
  @statusCode _: 202;
} | Problem;

// Public: the mailed code is the credential. Revokes every session.
/**
 * Public — the code is the credential. Success revokes every existing session, so sign in
 * fresh with the new password afterward.
 */
@summary("Set a new password against a mailed code.")
@useAuth(NoAuth)
@opExample(#{
  parameters: #{
    body: #{
      email: "me@example.com",
      code: "123456",
      password: "hunter2hunter2",
    },
  },
  returnType: #{
    data: #{
      reset: true,
    },
  },
})
@tag("Auth")
@route("/auth/password/reset")
@post
op resetPassword(@body body: AuthPasswordReset): Data<AuthPasswordResetResult> | Problem;

/** The names of the enabled sign-in providers. */
@summary("List the enabled OAuth providers.")
@useAuth(NoAuth)
@opExample(#{
  returnType: #{
    data: #["google", "github"],
  },
})
@tag("Auth")
@route("/auth/oauth/providers")
@get
op listOauthProviders(): Data<string[]> | Problem;
