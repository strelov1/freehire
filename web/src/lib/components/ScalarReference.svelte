<script lang="ts">
  // The Scalar mount + theme bridge shared by /docs/api and /docs/api/internal — the
  // only thing that differs between the two pages is which spec `config` points at.
  // `scalarHtml` is the SSR fragment from the page's own +page.server.ts (same config,
  // rendered via renderApiReferenceToString); this hydrates it client-side.
  import { themeStore } from '#lib/theme.svelte.js';
  import type { ApiReferenceInstance, AnyApiReferenceConfiguration } from '@scalar/types/api-reference';
  import '@scalar/api-reference/style.css';

  let { scalarHtml, config }: { scalarHtml: string; config: () => AnyApiReferenceConfiguration } = $props();

  // Scalar tracks its own light/dark state independently of the site's `.dark`
  // class on <html>, and reads `forceDarkModeState` only once at setup — not
  // reactively (@scalar/use-hooks' useColorMode destructures it from its opts
  // at call time, with no watch on later changes), so `updateConfiguration()`
  // alone cannot change it after mount despite otherwise updating the merged
  // config. A full destroy-and-recreate on every theme change is the only way
  // Scalar's own color mode actually follows the site's toggle.
  const darkModeState = $derived(themeStore.isDark ? 'dark' : 'light');

  let instance: ApiReferenceInstance | undefined;
  $effect(() => {
    const forceDarkModeState = darkModeState;
    let cancelled = false;
    void (async () => {
      const { createApiReference } = await import('@scalar/api-reference');
      if (cancelled) return;
      instance?.destroy();
      instance = createApiReference('#scalar-app', { ...config(), forceDarkModeState });
    })();
    return () => {
      cancelled = true;
    };
  });
</script>

<!-- eslint-disable-next-line svelte/no-at-html-tags -- server-rendered by renderApiReferenceToString from the generated OpenAPI document, no user input -->
<div id="scalar-app">{@html scalarHtml}</div>

<style>
  /* Map Scalar's own theme surface onto the design system's live tokens (not a
     static snapshot) so it tracks the site's light/dark toggle automatically —
     see openspec/changes/migrate-api-docs-scalar/design.md, Decision 6.

     Targets `.light-mode`/`.dark-mode` unscoped, not `#scalar-app` or any
     selector requiring them as its descendant: Scalar's `useColorMode` applies
     the mode class to `document.body` — an ANCESTOR of #scalar-app, which a
     descendant-only selector can never match — and separately re-declares the
     same variables on individual components (request/response example cards)
     that carry their own literal copy of the class. Both need the override,
     so the selector matches the class wherever it appears. */
  :global(.light-mode),
  :global(.dark-mode) {
    --scalar-background-1: var(--background) !important;
    --scalar-background-2: var(--secondary) !important;
    --scalar-background-3: var(--muted) !important;
    --scalar-background-accent: var(--brand-muted) !important;

    --scalar-color-1: var(--foreground) !important;
    --scalar-color-2: var(--muted-foreground) !important;
    --scalar-color-3: var(--muted-foreground) !important;
    --scalar-color-accent: var(--brand-strong) !important;

    --scalar-border-color: var(--border) !important;

    --scalar-link-color: var(--brand-strong) !important;
    --scalar-link-color-hover: var(--brand) !important;
    --scalar-link-color-visited: var(--brand-strong) !important;

    --scalar-color-danger: var(--destructive) !important;
    --scalar-background-danger: var(--secondary) !important;

    --scalar-button-1: var(--brand) !important;
    --scalar-button-1-color: var(--brand-foreground) !important;
    --scalar-button-1-hover: var(--brand-strong) !important;

    --scalar-font: var(--font-sans) !important;
    --scalar-font-code: var(--font-mono) !important;

    --scalar-radius: var(--radius) !important;
  }

  /* Scalar's markdown tables use `table-layout: fixed`, which splits every table's
     columns evenly regardless of content. The "Filtering jobs" facet table has a
     short Param/Filter column beside a Values column holding long comma-separated
     lists, so at the width available inside Scalar's content pane the fixed split
     squeezes Param/Filter narrower than a single word — and `word-break: break-word`
     (also Scalar's own) then breaks mid-word.

     `table-layout: auto` alone does not fix this: the browser's auto-layout
     algorithm still shrinks a column to its CSS min-content width when the table
     does not fit its container, and `word-break: break-word` makes a cell's
     min-content effectively one character wide — so Param/Filter kept breaking
     mid-word even with `auto` (confirmed live on freehire.me at both a 1280px and
     a 390px viewport: computed `th` width was 51px, one letter per line).

     `word-break: normal` restores the real min-content width (a whole word, or a
     hyphenated segment), which is what actually stops the mid-word break. That
     alone would just overflow the pane instead, since nothing here made the table
     — or any ancestor, checked live, all `overflow-x: visible` — a scroll
     container; `overflow-x: auto` on the table itself is what turns "wider than
     the pane" into a swipeable table instead of a layout overflow. The Values
     column still wraps normally at its own commas/spaces either way — only a
     single unbroken word (a param name, a short label) was ever forced this
     narrow. */
  :global(.scalar-app .markdown table) {
    table-layout: auto !important;
    overflow-x: auto !important;
    max-width: 100%;
  }

  :global(.scalar-app .markdown table th),
  :global(.scalar-app .markdown table td) {
    word-break: normal !important;
  }
</style>
