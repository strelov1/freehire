<script lang="ts">
  import { resolve } from '$app/paths';
  import { Check } from '@lucide/svelte';

  // The note under a job card right after it was saved: it is on the tracking board now,
  // and this is the way to it (`slug`, the job's public slug).
  let { slug }: { slug: string } = $props();
</script>

<!-- The same confirmation the job page shows after applying: saving puts the job on
   the tracking board, and this is the way there while the click is still fresh.
   Not in `compact`, where the chat column has no room for a second row. -->
<div
  class="relative z-10 mx-4 mb-3 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-brand/30 bg-brand-muted px-3 py-2 text-sm"
>
  <span class="inline-flex items-center gap-1.5 font-medium text-brand-strong">
    <Check class="size-4 shrink-0" aria-hidden="true" /> Saved to your board
  </span>
  <a
    href={resolve('/my/tracking/[id]', { id: slug })}
    class="font-medium text-brand-strong underline underline-offset-4"
  >
    Open in tracker →
  </a>
</div>
