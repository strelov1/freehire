<script lang="ts">
  import type { Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { TabStrip, tabStripId } from '#lib/ui/index.js';

  // The company page's views, each its own URL: the job list, the Hiring tab, the
  // closed list and the discussion. Routes rather than client state so each is
  // linkable, server-rendered and readable by a crawler — TabStrip renders an
  // `href` tab as a real anchor.
  type CompanyTab = 'jobs' | 'hiring' | 'closed' | 'discussion';

  let { slug, active, children }: { slug: string; active: CompanyTab; children: Snippet } = $props();

  // Open-thread count for the "Discussion · N" label; loaded client-side so the strip
  // renders immediately and the number fills in. Failures leave it off.
  let threadCount = $state<number | null>(null);
  $effect(() => {
    let alive = true;
    api
      .countThreads('company', slug)
      .then((n) => alive && (threadCount = n))
      .catch(() => {});
    return () => {
      alive = false;
    };
  });

  const PANEL_ID = 'company-tabpanel';
  const tabs = $derived([
    { id: 'jobs' as const, label: 'Jobs', href: resolve('/companies/[slug]', { slug }) },
    { id: 'hiring' as const, label: 'Hiring', href: resolve('/companies/[slug]/hiring', { slug }) },
    { id: 'closed' as const, label: 'Closed', href: resolve('/companies/[slug]/closed', { slug }) },
    {
      id: 'discussion' as const,
      label: threadCount ? `Discussion · ${threadCount}` : 'Discussion',
      href: resolve('/companies/[slug]/discussion', { slug }),
    },
  ]);
</script>

<TabStrip {tabs} {active} label="Company sections" panelId={PANEL_ID} class="mt-4" />

<div role="tabpanel" id={PANEL_ID} aria-labelledby={tabStripId(PANEL_ID, active)} tabindex="-1">
  {@render children()}
</div>
