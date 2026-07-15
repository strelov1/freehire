## 1. Link-state view model (pure logic)

- [x] 1.1 RED: unit-test a pure `inboxLinkState(email, lastUnlinked)` helper that
      returns which control to show — `linked` | `suggested` | `undo` | `picker`
      — covering: linked→linked; suggested→suggested; unlinked with matching
      lastUnlinked→undo; unlinked with non-matching/absent lastUnlinked→picker.
- [x] 1.2 GREEN: implement the helper (in a `.ts` module so it is vitest-testable
      apart from the component). REFACTOR + simplify under green.

## 2. ApplicationLinkPicker component

- [x] 2.1 Build `ApplicationLinkPicker.svelte`: props = tracked applications +
      `onpick(slug)`; searchable list; empty-state message. Presentational only.
- [x] 2.2 Verify via `svelte-check` + a visual headless-Chrome pass of the picker
      states (list, filtered, empty).

## 3. Wire into InboxView

- [x] 3.1 Add lazy `listTracked` fetch, the `lastUnlinked` rune, `linkTo(slug)`
      (calls `api.linkEmail`, clears `lastUnlinked`) and `undoUnlink()`; have
      `unlink()` stash `{id, slug, company}`; clear the stash on email select.
- [x] 3.2 Render the picker / "Unlinked · Undo" in the neither-state using the
      `inboxLinkState` helper.
- [x] 3.3 Verify: `svelte-check` clean + visual pass of link → unlink → undo and
      unlinked → pick → linked in a real browser.
