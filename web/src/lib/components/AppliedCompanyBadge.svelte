<script lang="ts">
  import { History } from '@lucide/svelte';
  import { cn } from '#lib/ui/index.js';

  // Marks a posting whose employer the signed-in reader has already applied to — through
  // this posting or any other of theirs (see appliedCompanies.svelte.ts). A fact about the
  // READER, so it is neutral rather than brand-toned: it informs a decision ("do I apply
  // there again?"), it does not recommend one. The parent decides whether it shows.
  let { class: className = '' }: { class?: string } = $props();
</script>

<span
  title="You already applied to a position at this company"
  class={cn(
    'inline-flex items-center gap-1.5 rounded-md border border-border bg-secondary px-2 py-0.5 text-xs font-medium text-secondary-foreground',
    className,
  )}
>
  <History class="size-3.5 shrink-0" aria-hidden="true" />
  Applied to this company
</span>
