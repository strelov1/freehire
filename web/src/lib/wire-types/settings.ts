import type { components } from '#lib/generated/api.js';
/** An API key as returned by the management endpoints — metadata only; the
 *  plaintext token is never part of this shape. `token_prefix` is a short,
 *  non-secret leading slice (e.g. "fhk_Ab12cd") shown so the user can tell keys
 *  apart. Timestamps are RFC3339 strings or null. */
export type ApiKey = components['schemas']['ApiKey'];

/** One MCP client (Claude, Cursor, ChatGPT, …) a user approved via OAuth — a
 *  "connected device". Timestamps are RFC3339 strings or null. */
export type OAuthGrant = components['schemas']['AccountOauthGrant'];

/** The response of creating a key: the metadata plus the plaintext `token`,
 *  returned exactly once and never retrievable again. */
export type CreatedApiKey = components['schemas']['ApiKeyCreated'];

/** The account's single saved-search webhook destination. Deliveries are
 *  plain, unsigned POSTs. Timestamps are RFC3339 strings or null;
 *  `disabled_at` is set once a delivery got a definitive 410 from the
 *  destination or the user turned it off. */
export interface WebhookConfig {
  url: string;
  enabled: boolean;
  created_at: string | null;
  last_success_at: string | null;
  disabled_at: string | null;
}

interface ConnectedIdentity {
  provider: string;
  linked_at: string;
  status: 'active' | 'revocation_pending';
  can_unlink: boolean;
}

export interface ConnectedIdentities {
  has_password: boolean;
  identities: ConnectedIdentity[];
}

/** A user's saved search: a named snapshot of the job-search filter state. `query`
 *  is the canonical search query string (the same the filter URL serializes), so
 *  applying it reproduces the exact filters. An empty `query` is the "show all" view.
 *  Timestamps are RFC3339 strings or null. */
export type SavedSearch = components['schemas']['SavedSearch'];
