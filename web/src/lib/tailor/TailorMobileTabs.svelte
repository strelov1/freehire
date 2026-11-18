<script lang="ts" generics="T extends string">
  import { Menu } from '@lucide/svelte';

  // The tailoring workspace's tab bar below lg, where its columns collapse to one view at a
  // time: the burger for the account navigation (`navOpen`), then one tab per view.
  let {
    tabs,
    current,
    navOpen = $bindable(),
    onPick,
  }: { tabs: readonly (readonly [T, string])[]; current: T; navOpen: boolean; onPick: (id: T) => void } = $props();
</script>

<!-- MOBILE TAB BAR: below lg the three columns collapse to one full-screen view; this flat,
     horizontally-scrollable bar switches between all of them. Hidden at lg (columns stack). -->
<nav class="flex items-center gap-1 overflow-x-auto border-b border-border bg-background px-2 py-1.5 text-sm lg:hidden">
  <!-- Burger opens the account nav drawer (the icon rail is hidden below lg). -->
  <button
    type="button"
    onclick={() => (navOpen = true)}
    aria-label="Open menu"
    aria-expanded={navOpen}
    class="shrink-0 rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
  >
    <Menu class="size-5" />
  </button>
  <span class="mr-0.5 h-5 w-px shrink-0 bg-border" aria-hidden="true"></span>
  {#each tabs as [id, label] (id)}
    <button
      type="button"
      onclick={() => onPick(id)}
      aria-current={current === id ? 'page' : undefined}
      class={[
        'shrink-0 rounded px-2 py-1 transition-colors',
        current === id ? 'bg-muted font-medium text-foreground' : 'text-muted-foreground hover:text-foreground',
      ]}
    >
      {label}
    </button>
  {/each}
</nav>
