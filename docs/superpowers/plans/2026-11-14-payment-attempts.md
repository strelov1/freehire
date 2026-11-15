# Payment attempt history

The user approved a history of individual payment attempts on the plan page, with date,
amount, masked card, outcome and actionable explanations. Read Stripe charges rather than
PaymentIntents: retrying a PaymentIntent creates another charge. Keep the existing receipt
and subscription views intact. No new local payment state or migration is needed.

1. Add a customer-scoped Stripe charge reader with a bounded recent history. Return only
   masked card data, normalized outcomes and allowlisted reason keys; never expose risk
   scores or provider messages. Test repeated attempts, refunds, pending payments, unknown
   reasons and provider failures.
2. Bind new checkout customers before creating their session, using the existing write-once
   customer helper. Reuse the winner of a concurrent bind. Test first-purchase failures and
   subsequent customer reuse.
3. Expose a cookie-authenticated `/billing/attempts` read endpoint. Resolve the customer
   from the session account, return an empty list for an account with no customer, and an
   explicit temporary failure when Stripe cannot be read. Test account isolation.
4. Add a separate payment history section to the plan page for free and paid accounts.
   Show loading, retryable errors, empty history, localized reason/help text, and a refresh
   button. Do not offer repayment for historical failures after a later success; use the
   existing pricing and management links. Clear data on account changes and drop late
   responses. Verify UI tests, type checks, rendering and Go checks.

Historical attempts under customers that were never bound before this feature cannot be
recovered by querying the current customer. Do not discover ownership through matching email
or untrusted Payment Link references. Future checkouts keep a stable customer from the start.
