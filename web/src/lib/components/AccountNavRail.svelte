<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { isAuthenticated, currentUser } from '$lib/auth.svelte';
  import { visibleAccountNav, isSectionActive } from '$lib/accountNav';
  import { accountNavIcons } from '$lib/accountNavIcons';
  import { cn } from '$lib/utils';

  // A compact, icon-only mirror of the account-section navigation (see
  // my/+layout.svelte), pinned to the left edge of full-width surfaces that reset
  // past the account shell — currently the Agent page. Same items, gating, order,
  // and active rule as the sidebar; no labels, no collapse. Reuses the shared
  // visible-nav model and icon map so there's one source of truth. Renders nothing
  // for a signed-out visitor.

  const path = $derived(page.url.pathname);
  const navItems = $derived(
    visibleAccountNav(currentUser()?.role === 'moderator', currentUser()?.beta_tester ?? false),
  );
</script>

{#if isAuthenticated()}
  <nav
    aria-label="Account sections"
    class="flex w-14 shrink-0 flex-col gap-1 overflow-y-auto border-r border-border bg-background p-2"
  >
    {#each navItems as item (item.href)}
      {@const active = isSectionActive(path, item.href)}
      {@const Icon = accountNavIcons[item.href]}
      <a
        href={resolve(item.href)}
        aria-current={active ? 'page' : undefined}
        title={item.label}
        class={cn(
          'flex items-center justify-center rounded-md p-2 transition-colors',
          active
            ? 'bg-secondary text-secondary-foreground'
            : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
        )}
      >
        <Icon class="size-4 shrink-0" />
        <span class="sr-only">{item.label}</span>
      </a>
    {/each}
  </nav>
{/if}
