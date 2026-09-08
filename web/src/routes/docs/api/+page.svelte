<script lang="ts">
  // The API reference: server-rendered by +page.server.ts (renderApiReferenceToString)
  // for real content on the initial response, then hydrated client-side here with the
  // identical config (minus how the spec reaches each side — see scalarConfig.ts).
  // Scalar owns all navigation/search/try-it inside #scalar-app; this file only supplies
  // the page's SEO metadata and the design-system theme mapping around it.
  import { onMount } from 'svelte';
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
  // class on <html> — left alone, it never follows the site's theme toggle at
  // all. `forceDarkModeState` at mount picks the state that matches paint (the
  // no-FOUC inline script has already set the class by the time this runs);
  // `updateConfiguration` keeps it in sync with every later toggle.
  let instance: ApiReferenceInstance | undefined;
  onMount(async () => {
    const { createApiReference } = await import('@scalar/api-reference');
    instance = createApiReference('#scalar-app', {
      ...scalarConfigFromUrl(),
      forceDarkModeState: themeStore.isDark ? 'dark' : 'light',
    });
  });

  $effect(() => {
    const isDark = themeStore.isDark;
    instance?.updateConfiguration({ ...instance.getConfiguration(), forceDarkModeState: isDark ? 'dark' : 'light' });
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

     Targets `.light-mode`/`.dark-mode` directly, not `#scalar-app` itself:
     Scalar re-declares these variables on many individual descendants tagged
     with those classes (sidebar, request cards, ...), each shadowing an
     ancestor's value for its own subtree — an override placed only on
     `#scalar-app` would be shadowed the same way and never actually apply. */
  #scalar-app :global(.light-mode),
  #scalar-app :global(.dark-mode) {
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
