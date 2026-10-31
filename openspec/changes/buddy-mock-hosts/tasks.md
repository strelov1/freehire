## 1. Backend

- [x] 1.1 Batched queries: host candidates (capped), availability and busy time for many
      members, and the member-page summary as a list query
- [x] 1.2 `MockService.Hosts`: slots per candidate through the existing engine, the
      review aggregate, sorting, the topic check
- [x] 1.3 `GET /api/v1/me/buddies/mocks/hosts`, cookie-only, `ErrUnknownTopic` → 422
- [x] 1.4 Integration tests: exclusions, topic filter, order, summary across a block,
      non-member, handler auth/403/422

## 2. Web

- [ ] 2.1 "Who runs mocks" block on `/buddies/mocks` with topic filter, loading, error and
      empty states
- [ ] 2.2 Weekly availability editor reworked (shared with the mentor schedule page)
- [ ] 2.3 Settings form and the member's own mock list polished
