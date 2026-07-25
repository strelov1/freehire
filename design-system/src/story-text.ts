import { createRawSnippet } from 'svelte';

/**
 * A component's children are a `Snippet` in Svelte 5, so a story cannot hand it
 * plain text through `args`. Wraps an authored string in the smallest snippet
 * that renders it.
 */
export function text(value: string) {
  return createRawSnippet(() => ({ render: () => `<span>${value}</span>` }));
}
