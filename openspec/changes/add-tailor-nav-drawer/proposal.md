## Why

On the tailoring workspace the account icon rail (`AccountNavRail`) is pinned to
the left edge at every width. On a phone that fixed ~56px column steals space
from the already-cramped single-column view and is redundant with the new mobile
tab bar. The rail should get out of the way on mobile and be reachable on demand,
the way a mobile app hides its nav behind a burger.

## What Changes

- `AccountNavRail` gains an opt-in `collapsible` mode. Without it the rail renders
  exactly as before at every width (so `/my/assistant` is unaffected). With it:
  - at `lg` and up the icon rail shows as today;
  - below `lg` the rail is hidden (freeing the width) and instead opens as a
    labelled slide-in drawer over a dimmed backdrop, driven by a bindable `open`
    flag. The drawer closes on backdrop click, `Escape`, its close button, or
    following a link.
- The tailoring workspace opts in (`collapsible bind:open`) and renders the
  trigger — a burger button at the start of the mobile tab bar.

Scoped to the tailoring workspace; `/my/assistant` keeps the always-on rail.
No breaking changes — `collapsible`/`open` are additive and default to the prior
behaviour.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `tailor-workspace`: below `lg` the account icon rail collapses into a
  burger-triggered drawer instead of occupying a fixed left column.

## Impact

- **Frontend only:** `web/src/lib/components/AccountNavRail.svelte` (opt-in
  `collapsible` + bindable `open`, desktop rail + mobile drawer, shared link
  snippet) and `web/src/routes/tailor/[slug]/+page.svelte` (burger trigger in the
  mobile tab bar, `collapsible bind:open`).
- **Known limitation:** the burger lives in the mobile tab bar, which only
  renders in the workspace's ready state, so during the brief loading/error
  states there is no mobile nav trigger (the error state keeps its own back
  link). `/my/assistant` is intentionally out of scope.
