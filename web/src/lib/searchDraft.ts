// The header search box holds a DRAFT. Typing no longer runs the search — only
// Enter or choosing a suggestion commits it — so one string becomes two, and the
// difficulty is keeping them honest when the committed query moves on its own:
// back/forward, a filter chip removed, a suggestion applied from elsewhere.
//
// Pure by design: no Svelte, no DOM. The component holds one of these in `$state`
// and replaces it on each transition, so every decision below is testable without a
// Svelte runtime (the same reason paginated.svelte.test.ts covers `loadWithRetry`
// rather than `Paginator`).

/** What the box shows, and the committed query it was last reconciled against.
 *  `committed` is not "what the list is filtered by" — it is our record of it, and
 *  the difference between the two is what `reconcile` reads. */
export interface SearchDraft {
  /** The text in the box, committed or not. */
  readonly text: string;
  /** The committed query as of the last transition. */
  readonly committed: string;
}

/** A draft showing an already-committed query — the state the box opens in, so a
 *  shared `?q=…` link renders its query rather than an empty box. */
export function emptyDraft(committed: string): SearchDraft {
  return { text: committed, committed };
}

/** The visitor typed. The committed query is untouched: that is the whole point. */
export function edit(draft: SearchDraft, text: string): SearchDraft {
  return { text, committed: draft.committed };
}

/** Enter, or a chosen suggestion. Trimmed, so a stray space does not read as a
 *  different query — and the trimmed text becomes what the box shows, so the box and
 *  the list cannot disagree about what was searched. */
export function commit(draft: SearchDraft): SearchDraft {
  const text = draft.text.trim();
  return { text, committed: text };
}

/** Fold in the query the list actually carries now.
 *
 *  A value DIFFERENT from the one we last recorded can only have come from outside
 *  the box — history navigation, a chip removed, a facet applied — and must reach the
 *  box, or it shows a query the list is no longer running.
 *
 *  An UNCHANGED value tells us nothing new, so uncommitted typing survives it. That
 *  asymmetry is the feature: without it, every store notification would overwrite the
 *  half-typed word with the last committed query. */
export function reconcile(draft: SearchDraft, committed: string): SearchDraft {
  if (committed === draft.committed) return draft;
  return { text: committed, committed };
}
