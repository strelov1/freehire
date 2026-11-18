<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { ChevronDown, Settings, type LucideIcon } from '@lucide/svelte';
  import { currentUser } from '#lib/auth.svelte.js';
  import { activeSection, visibleAccountNav, visibleWorkspaces, type AccountNavItem } from '#lib/accountNav.js';
  import { workspaceIcons } from '#lib/accountNavIcons.js';
  import { toPath } from '#lib/pathname.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { messages, navLabel } from '#lib/i18n/shell.js';
  import { t } from '#lib/i18n/t.js';
  import { cn } from '#lib/ui/index.js';

  // Shared by the desktop profile menu and mobile drawer. Only the expanded
  // workspace exposes its destinations; settings belong to their own group.
  let { rowClass, onselect }: { rowClass: string; onselect: () => void } = $props();

  const uid = $props.id();
  const s = $derived(t(messages, locale()));
  const items = $derived(visibleAccountNav(currentUser()?.role === 'moderator', currentUser()?.beta_tester ?? false));
  const current = $derived(activeSection(page.url.pathname, items));
  const currentGroup = $derived(current ? (current.workspace ?? 'settings') : undefined);
  const workspaceList = $derived(visibleWorkspaces(currentUser()?.beta_tester ?? false, !!currentUser()?.api_tier));
  // undefined follows the current route; null is an explicit collapse.
  let expanded = $state<string | null | undefined>(undefined);
  const openGroup = $derived(expanded === undefined ? (currentGroup ?? null) : expanded);
  const settings = $derived(items.filter((item) => item.workspace === null));
</script>

{#snippet accordion(id: string, label: string, Icon: LucideIcon, links: readonly AccountNavItem[])}
  {@const open = openGroup === id}
  <button
    type="button"
    role="menuitem"
    id={`${uid}-${id}-trigger`}
    aria-expanded={open}
    aria-controls={`${uid}-${id}-links`}
    onclick={() => (expanded = open ? null : id)}
    class={cn(
      rowClass,
      'w-full text-left any-pointer-coarse:min-h-11',
      currentGroup === id && 'font-medium text-foreground',
    )}
  >
    <Icon class="size-4 shrink-0" aria-hidden="true" />
    <span class="min-w-0 flex-1">{label}</span>
    <ChevronDown
      class={cn('size-4 shrink-0 transition-transform motion-reduce:transition-none', open && 'rotate-180')}
      aria-hidden="true"
    />
  </button>
  <div id={`${uid}-${id}-links`} role="group" aria-labelledby={`${uid}-${id}-trigger`} hidden={!open}>
    {#if open}
      {#each links as item (item.href)}
        <a
          href={resolve(toPath(item.href))}
          role="menuitem"
          aria-current={current?.href === item.href ? 'page' : undefined}
          onclick={onselect}
          class={cn(
            rowClass,
            'pl-9 any-pointer-coarse:min-h-11',
            current?.href === item.href && 'bg-accent font-medium text-brand-strong',
          )}
        >
          {navLabel(s, item.href, item.label)}
        </a>
      {/each}
    {/if}
  </div>
{/snippet}

{#each workspaceList as w (w.id)}
  {@const Icon = workspaceIcons[w.id]}
  {#if w.id === 'buddies'}
    <a
      href={resolve(w.home)}
      role="menuitem"
      aria-current={current?.workspace === w.id ? 'page' : undefined}
      onclick={onselect}
      class={cn(
        rowClass,
        'any-pointer-coarse:min-h-11',
        current?.workspace === w.id && 'bg-accent font-medium text-brand-strong',
      )}
    >
      <Icon class="size-4 shrink-0" aria-hidden="true" />
      {s.workspaces[w.id]}
    </a>
  {:else}
    {@render accordion(
      w.id,
      s.workspaces[w.id],
      Icon,
      items.filter((item) => item.workspace === w.id),
    )}
  {/if}
{/each}
<div class="my-1 h-px shrink-0 bg-border"></div>
{@render accordion('settings', s.shell.settings, Settings, settings)}
