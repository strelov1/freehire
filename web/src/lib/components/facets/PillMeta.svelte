<script lang="ts">
  import { cn } from '#lib/ui/index.js';
  import { pillCountClass } from './pill';

  // The tail of a filter pill's content, after its label: the visually hidden
  // ", excluded" state word (see pillPressed in pill.ts for the pattern) and the option
  // count. The count carries a hidden ", " in front of it — an accessible name is the
  // pill's text run together, so without it "Remote" and "1,234" are announced as
  // "Remote1,234". `count` arrives formatted, because the call sites format it
  // differently and that is not this component's decision.
  let {
    included,
    excluded,
    count,
    class: className,
  }: { included: boolean; excluded: boolean; count?: string; class?: string } = $props();
</script>

{#if excluded}<span class="sr-only">, excluded</span>{/if}{#if count !== undefined}<span
    class={cn(pillCountClass(included || excluded), className)}><span class="sr-only">, </span>{count}</span
  >{/if}
