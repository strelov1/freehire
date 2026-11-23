<script lang="ts">
  import { Ellipsis } from '@lucide/svelte';
  import { menuNav } from '#lib/actions/menuNav.js';

  let { company, onHide }: { company: string; onHide: (trigger: HTMLButtonElement) => void } = $props();
  let root = $state<HTMLDivElement | null>(null);
  let trigger = $state<HTMLButtonElement | null>(null);
  let open = $state(false);
  const close = () => (open = false);

  function hide() {
    close();
    if (trigger) onHide(trigger);
  }
</script>

<svelte:window onclick={(event) => open && !root?.contains(event.target as Node) && close()} />

<div class="relative ml-auto w-fit" bind:this={root}>
  <button
    type="button"
    bind:this={trigger}
    aria-label={`More actions for ${company}`}
    aria-haspopup="menu"
    aria-expanded={open}
    class="grid size-11 place-items-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
    onclick={() => (open = !open)}
  >
    <Ellipsis class="size-5" aria-hidden="true" />
  </button>
  {#if open}
    <div
      role="menu"
      aria-label="More actions"
      tabindex="-1"
      class="absolute right-0 z-20 mt-1 min-w-max rounded-md border border-border bg-popover p-1 shadow-lg"
      {@attach menuNav({ onclose: close })}
    >
      <button
        type="button"
        role="menuitem"
        class="flex min-h-11 w-full rounded px-3 py-2 text-left text-sm hover:bg-accent focus:bg-accent focus:outline-none"
        onclick={hide}
      >
        Hide company
      </button>
    </div>
  {/if}
</div>
