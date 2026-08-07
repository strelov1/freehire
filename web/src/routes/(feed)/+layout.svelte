<script lang="ts">
  import type { Snippet } from 'svelte';
  import { page } from '$app/state';
  import JobsView from '$lib/components/JobsView.svelte';
  import type { LayoutData } from './$types';

  let { data, children }: { data: LayoutData; children: Snippet } = $props();

  // Whether a job is open — drives the rail's selected-card ring and, below the
  // desktop breakpoint, which of {list, pane} is visible. Read from the route's own
  // param (not click state), so a direct link to /jobs/[slug] renders identically to
  // a client-side selection.
  const selectedSlug = $derived(page.params.slug);
</script>

<!-- Deliberately no fixed/viewport-capped height on this row: the banners above it
     (ProductHuntBanner, EmailVerificationBanner) are self-gating and change the space
     left under the viewport, so a hardcoded `100vh - <topbar>` figure would either
     leave a gap or, worse, cap this row shorter than the detail pane's real content —
     which would overflow past the box and get covered by the Footer sitting right
     after it in the root layout. Instead only the list column bounds its own height
     (sticky + max-h + its own scroll, independent of whatever the pane does); the pane
     and the page around it grow with content like any ordinary page, so the Footer
     always lands after everything, never under it. -->
<div class="mx-auto flex w-full max-w-[1600px] gap-6 px-4 py-6">
  <!-- List column: on mobile, hidden once a job is open (the pane takes the full
       width instead); at the desktop breakpoint both columns always show together. -->
  <div
    class={[
      'w-full shrink-0 lg:w-[360px]',
      selectedSlug ? 'hidden lg:block' : 'block',
      'lg:sticky lg:top-[calc(3.5rem+1.5rem)] lg:max-h-[calc(100vh-3.5rem-3rem)] lg:overflow-y-auto',
    ]}
  >
    <JobsView initial={data.initial} layout="stacked" {selectedSlug} />
  </div>

  <!-- Detail pane: on mobile, hidden until a job is open; at the desktop breakpoint
       always shown (the no-selection placeholder from +page.svelte fills it). Grows
       naturally with its content — no scroll/height constraint of its own. -->
  <div class={['min-w-0 flex-1', selectedSlug ? 'block' : 'hidden lg:block']}>
    {@render children()}
  </div>
</div>
