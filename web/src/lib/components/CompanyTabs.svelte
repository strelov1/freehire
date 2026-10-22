<script lang="ts">
  import type { Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { TabStrip, tabStripId } from '#lib/ui/index.js';

  // The company page's two views, each its own URL: the job list and the Hiring tab.
  // Routes rather than client state so the Hiring view is linkable, server-rendered and
  // readable by a crawler — TabStrip renders an `href` tab as a real anchor.
  type CompanyTab = 'jobs' | 'hiring';

  let { slug, active, children }: { slug: string; active: CompanyTab; children: Snippet } = $props();

  const PANEL_ID = 'company-tabpanel';
  const tabs = $derived([
    { id: 'jobs' as const, label: 'Jobs', href: resolve('/companies/[slug]', { slug }) },
    { id: 'hiring' as const, label: 'Hiring', href: resolve('/companies/[slug]/hiring', { slug }) },
  ]);
</script>

<TabStrip {tabs} {active} label="Company sections" panelId={PANEL_ID} class="mt-4" />

<div role="tabpanel" id={PANEL_ID} aria-labelledby={tabStripId(PANEL_ID, active)} tabindex="-1">
  {@render children()}
</div>
