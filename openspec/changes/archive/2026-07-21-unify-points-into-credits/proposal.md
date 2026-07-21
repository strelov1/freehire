## Why

The app has two parallel "points" quantities that confuse users and the codebase: the legacy `users.points` counter (incremented +1 per accepted board contribution and shown on `/my/contributions` as "points"), and the real AI-credits ledger that gates the metered AI features. A board contribution already awards +5 AI credits via `credits.Reward()`, so the `users.points` counter is dead weight that dilutes the single unit users should reason about. Collapsing everything into AI credits — and giving credits a home page with a transaction history — makes the economy legible.

## What Changes

- **BREAKING**: Remove the legacy `users.points` system entirely — drop the `users.points` column (migration), delete the `IncrementUserPoints` query and its call in `contribution/repository.go`, and stop returning `points` from `/auth/me` (users query, `db` model, and the frontend `User` type). The +5 AI-credits contribution reward via `credits.Reward()` is unchanged and becomes the sole reward.
- Rewrite all "point"/"points" user-facing copy on `/my/contributions` (`ContributeView.svelte`) and the contribute landing (`ContributeLandingView.svelte`) to speak in AI credits ("+5 AI credits per new board").
- Remove the inline `CreditsBalance` widget from the Activity → Matches tab and the Profile page; the balance now lives on its own page.
- Add a new page at `/my/credits` titled "Credits" showing the current balance (remaining this month + reset date) and a transaction history read from `credit_ledger`: monthly grants (+20), match debits (−1), tailor debits (−3), and contribution rewards (+5). Each debit resolves its `ref` to a human label (match → job title/slug, tailor → CV label).
- Add a new read endpoint `GET /api/v1/me/credits/history` (newest first) backed by a new sqlc query over `credit_ledger`, plus the frontend API method, route, and component.
- Add a "Credits" item to the account section navigation.

## Capabilities

### New Capabilities
- `credits-page`: The user-facing Credits page and its history endpoint — current balance display and a paged, human-labelled transaction history over the credit ledger.

### Modified Capabilities
- `link-contributions`: The contribution reward and its surfacing are expressed in AI credits, not a separate points counter; the `users.points` balance requirement is removed and the contributions view copy speaks in credits.
- `account-navigation`: The account section gains a "Credits" navigation item.

## Impact

- **Migrations**: new migration dropping `users.points` (manual `SET ROLE hire` run on prod before deploy, per project convention).
- **Backend**: `internal/db/queries/{link_contributions,users,credits}.sql` + regenerated sqlc; `internal/contribution/repository.go`; `internal/db/models.go` (`points` field); `internal/handler/{me_credits.go,handler.go}` (new history handler + route); auth/me response.
- **Frontend**: `web/src/lib/types.ts` (`User.points` removed, new history type); `web/src/lib/api.ts` (new method); `ContributeView.svelte`, `ContributeLandingView.svelte` (copy); Activity/Matches + Profile pages (widget removal); new `/my/credits` route + component; account nav.
- **No change** to the credits economics (`internal/config/credits.go`), the `credit_ledger`/`credit_balances` schema, or the `Reward()`/`Debit()` logic.
