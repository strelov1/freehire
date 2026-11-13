import "@typespec/http";

using Http;

namespace Freehire;

/** The checkout link and which offer was applied to it. */
model BillingCheckout {
  url: string;

  /** The discount actually applied, 0 when none. */
  discount_percent: int64;

  /** The label of the applied discount, empty when none. */
  discount_source: string;
}

model BillingUrl {
  url: string;
}

model BillingInvoice {
  id: string;
  date: utcDateTime;

  /** The smallest currency unit, as the payment provider stores it. */
  amount_cents: int64;

  currency: string;
  status: string;

  /** The provider's hosted invoice, absent for a draft. */
  receipt_url?: string;
}

/** What the account is paying and what has been charged, read from the provider. */
model BillingOverview {
  status: string;
  amount_cents: int64;
  currency: string;
  interval: string;

  /** Absent when the subscription is ending. */
  renews_at?: utcDateTime;

  /** Set when the subscriber has cancelled. */
  ends_at?: utcDateTime;

  invoices: BillingInvoice[];
  multiple_subscriptions?: boolean;
}

model BillingStoreSync {
  /** `synced` or `no_subscription`. */
  status: string;
}

/** 404 when checkout is unconfigured or the price is not offered. Cookie session only. */
@tag("Billing")
@route("/billing/checkout")
@get
op getBillingCheckout(@query price?: string): Data<BillingCheckout> | Problem;

/** 404 when there is no subscription to manage. Cookie session only. */
@tag("Billing")
@route("/billing/manage")
@get
op getBillingManage(): Data<BillingUrl> | Problem;

/** 404 when the account has no subscription. Cookie session only. */
@tag("Billing")
@route("/billing/subscription")
@get
op getBillingSubscription(): Data<BillingOverview> | Problem;

// 503 when the store provider cannot be reached. Cookie session only.
/**
 * For a mobile client that has just completed an App Store or Google Play purchase. The
 * purchase finishes on the device before the provider’s webhook reaches us, and if that
 * delivery is lost the provider stops retrying after 80 minutes — so without this route a
 * paid subscriber can be left looking at a paywall. It names nobody: the account is the
 * session’s, and a user id in the request is ignored. Absent entirely where the store
 * provider is unconfigured. `status` is `synced` when the plan was written, or
 * `no_subscription` when RevenueCat holds nothing for this account; a 503 means the
 * provider could not be reached and the hourly reconciler will finish the job.
 */
@summary("Re-read your own store subscription now.")
@opExample(#{
  returnType: #{
    data: #{
      status: "synced",
    },
  },
})
@tag("Billing")
@route("/billing/revenuecat/sync")
@post
op syncBillingStore(): Data<BillingStoreSync> | Problem;
