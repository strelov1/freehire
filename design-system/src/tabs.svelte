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

  function activate(v: string) {
    value = v;
  }

  function onKeydown(e: KeyboardEvent) {
    const idx = tabs.findIndex((t) => t.value === value);
    if (idx === -1) return;
    if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') {
      e.preventDefault();
      const dir = e.key === 'ArrowRight' ? 1 : -1;
      const next = (idx + dir + tabs.length) % tabs.length;
      activate(tabs[next].value);
      // Roving tabindex: the old trigger just became tabindex="-1", so focus
      // has to follow the selection or it lands nowhere.
      triggers[next]?.focus();
    }
  }
</script>

<div class={cn('flex flex-col gap-2', className)}>
  <div
    class={tabsListVariants()}
    role="tablist"
    onkeydown={onKeydown}
  >
    {#each tabs as tab, i (tab.value)}
      <button
        bind:this={triggers[i]}
        type="button"
        role="tab"
        id={tabId(tab.value)}
        aria-controls={panelId}
        aria-selected={value === tab.value}
        tabindex={value === tab.value ? 0 : -1}
        class={tabsTriggerVariants({ active: value === tab.value })}
        onclick={() => activate(tab.value)}
      >
        {tab.label}
      </button>
    {/each}
  </div>
  <div role="tabpanel" id={panelId} aria-labelledby={value ? tabId(value) : undefined}>
    {@render children()}
  </div>
</div>
