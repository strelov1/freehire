## 1. API
- [x] 1.1 Migration 0219 `buddy_reactions`; upsert, delete and per-event counts queries
- [x] 1.2 `Service.React` admits only events the member's feed shows; feed entries carry reactions (integration tests)
- [x] 1.3 `PUT/DELETE /me/buddies/feed/:id/reaction` (handler integration test)

## 2. Web
- [x] 2.1 `buddyReactions.ts` (default per news, summary, optimistic apply), unit-tested
- [x] 2.2 `BuddyReactionBar` under every post
