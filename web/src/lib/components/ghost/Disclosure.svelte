<script lang="ts">
  import type { Snippet } from 'svelte';
  import { ChevronDown } from '@lucide/svelte';

  // A styling wrapper over native `<details>`, and nothing more.
  //
  // The platform supplies the expanded state, the trigger-to-content association and
  // keyboard operation, and it all works before client JavaScript runs — which matters
  // on a server-rendered page whose collapsed text must also be present for the FAQ
  // structured data. GhostChecklist hand-rolls its disclosure because its trigger row
  // carries a gauge, a chip and a scale with custom layout; a plain one has no such
  // excuse, and re-implementing `aria-expanded` here would be a shim over the API that
  // already does it.
  let {
    summary,
    children,
    class: className = '',
  }: { summary: string; children: Snippet; class?: string } = $props();
</script>

<details class="group {className}">
  <summary
    class="flex cursor-pointer list-none items-center gap-1.5 text-sm text-muted-foreground marker:hidden hover:text-foreground [&::-webkit-details-marker]:hidden"
  >
    {summary}
    <ChevronDown
      class="size-3.5 shrink-0 transition-transform group-open:rotate-180"
      aria-hidden="true"
    />
  </summary>
  {@render children()}
</details>
