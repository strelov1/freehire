<script lang="ts">
  import { resolve } from '$app/paths';
  import States from './States.svelte';

  // The job list with nothing in it: under the Hiring again or watchlist scope (`kind`), each
  // saying where its companies come from, or a plain miss, which `onRelax` (when the list can
  // drop its narrowest filter) offers to broaden.
  let { kind, onRelax }: { kind: 'formerEmployers' | 'watchlist' | 'none'; onRelax?: () => void } = $props();
</script>

{#if kind === 'formerEmployers'}
  <!-- Offer the experience bank without attributing a combined-filter miss to
       the employers' hiring activity. Ambiguous company names are not guessed. -->
  <States state="empty" message="No jobs at your former employers match the current filters." />
  <p class="mt-2 text-center text-sm text-muted-foreground">
    We match the companies in
    <a href={resolve('/my/profile/experience')} class="font-medium text-foreground underline underline-offset-2"
      >your experience</a
    >, and leave out any name that could be more than one company.
  </p>
{:else if kind === 'watchlist'}
  <States state="empty" message="No jobs at the companies on your watchlist match the current filters." />
  <p class="mt-2 text-center text-sm text-muted-foreground">
    Add more companies on
    <a href={resolve('/my/watchlist')} class="font-medium text-foreground underline underline-offset-2"
      >your watchlist</a
    >.
  </p>
{:else}
  <States state="empty" message="No matching jobs." />
  {#if onRelax}
    <!-- No semantic fallback in this slice: offer an honest one-step broaden
         instead of silently widening the feed. -->
    <div class="mt-4 flex justify-center">
      <button
        type="button"
        onclick={onRelax}
        class="inline-flex items-center gap-1.5 rounded-lg bg-brand px-4 py-2 text-sm font-semibold text-brand-foreground transition-opacity hover:opacity-90"
      >
        Broaden search
      </button>
    </div>
  {/if}
{/if}
