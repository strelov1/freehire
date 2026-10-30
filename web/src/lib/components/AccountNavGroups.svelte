<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { toPath } from '#lib/pathname.js';
  import { currentUser } from '#lib/auth.svelte.js';
  import { activeSectionHref, workspaceFor, workspaceNav } from '#lib/accountNav.js';
  import { accountNavIcons, workspaceIcons } from '#lib/accountNavIcons.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { messages, navLabel } from '#lib/i18n/shell.js';
  import { t } from '#lib/i18n/t.js';
  import { cn } from '#lib/ui/index.js';

  // The current workspace's sections, grouped — the one rendering the account sidebar,
  // the phone drawer and the icon rail share, so the three cannot disagree about what a
  // workspace holds. The host owns the <nav> landmark around it.
  //
  //  - `sidebar`: labelled rows; `iconOnly` is the collapsed sidebar.
  //  - `drawer`: labelled rows with 44px touch targets.
  //  - `rail`: icon-only, the label kept for screen readers and the tooltip.

  let {
    variant,
    iconOnly = false,
    onnavigate,
  }: { variant: 'sidebar' | 'drawer' | 'rail'; iconOnly?: boolean; onnavigate?: () => void } = $props();

  const s = $derived(t(messages, locale()));
  const path = $derived(page.url.pathname);
  const compact = $derived(iconOnly || variant === 'rail');

  const workspaceId = $derived(workspaceFor(path));
  const groups = $derived(
    workspaceNav(workspaceId, currentUser()?.role === 'moderator', currentUser()?.beta_tester ?? false),
  );
  const activeHref = $derived(activeSectionHref(path, groups.flatMap((g) => g.items)));

  const rowClass = (active: boolean) =>
    cn(
      'flex items-center rounded-md transition-colors',
      variant === 'rail' ? 'justify-center p-2' : 'px-3 py-2 text-sm',
      variant === 'sidebar' && 'gap-2',
      variant === 'drawer' && 'gap-3 any-pointer-coarse:min-h-11',
      iconOnly && 'justify-center',
      active
        ? cn('bg-secondary text-secondary-foreground', variant === 'sidebar' && 'font-medium')
        : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
    );
</script>

<!-- Job search is home and needs no banner; a role workspace says which one you are in,
     since its sidebar no longer carries the sections you might be looking for. -->
{#if workspaceId !== 'seeker' && !compact}
  {@const Icon = workspaceIcons[workspaceId]}
  <div class="mb-2 flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm font-medium text-foreground">
    <Icon class="size-4 shrink-0" aria-hidden="true" />
    {s.workspaces[workspaceId]}
  </div>
{/if}

{#each groups as group, i (group.group)}
  {#if group.group !== 'main' && !compact}
    <div class="mt-4 px-3 pb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground/70">
      {s.navGroups[group.group]}
    </div>
  {:else if i > 0}
    <div class="my-2 h-px shrink-0 bg-border" role="presentation"></div>
  {/if}
  {#each group.items as item (item.href)}
    {@const active = item.href === activeHref}
    {@const Icon = accountNavIcons[item.href]}
    {@const label = navLabel(s, item.href, item.label)}
    <a
      href={resolve(toPath(item.href))}
      aria-current={active ? 'page' : undefined}
      title={compact ? label : undefined}
      onclick={onnavigate}
      class={rowClass(active)}
    >
      <Icon class="size-4 shrink-0" />
      {#if compact}
        <span class="sr-only">{label}</span>
      {:else}
        {label}
      {/if}
    </a>
  {/each}
{/each}
