<script lang="ts" module>
  import { tv, type VariantProps } from 'tailwind-variants';

  export const tabsListVariants = tv({
    base: 'inline-flex items-center justify-center gap-1 rounded-lg bg-muted p-1',
  });

  export const tabsTriggerVariants = tv({
    base: 'inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1 text-sm font-medium transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50',
    variants: {
      active: {
        true: 'bg-card text-foreground shadow-sm',
        false: 'text-muted-foreground hover:text-foreground',
      },
    },
    defaultVariants: { active: false },
  });

  export type TabsTriggerActive = VariantProps<typeof tabsTriggerVariants>['active'];
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import { cn } from './cn.js';

  let {
    value = $bindable(),
    tabs,
    class: className,
    children,
  }: {
    value?: string;
    tabs: { value: string; label: string }[];
    class?: string;
    children: Snippet;
  } = $props();

  const uid = $props.id();
  const tabId = (v: string) => `${uid}-tab-${v}`;
  const panelId = `${uid}-panel`;

  let triggers: HTMLButtonElement[] = $state([]);

  // A tablist always has exactly one selected tab, so an unset `value` reads as
  // the first one. Keying the roving tabindex off `value` directly would leave
  // every trigger at tabindex="-1" until the consumer picked a tab: the tablist
  // unreachable by keyboard, and the arrow keys with no index to move from.
  let selected = $derived(value ?? tabs[0]?.value);

  function activate(v: string) {
    value = v;
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return;
    const dir = e.key === 'ArrowRight' ? 1 : -1;
    const idx = tabs.findIndex((t) => t.value === selected);
    if (idx === -1) return;
    const nextIdx = (idx + dir + tabs.length) % tabs.length;
    const next = tabs[nextIdx];
    if (!next) return;
    e.preventDefault();
    activate(next.value);
    // Roving tabindex: the old trigger just became tabindex="-1", so focus
    // has to follow the selection or it lands nowhere.
    triggers[nextIdx]?.focus();
  }
</script>

<div class={cn('flex flex-col gap-2', className)}>
  <div class={tabsListVariants()} role="tablist">
    {#each tabs as tab, i (tab.value)}
      <button
        bind:this={triggers[i]}
        type="button"
        role="tab"
        id={tabId(tab.value)}
        aria-controls={panelId}
        aria-selected={selected === tab.value}
        tabindex={selected === tab.value ? 0 : -1}
        class={tabsTriggerVariants({ active: selected === tab.value })}
        onclick={() => activate(tab.value)}
        onkeydown={onKeydown}
      >
        {tab.label}
      </button>
    {/each}
  </div>
  <div role="tabpanel" id={panelId} aria-labelledby={selected ? tabId(selected) : undefined}>
    {@render children()}
  </div>
</div>
