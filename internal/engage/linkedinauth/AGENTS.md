# internal/engage/linkedinauth

The credential the daily digest posts to LinkedIn with: how it is minted, where it is
kept, and when it must be renewed or re-granted by a person. Driven by
[cmd/linkedin-auth](../../../cmd/linkedin-auth/main.go) (a person signs in) and
[cmd/linkedin-token-refresh](../../../cmd/linkedin-token-refresh/main.go) (a worker
renews, daily). Read by [socialdigest](../socialdigest/AGENTS.md)'s LinkedIn
publisher through a one-method interface it declares itself.

## The one rule that matters

**A refresh token may never arrive, and that is the expected case.** LinkedIn issues
programmatic refresh tokens to approved Marketing Developer Platform partners only,
and the Community Management API — the product that lets us post to our own page —
is not that program. Whether our application gets one is not knowable from the
documentation, only from the token response.

So both outcomes are first-class. With a refresh token the worker renews silently a
fortnight before expiry; without one it cannot renew anything, and its job is to warn
while there are still two weeks to act. Shipping only the happy path would have meant
discovering the answer as a digest that stopped publishing 60 days after launch, and
a channel that stopped is indistinguishable from a quiet day.

## Shape

- `Credentials` — the application's own identity, plus `AuthorizeURL` / `Exchange` /
  `Refresh`. `RedirectURI` must be absolute HTTPS and byte-identical in both legs;
  LinkedIn refuses `http`, **including on localhost**, which is why the sign-in hands
  a code to a command rather than catching it on a loopback listener.
- `Token` — absolute times, not the "seconds remaining" LinkedIn returns. Everything
  downstream compares against a clock, and a duration would be re-anchored at each of
  those comparisons. An empty `RefreshToken` means "cannot be renewed".
- `Renewer.Run` — the decision table: missing / healthy / renewed / warn / expired.
  Each branch returns rather than falling through, and the order is the order of
  urgency. It returns an `Outcome` rather than logging, because the caller decides
  both the exit code and whether a person is told.
- `PostgresStore` — the store, plus `AccessToken` for the publisher. The expiry is
  checked here rather than left to LinkedIn's 401: an expired credential is somebody's
  job and says so, while a 401 on a live token is LinkedIn revoking us.

## Conventions

- **A renewal does not extend the grant.** The refresh token keeps the 365 days it was
  granted at sign-in, so a deployment that renews forever still owes a person one
  sign-in a year — which is why `Run` warns on the grant's own expiry even when the
  access token is healthy.
- **The refresh token is carried forward when a renewal response omits it.** Storing
  the empty value would silently turn a renewable credential into one that warns
  tomorrow; a stale refresh token fails loudly at the next exchange instead.
- **A renewal that loses a race writes nothing.** `Renew` is guarded on the access
  token it was issued against, so a sign-in that landed mid-flight keeps its newer
  credential.
- **The stored token is plaintext**, unlike `gmail_tokens`. That table holds hundreds
  of USERS' credentials; this holds exactly one of ours, of the same sensitivity as the
  webhook URL sitting in cleartext in `.env`. See migration `0151`.
