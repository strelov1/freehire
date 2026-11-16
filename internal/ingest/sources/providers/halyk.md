**Halyk Bank traps** (`halyk`; checked 2026-11-16):

- **The numeric URL is a list position, not a posting id.** `/vacancies-inner/32`
  and `/40` can change when the list is reordered. The application form's `data-id`
  is `69` on both pages, so it cannot identify either posting. Every crawl reads
  details again and updates each stored URL.
- **Title and city are not always unique.** The live listing stated 81 vacancies
  and included two `Junior Developer` cards in Almaty: one described Creatio CRM
  development, the other .NET microservices and ReportServer. Their branch badges
  were `Головной банк` and blank. The bounded fixtures retain the two public bodies
  stored on 2026-11-14 and checked against those live pages on 2026-11-16.
- **Only a current collision adds a content suffix.** Ordinary postings keep the
  existing title/city hash. Every member of a shared title/city group appends the
  first eight SHA-256 bytes of its lowercased, whitespace-normalized visible body
  text. This reproduces the two existing stored ids ending `ddb8dc2bcc268b18` and
  `1a51477fe215da03`. Card order and URL positions do not affect it; markup, case
  and whitespace changes do not either. The body is used because a branch badge
  can be absent and does not identify the role.
- **Unresolved identity fails the whole board.** If a shared-title detail is
  unreadable, empty or normalizes to another member's body, the crawl cannot safely
  identify every listed role and returns no partial success. Ordinary unreadable
  details still produce unreadable markers; explicit 404/410 responses still drop
  gone postings. The raw card count must equal the site's stated total.
- **Collision history is not persisted.** A substantive body edit in a collision
  changes that posting's suffix. If only one member remains listed, it returns to
  the original title/city id. Introducing a collision therefore leaves any former
  unsuffixed row unseen; resolving one leaves suffixed rows unseen. Those rows close
  through the normal unseen-job lifecycle. No historical matching or database
  rewrite is attempted, and an old identity can coexist until that close.
