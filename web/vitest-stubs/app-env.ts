// See app-state.ts — exists so `vi.mock('$app/env', ...)` has a module id to intercept;
// a spec that renders something built on `FilterStore` (via urlSynced.svelte.ts) needs it.
export const browser = false;
