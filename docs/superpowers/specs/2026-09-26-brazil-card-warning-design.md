# Telling Brazilian visitors their card cannot pay, before they try

Date: 2026-09-26

## The problem, from the live data

Our Stripe account is registered in Brazil and prices are in USD. A Brazilian bank
cannot be charged in USD by a Brazilian business — the payment is domestic, nothing
crosses a border, so there is no conversion to make. Stripe refuses it with
`currency_not_supported`.

This is not a defect to repair. Accepting domestic Brazilian revenue would put the
business under a heavier tax regime, so the limitation does the owner's work for him.
What it does badly is *say so*.

On 26.09.2026 one visitor met it. Four charges, in eleven minutes:

```
05:28  BR  not_assessed  currency_not_supported
05:28  BR  not_assessed  currency_not_supported
05:29  US  highest       highest_risk_level      ← second card, blocked by Radar
05:36  BR  not_assessed  currency_not_supported
```

He was not told why. He tried again, then reached for a second, non-Brazilian card —
and by then the run of failures had marked him risky, so Radar blocked the one card
that could have worked. The silence cost a sale that was otherwise available.

## Why the message cannot go where the failure happens

Checkout is Stripe's **hosted** page (`POST /checkout/sessions`,
`internal/identity/billing/client.go`). The decline is rendered on Stripe's domain in
Stripe's words. We do not control that text and cannot add to it.

So the only place we can speak is *before* the redirect, on `/pricing` — which is also
the only place in the app that starts a checkout at all (`billingCheckout` has exactly
two call sites, both there).

## What we build

A line above the upgrade buttons, shown only to visitors the edge places in Brazil:

> Cards issued in Brazil can't be charged here — payments are taken in USD.
> Use a card from a bank outside Brazil.

Phrased as an instruction, not a refusal. "Payment unavailable" would end the
conversation; naming the card that does work keeps the sale reachable for anyone
holding one.

### The button stays enabled

Geography says where a person is, not what card they hold. A visitor in Brazil may
well be carrying a foreign card, and a Brazilian abroad — or behind a VPN — will not
see the warning at all. A guess this soft may inform a decision but must not take one
away: disabling the button would refuse money from people who can pay, to spare
people who cannot a message they have already been given.

## Where the country comes from

`/geo/region` already derives the visitor's location from Cloudflare's `CF-IPCountry`.
It grows one field: `{ region }` becomes `{ region, country }`.

**Not a second endpoint.** That route carries two pieces of reasoning that are easy to
get wrong and must not exist in two copies:

- `cache-control: private, no-store` — without it a CDN is glad to hold the response
  and replay one visitor's country to the next.
- the crawler check — an automated client must not be handed a location.

Duplicating those into a `/geo/country` would create a second place obliged to stay in
step with the first, and the one that drifts is the one nobody is reading.

The path keeps its name. `region` is now half the story, which the route's comment says
plainly; renaming it would touch four call sites in the jobs feed to buy nothing.

**The country must not travel in page data.** A server `load` returning it would
serialize it into the HTML, and that HTML is held by a shared cache keyed on the URL
alone — the first Brazilian to open `/pricing` would hand the warning to everyone
after them. This is the same trap the `/signin` crawl work hit with `s-maxage`.

## The rule itself

A new pure module, `web/src/lib/billingGeo.ts`, beside `geoScope.ts` and following it:
no DOM, no SvelteKit, callers pass in what they already hold.

The rule is not "Brazil is a problem country". It is: **a card issued in the country
our own Stripe account is registered in cannot be charged in our pricing currency.**
The account's country is the reason, and the constant is named for that, so a reader
who later moves the account knows what to change.

Reserved `CF-IPCountry` values (`XX`, `T1`) and a missing header mean "not placed" and
must read as *payable* — withholding nothing is the safe direction for a guess.

## Tests

- `billingGeo.test.ts` — `br` and `BR` refuse; `us`, `pt`, `null`, `undefined`, `''`,
  `XX`, `T1` permit.
- `geo/region` endpoint — returns the country, blanks it for a crawler, and still sets
  `private, no-store`.

## Out of scope

- Adding BRL as a second currency. Explicitly rejected: the owner does not want
  Brazilian revenue.
- Any change to Stripe Radar. Three of this visitor's four failures never reached
  Radar, so nothing there would have helped him.
