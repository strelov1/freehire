import "@typespec/http";

using Http;

namespace Freehire;

/** The kinds of change a webhook can ask for; the same as the change feed's. */
enum ChangeWebhookKind {
  opened,
  closed,
  reopened,
}

/** A webhook subscription to the change feed. The secret is never part of it after creation. */
model ChangeWebhook {
  id: int64;
  url: string;

  /** Empty: every company. Otherwise only changes of these companies' postings are delivered. */
  company_slugs: string[];

  kinds: ChangeWebhookKind[];
  created_at: utcDateTime;
  last_success_at: utcDateTime | null;

  /** Failed deliveries in a row; reset by a delivery that succeeds. */
  consecutive_failures: int64;

  /** What the endpoint answered to the last failed delivery; empty when the last one succeeded. */
  last_error: string;

  /** Set when the subscription was switched off after sustained failure or a `410`; null while it delivers. */
  disabled_at: utcDateTime | null;

  disabled_reason: string;
}

/** The subscription plus its signing secret, shown exactly once, in the answer that creates it. */
model ChangeWebhookCreated {
  ...ChangeWebhook;

  /** The HMAC key for `X-Freehire-Signature`. Store it now: it cannot be read again. */
  secret: string;
}

model ChangeWebhookCreate {
  /** Where to POST: an absolute http(s) URL. A private, loopback or link-local address is refused. */
  url: string;

  /** At most 100 company slugs, as `GET /companies/{slug}` knows them. Absent or empty: all companies. */
  company_slugs?: string[];

  /** Absent or empty: all three. */
  kinds?: ChangeWebhookKind[];
}

// Push delivery of the change feed. Needs an API plan (`api` or `api_data`).

/**
 * Subscribes a URL to the change feed and answers with the signing secret, this once. Needs
 * an `api` or `api_data` plan (`402` without one), at most 5 subscriptions per account (`409`
 * past that). The URL is probed with a signed `webhook.test` event first and refused with
 * `422` if it does not answer 2xx. Deliveries start from about the time of this call.
 */
@summary("Subscribe a URL to the change feed.")
@useAuth(SessionOrKey)
@opExample(#{
  parameters: #{
    body: #{ url: "https://mirror.example.com/hooks/freehire", company_slugs: #["acme"], kinds: #[ChangeWebhookKind.opened, ChangeWebhookKind.closed] },
  },
  returnType: #{
    _: 201,
    body: #{
      data: #{
        id: 7,
        url: "https://mirror.example.com/hooks/freehire",
        company_slugs: #["acme"],
        kinds: #[ChangeWebhookKind.opened, ChangeWebhookKind.closed],
        created_at: utcDateTime.fromISO("2026-06-20T04:10:00Z"),
        last_success_at: null,
        consecutive_failures: 0,
        last_error: "",
        disabled_at: null,
        disabled_reason: "",
        secret: "whsec_example_secret_shown_once",
      },
    },
  },
})
@tag("Change webhooks")
@route("/me/change-webhooks")
@post
op createChangeWebhook(@body body: ChangeWebhookCreate): {
  @statusCode _: 201;
  @body body: Data<ChangeWebhookCreated>;
} | Problem;

/** The caller's subscriptions, oldest first, with their delivery state. Secrets are not included. */
@summary("List change webhook subscriptions.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #[
      #{
        id: 7,
        url: "https://mirror.example.com/hooks/freehire",
        company_slugs: #["acme"],
        kinds: #[ChangeWebhookKind.opened, ChangeWebhookKind.closed],
        created_at: utcDateTime.fromISO("2026-06-20T04:10:00Z"),
        last_success_at: utcDateTime.fromISO("2026-06-21T09:00:00Z"),
        consecutive_failures: 0,
        last_error: "",
        disabled_at: null,
        disabled_reason: "",
      }
    ],
  },
})
@tag("Change webhooks")
@route("/me/change-webhooks")
@get
op listChangeWebhooks(): Data<ChangeWebhook[]> | Problem;

/** An id that is not the caller's is a `404`. Delete a disabled subscription and create it again to resume (from the changes made then). */
@summary("Delete a change webhook subscription.")
@useAuth(SessionOrKey)
@tag("Change webhooks")
@route("/me/change-webhooks/{id}")
@delete
op deleteChangeWebhook(@path id: string): NoContentResponse | Problem;

/** Sends a signed `webhook.test` event with no changes. `204` when the endpoint answered 2xx, `422` with what it answered otherwise. */
@summary("Send a test event to a subscription.")
@useAuth(SessionOrKey)
@tag("Change webhooks")
@route("/me/change-webhooks/{id}/test")
@post
op testChangeWebhook(@path id: string): NoContentResponse | Problem;
