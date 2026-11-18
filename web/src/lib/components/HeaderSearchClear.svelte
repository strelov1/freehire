<script lang="ts">
  import { X } from '@lucide/svelte';

  // The header search box's tail: the clear button while it holds text, the `/` hotkey
  // hint while it is empty.
  let { empty, onclear }: { empty: boolean; onclear: () => void } = $props();
</script>

{#if !empty}
  <!-- Clearing is an explicit act, not typing, so it commits at once: the visitor
       asked for the unfiltered list, not for an empty box over the old results. -->
  <button
    type="button"
    onclick={onclear}
    aria-label="Clear search"
    class="grid size-6 shrink-0 place-items-center rounded text-muted-foreground transition-colors hover:text-foreground"
  >
    <!-- 24px box around the 16px glyph: the glyph alone was the whole target. -->
    <X class="size-4" aria-hidden="true" />
  </button>
{:else}
  <kbd class="hidden shrink-0 rounded border border-border px-1.5 text-xs text-muted-foreground sm:inline">/</kbd>
{/if}
