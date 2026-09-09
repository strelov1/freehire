<script lang="ts">
  // The API reference: server-rendered by +page.server.ts (renderApiReferenceToString)
  // for real content on the initial response, then hydrated client-side here with the
  // identical config (minus how the spec reaches each side — see scalarConfig.ts).
  // Scalar owns all navigation/search/try-it inside #scalar-app; this file only supplies
  // the page's SEO metadata and the design-system theme mapping around it.
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import { scalarConfigFromUrl } from '$lib/docs/scalarConfig';
  import { breadcrumbJsonLd, jsonLdScript, webApiJsonLd } from '$lib/seo';
  import { themeStore } from '$lib/theme.svelte';
  import type { ApiReferenceInstance } from '@scalar/types/api-reference';
  import '@scalar/api-reference/style.css';

  let { data } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/docs/api`);
  const jsonLd = $derived(
    jsonLdScript([
      webApiJsonLd(origin),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'API reference', url: canonical },
      ]),
    ]),
  );

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
      instance = createApiReference('#scalar-app', { ...scalarConfigFromUrl(), forceDarkModeState });
    })();
    return () => {
      cancelled = true;
    };
  });
</script>

<Seo
  title="freehire API reference — query jobs by filters"
  description="The freehire HTTP API: a read-first, open endpoint set over the job catalogue. Search and filter jobs by seniority, skills, region, salary and more, read companies, and track applications with an API key."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<!-- eslint-disable-next-line svelte/no-at-html-tags -- server-rendered by renderApiReferenceToString from the generated OpenAPI document, no user input -->
<div id="scalar-app">{@html data.scalarHtml}</div>

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
</style>
