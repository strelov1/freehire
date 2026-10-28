<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { toPath } from '#lib/pathname.js';
  import { isAuthenticated, currentUser } from '#lib/auth.svelte.js';
  import { visibleAccountNav, isSectionActive } from '#lib/accountNav.js';
  import { accountNavIcons } from '#lib/accountNavIcons.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { messages, navLabel } from '#lib/i18n/shell.js';
  import { t } from '#lib/i18n/t.js';
  import { cn } from '#lib/ui/index.js';
  import AccountNavDrawer from './AccountNavDrawer.svelte';

  // A compact, icon-only mirror of the account-section navigation (see
  // my/+layout.svelte), pinned to the left edge of full-width surfaces that reset
  // past the account shell. Same items, gating, order, and active rule as the
  // sidebar; reuses the shared visible-nav model and icon map so there's one source
  // of truth. Renders nothing for a signed-out visitor.
  //
  // `collapsible` opts a host into responsive behaviour: at lg+ the icon rail shows
  // as usual, but below lg it disappears (freeing the width) and instead opens as a
  // labelled slide-in drawer driven by `open` — the host renders its own trigger
  // (e.g. a burger in its header) and binds `open`. Without `collapsible` the rail
  // renders exactly as before at every width.

  let { open = $bindable(false), collapsible = false }: { open?: boolean; collapsible?: boolean } =
    $props();
  const s = $derived(t(messages, locale()));
  const path = $derived(page.url.pathname);
  const navItems = $derived(
    visibleAccountNav(currentUser()?.role === 'moderator', currentUser()?.beta_tester ?? false),
  );
</script>

{#snippet navLink(item: ReturnType<typeof visibleAccountNav>[number])}
  {@const active = isSectionActive(path, item.href)}
  {@const Icon = accountNavIcons[item.href]}
  {@const label = navLabel(s, item.href, item.label)}
  <a
    href={resolve(toPath(item.href))}
    aria-current={active ? 'page' : undefined}
    title={label}
    class={cn(
      'flex items-center justify-center rounded-md p-2 transition-colors',
      active
        ? 'bg-secondary text-secondary-foreground'
        : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
    )}
  >
    <Icon class="size-4 shrink-0" />
    <span class="sr-only">{label}</span>
  </a>
{/snippet}

{#if isAuthenticated()}
  {#if collapsible}
    <!-- Desktop icon rail (lg+): identical to the non-collapsible rail. -->
    <nav
      aria-label={s.shell.accountSections}
      class="hidden w-14 shrink-0 flex-col gap-1 overflow-y-auto border-r border-border bg-background p-2 lg:flex"
    >
      {#each navItems as item (item.href)}
        {@render navLink(item)}
      {/each}
    </nav>

    <!-- Mobile drawer (<lg): a labelled slide-in over a dimmed backdrop. -->
    <AccountNavDrawer bind:open />
  {:else}
    <nav
      aria-label={s.shell.accountSections}
      class="flex w-14 shrink-0 flex-col gap-1 overflow-y-auto border-r border-border bg-background p-2"
    >
      {#each navItems as item (item.href)}
        {@render navLink(item)}
      {/each}
    </nav>
  {/if}
{/if}
