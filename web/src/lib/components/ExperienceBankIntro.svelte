<script lang="ts">
  import type { Snippet } from 'svelte';

  // The experience bank's header: what the page is for, and the way into the interview
  // (`children`).
  let { totalAtoms, children }: { totalAtoms: number; children: Snippet } = $props();
</script>

<!-- The header states what this page is FOR, because "experience bank" means nothing
       to someone meeting it for the first time. -->
<!-- A container query, not a viewport one: the same view fills /my/profile and sits in
       the tailoring workspace's ~350px side panel, where a wide viewport still left the
       paragraph one word per line beside the button. -->
<div class="@container">
  <div class="flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
    <p class="max-w-prose flex-1 text-sm text-muted-foreground">
      {#if totalAtoms === 0}
        Nothing recorded yet. Upload a CV, add a job or project below, or talk it through with the assistant — whatever
        you confirm is kept here and reused in every CV you build.
      {:else}
        {totalAtoms} achievement{totalAtoms === 1 ? '' : 's'} on record. These are what your tailored CVs are built from —
        edit or remove anything that is not right.
      {/if}
    </p>
    <!-- Capped, and right-aligned only once it shares the row: the example is two lines
           of hint, and letting it set the row's width pushed the whole action under the
           paragraph. Narrow, it takes its own line and follows the text's edge.
           `basis-full` below `@lg` is what forces that own line — `flex-wrap` alone could not,
           because the paragraph beside it is `flex-1` (so `flex-basis: 0`) and never claims
           enough width to push anything down. With the action also refusing to shrink, it held
           its full 16rem on a phone and crushed the paragraph to one word per line. -->
    <div
      class="flex basis-full flex-col items-start gap-1.5 text-left @lg:max-w-[16rem] @lg:basis-auto @lg:shrink-0 @lg:items-end @lg:text-right"
    >
      {@render children()}
    </div>
  </div>
</div>
